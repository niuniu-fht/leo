package handler

import (
	"log"
	"strings"
	"sync"
	"time"

	"leo2api/internal/provider/leonardo"
)

const defaultTokenDispatchBucketRebuildInterval = 5 * time.Minute

type tokenDispatchBucketKey struct {
	Bucket string
	Tier   string
}

type tokenDispatchBuckets struct {
	mu           sync.Mutex
	buckets      map[tokenDispatchBucketKey][]string
	cursor       map[tokenDispatchBucketKey]int
	cooldown     map[string]time.Time
	lastRebuild  time.Time
	lastTokenPut map[string]time.Time
}

var tokenDispatchImageTiers = []string{"1k", "2k", "4k"}
var tokenDispatchRepresentativeModels = []string{
	"gpt-image-2",
	"gpt-image-2-high",
	"gpt-image-2-higher",
	"gpt-image-gemini-3.1-flash-image",
	"gpt-image-gemini-3-pro-image",
}

func newTokenDispatchBuckets() *tokenDispatchBuckets {
	return &tokenDispatchBuckets{
		buckets:      make(map[tokenDispatchBucketKey][]string),
		cursor:       make(map[tokenDispatchBucketKey]int),
		cooldown:     make(map[string]time.Time),
		lastTokenPut: make(map[string]time.Time),
	}
}

func (s *Server) StartTokenDispatchBucketLoop() {
	if s == nil || s.TokenMgr == nil {
		return
	}
	s.tokenBucketLoopMu.Lock()
	if s.tokenBucketLoopStarted {
		s.tokenBucketLoopMu.Unlock()
		return
	}
	s.tokenBucketLoopStarted = true
	s.tokenBucketLoopMu.Unlock()

	s.rebuildTokenDispatchBuckets("startup")
	go func() {
		ticker := time.NewTicker(s.tokenDispatchBucketRebuildInterval())
		defer ticker.Stop()
		log.Printf("[token_bucket] rebuild loop started interval=%s", s.tokenDispatchBucketRebuildInterval())
		for range ticker.C {
			s.rebuildTokenDispatchBuckets("interval")
		}
	}()
}

func (s *Server) tokenDispatchBucketRebuildInterval() time.Duration {
	if s != nil && s.Config != nil {
		minutes := s.Config.GetInt("token_bucket_rebuild_interval_minutes", int(defaultTokenDispatchBucketRebuildInterval/time.Minute))
		if minutes < 1 {
			minutes = 1
		}
		if minutes > 60 {
			minutes = 60
		}
		return time.Duration(minutes) * time.Minute
	}
	return defaultTokenDispatchBucketRebuildInterval
}

func (s *Server) ensureTokenDispatchBuckets() *tokenDispatchBuckets {
	if s == nil {
		return nil
	}
	s.tokenBucketLoopMu.Lock()
	defer s.tokenBucketLoopMu.Unlock()
	if s.tokenBuckets == nil {
		s.tokenBuckets = newTokenDispatchBuckets()
	}
	return s.tokenBuckets
}

func (s *Server) rebuildTokenDispatchBuckets(reason string) {
	if s == nil || s.TokenMgr == nil {
		return
	}
	mgr := s.ensureTokenDispatchBuckets()
	if mgr == nil {
		return
	}
	strategy := "round_robin"
	if s.Config != nil {
		strategy = strings.TrimSpace(s.Config.GetString("token_rotation_strategy", "round_robin"))
	}
	tokens := s.TokenMgr.AvailableTokensForPlatform("leonardo", strategy)
	newBuckets := make(map[tokenDispatchBucketKey][]string)
	for _, info := range tokens {
		tokenID := strings.TrimSpace(toString(info["id"]))
		if tokenID == "" || !s.tokenBaseEligibleForDispatchBucket(info) {
			continue
		}
		for _, modelID := range tokenDispatchRepresentativeModels {
			bucket := imageCreditThresholdBucket(modelID)
			if bucket == "" {
				continue
			}
			for _, tier := range tokenDispatchImageTiers {
				if s.tokenCanRunImageBucketByLocalCredits(info, modelID, tier) {
					key := tokenDispatchBucketKey{Bucket: bucket, Tier: tier}
					newBuckets[key] = append(newBuckets[key], tokenID)
				}
			}
		}
	}
	mgr.mu.Lock()
	mgr.buckets = newBuckets
	mgr.lastRebuild = time.Now()
	for k, idx := range mgr.cursor {
		if size := len(newBuckets[k]); size == 0 {
			delete(mgr.cursor, k)
		} else if idx >= size {
			mgr.cursor[k] = idx % size
		}
	}
	mgr.mu.Unlock()
	log.Printf("[token_bucket] rebuilt reason=%s tokens=%d buckets=%d", strings.TrimSpace(reason), len(tokens), len(newBuckets))
}

