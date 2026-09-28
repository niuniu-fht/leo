package handler

import (
	"log"
	"strings"
	"time"

	"leo2api/internal/token"
)

const (
	// concurrentLimitHitWindow is the rolling window in which upstream
	// concurrent-limit rejections are counted per token.
	concurrentLimitHitWindow = time.Hour
	// concurrentLimitHitThreshold is how many rejections inside the window
	// mark the token rate_limited. Transient rejections (the 2-minute
	// dispatch cooldown already backs them off) stay below this; only
	// accounts stuck at "(0)" on Leonardo's side reach it.
	concurrentLimitHitThreshold = 5
)

// recordTokenConcurrentLimitHit counts an upstream "maximum number of
// concurrent ... requests (0)" rejection for the token. Accounts that keep
// hitting the limit inside the window are marked rate_limited: Leonardo's
// per-account concurrency counter is stuck (verified: idle queue, credits
// intact, rejected across models/IPs), so the account leaves scheduling and
// credit totals until manually re-enabled.
func (s *Server) recordTokenConcurrentLimitHit(tokenID string) {
	tokenID = strings.TrimSpace(tokenID)
	if s == nil || tokenID == "" || s.TokenMgr == nil {
		return
	}
	s.concurrentLimitMu.Lock()
	if s.concurrentLimitHits == nil {
		s.concurrentLimitHits = make(map[string][]time.Time)
	}
	now := time.Now()
	hits := s.concurrentLimitHits[tokenID]
	kept := make([]time.Time, 0, len(hits)+1)
	for _, ts := range hits {
		if now.Sub(ts) <= concurrentLimitHitWindow {
			kept = append(kept, ts)
		}
	}
	kept = append(kept, now)
	s.concurrentLimitHits[tokenID] = kept
	reached := len(kept) >= concurrentLimitHitThreshold
	if !reached {
		s.concurrentLimitMu.Unlock()
		return
	}
	delete(s.concurrentLimitHits, tokenID)
	s.concurrentLimitMu.Unlock()

	info := s.TokenMgr.GetByID(tokenID)
	if info == nil {
		return
	}
	status := strings.ToLower(strings.TrimSpace(toString(info["status"])))
	if status != "active" && status != token.StatusTemporaryUnavailable && status != "pending" {
		return
	}
	if err := s.TokenMgr.SetStatus(tokenID, token.StatusRateLimited); err != nil {
		log.Printf("[token] failed to mark token %s rate_limited: %v", tokenID, err)
		return
	}
	s.refreshTokenDispatchBucketForToken(tokenID)
	log.Printf("[token] marked token %s rate_limited after %d concurrent-limit rejections in %s (manual re-enable required)", tokenID, len(kept), concurrentLimitHitWindow)
}

// resetTokenConcurrentLimitHits clears the rejection counter after a
// successful submission, so a healthy account never accumulates toward the
// threshold from occasional transient rejections.
func (s *Server) resetTokenConcurrentLimitHits(tokenID string) {
	tokenID = strings.TrimSpace(tokenID)
	if s == nil || tokenID == "" {
		return
	}
	s.concurrentLimitMu.Lock()
	delete(s.concurrentLimitHits, tokenID)
	s.concurrentLimitMu.Unlock()
}