func (s *Server) refreshTokenDispatchBucketForToken(tokenID string) {
	tokenID = strings.TrimSpace(tokenID)
	if s == nil || s.TokenMgr == nil || tokenID == "" {
		return
	}
	mgr := s.ensureTokenDispatchBuckets()
	if mgr == nil {
		return
	}
	info := s.TokenMgr.GetByID(tokenID)
	memberships := make(map[tokenDispatchBucketKey]bool)
	if info != nil && s.tokenBaseEligibleForDispatchBucket(info) {
		for _, modelID := range tokenDispatchRepresentativeModels {
			bucket := imageCreditThresholdBucket(modelID)
			if bucket == "" {
				continue
			}
			for _, tier := range tokenDispatchImageTiers {
				if s.tokenCanRunImageBucketByLocalCredits(info, modelID, tier) {
					memberships[tokenDispatchBucketKey{Bucket: bucket, Tier: tier}] = true
				}
			}
		}
	}
	mgr.mu.Lock()
	defer mgr.mu.Unlock()
	for key, ids := range mgr.buckets {
		filtered := ids[:0]
		for _, id := range ids {
			if id != tokenID {
				filtered = append(filtered, id)
			}
		}
		if len(filtered) == 0 {
			delete(mgr.buckets, key)
			delete(mgr.cursor, key)
			continue
		}
		mgr.buckets[key] = filtered
		if mgr.cursor[key] >= len(filtered) {
			mgr.cursor[key] = mgr.cursor[key] % len(filtered)
		}
	}
	for key := range memberships {
		if !containsTokenID(mgr.buckets[key], tokenID) {
			mgr.buckets[key] = append(mgr.buckets[key], tokenID)
		}
	}
	mgr.lastTokenPut[tokenID] = time.Now()
}

func (s *Server) coolDownTokenDispatchBucket(tokenID string, d time.Duration) {
	tokenID = strings.TrimSpace(tokenID)
	if s == nil || tokenID == "" || d <= 0 {
		return
	}
	mgr := s.ensureTokenDispatchBuckets()
	if mgr == nil {
		return
	}
	mgr.mu.Lock()
	mgr.cooldown[tokenID] = time.Now().Add(d)
	mgr.mu.Unlock()
	s.refreshTokenDispatchBucketForToken(tokenID)
}

func (s *Server) nextTokenFromDispatchBucket(modelID, imageSizeTier string, excluded map[string]bool) string {
	bucket := imageCreditThresholdBucket(modelID)
	tier := strings.ToLower(strings.TrimSpace(imageSizeTier))
	if bucket == "" || tier == "" || s == nil || s.TokenMgr == nil {
		return ""
	}
	mgr := s.ensureTokenDispatchBuckets()
	if mgr == nil {
		return ""
	}
	key := tokenDispatchBucketKey{Bucket: bucket, Tier: tier}
	now := time.Now()
	mgr.mu.Lock()
	ids := append([]string(nil), mgr.buckets[key]...)
	start := mgr.cursor[key]
	if len(ids) > 0 {
		mgr.cursor[key] = (start + 1) % len(ids)
	}
	mgr.mu.Unlock()
	if len(ids) == 0 {
		return ""
	}
	for offset := 0; offset < len(ids); offset++ {
		idx := (start + offset) % len(ids)
		tokenID := strings.TrimSpace(ids[idx])
		if tokenID == "" || (excluded != nil && excluded[tokenID]) {
			continue
		}
		mgr.mu.Lock()
		coolUntil := mgr.cooldown[tokenID]
		if !coolUntil.IsZero() && !coolUntil.After(now) {
			delete(mgr.cooldown, tokenID)
			coolUntil = time.Time{}
		}
		mgr.mu.Unlock()
		if !coolUntil.IsZero() && coolUntil.After(now) {
			continue
		}
		info := s.TokenMgr.GetByID(tokenID)
		if info == nil || !s.tokenBaseEligibleForDispatchBucket(info) || !s.tokenCanRunImageBucketByLocalCredits(info, modelID, tier) {
			s.refreshTokenDispatchBucketForToken(tokenID)
			continue
		}
		if !s.tokenCanAcceptSubmission(tokenID) {
			continue
		}
		return tokenID
	}
	return ""
}

func (s *Server) tokenBaseEligibleForDispatchBucket(info map[string]interface{}) bool {
	if info == nil {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(toString(info["platform"])), "leonardo") {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(toString(info["status"])), "active") {
		return false
	}
	if isExpiredTokenInfo(info) || generationJWTWindowPriority(info) > 1 {
		return false
	}
	if strings.TrimSpace(toString(info["value"])) == "" {
		return false
	}
	return true
}

func (s *Server) tokenCanRunImageBucketByLocalCredits(info map[string]interface{}, modelID, imageSizeTier string) bool {
	if imageCreditThresholdBucket(modelID) == "" || strings.TrimSpace(imageSizeTier) == "" {
		return false
	}
	requiredCredits, ok := s.requiredCreditsForImageRequest(modelID, imageSizeTier)
	if !ok {
		return false
	}
	availableCredits, known := tokenCreditsAvailable(info)
	if !known {
		return false
	}
	if availableCredits < s.tokenExhaustionCreditThreshold() {
		return false
	}
	return availableCredits > requiredCredits
}

func containsTokenID(ids []string, target string) bool {
	for _, id := range ids {
		if id == target {
			return true
		}
	}
	return false
}

func (s *Server) getLeonardoSessionFromDispatchBucket(modelID, imageSizeTier string, excluded map[string]bool, usePreparationLease bool) (*leonardo.TokenSession, string, func()) {
	release := func() {}
	if s == nil || s.TokenMgr == nil || imageCreditThresholdBucket(modelID) == "" || strings.TrimSpace(imageSizeTier) == "" {
		return nil, "", func() {}
	}
	for attempts := 0; attempts < 32; attempts++ {
		foundID := s.nextTokenFromDispatchBucket(modelID, imageSizeTier, excluded)
		if foundID == "" {
			return nil, "", func() {}
		}
		release = func() {}
		if usePreparationLease {
			if !s.reserveTokenPreparation(foundID) {
				continue
			}
			release = func(id string) func() { return func() { s.releaseTokenPreparation(id) } }(foundID)
		}
		info := s.TokenMgr.GetByID(foundID)
		if info == nil {
			release()
			release = func() {}
			s.refreshTokenDispatchBucketForToken(foundID)
			continue
		}
		rawToken := strings.TrimSpace(toString(info["value"]))
		if rawToken == "" {
			release()
			release = func() {}
			s.refreshTokenDispatchBucketForToken(foundID)
			continue
		}
		session := s.getOrCreateLeonardoSession(foundID, rawToken)
		if session == nil {
			release()
			release = func() {}
			s.coolDownTokenDispatchBucket(foundID, 2*time.Minute)
			continue
		}
		if err := s.ensureGenerationJWTUsable(foundID, session); err != nil {
			log.Printf("[token_bucket] failed to prepare Leonardo session for %s: %v", foundID, err)
			release()
			release = func() {}
			s.coolDownTokenDispatchBucket(foundID, 2*time.Minute)
			continue
		}
		strategy := "round_robin"
		if s.Config != nil {
			strategy = strings.TrimSpace(s.Config.GetString("token_rotation_strategy", "round_robin"))
		}
		s.TokenMgr.CommitAvailableTokenForPlatform("leonardo", foundID, strategy)
		return session, foundID, release
	}
	return nil, "", func() {}
}
