package handler

import (
	"log"
	"strings"
	"time"
)

type tokenCleanupResult struct {
	Status              string
	MatchedCount        int
	DeletedCount        int
	SkippedRunningCount int
	FailedCount         int
	DeletedIDs          []string
	SkippedRunningIDs   []string
}

// StartExhaustedTokenCleanupLoop starts the background cleanup for exhausted tokens.
func (s *Server) StartExhaustedTokenCleanupLoop() {
	if s == nil || s.TokenMgr == nil {
		return
	}

	s.tokenCleanupMu.Lock()
	if s.tokenCleanupLoopStarted {
		s.tokenCleanupMu.Unlock()
		return
	}
	s.tokenCleanupLoopStarted = true
	s.tokenCleanupMu.Unlock()

	go func() {
		log.Printf("[token] exhausted-token auto-cleanup loop started")
		var nextRun time.Time
		var lastInterval time.Duration
		for {
			if !s.exhaustedTokenAutoCleanupEnabled() {
				nextRun = time.Time{}
				lastInterval = 0
				time.Sleep(time.Minute)
				continue
			}

			interval := s.exhaustedTokenAutoCleanupInterval()
			now := time.Now()
			if nextRun.IsZero() || interval != lastInterval {
				nextRun = now.Add(interval)
				lastInterval = interval
			}
			if now.Before(nextRun) {
				sleepFor := time.Until(nextRun)
				if sleepFor > time.Minute {
					sleepFor = time.Minute
				}
				time.Sleep(sleepFor)
				continue
			}

			result := s.cleanupTokensByStatus("exhausted")
			if result.MatchedCount > 0 || result.FailedCount > 0 {
				log.Printf("[token] exhausted-token auto-cleanup completed: matched=%d deleted=%d skipped_running=%d failed=%d",
					result.MatchedCount, result.DeletedCount, result.SkippedRunningCount, result.FailedCount)
			}
			nextRun = time.Now().Add(interval)
		}
	}()
}

func (s *Server) cleanupTokensByStatus(status string) tokenCleanupResult {
	status = strings.ToLower(strings.TrimSpace(status))
	result := tokenCleanupResult{Status: status}
	if s == nil || s.TokenMgr == nil || !isTokenCleanupStatus(status) {
		return result
	}

	s.tokenCleanupMu.Lock()
	if s.tokenCleanupRunning {
		s.tokenCleanupMu.Unlock()
		return result
	}
	s.tokenCleanupRunning = true
	s.tokenCleanupMu.Unlock()
	defer func() {
		s.tokenCleanupMu.Lock()
		s.tokenCleanupRunning = false
		s.tokenCleanupMu.Unlock()
	}()
	s.tokenLifecycleMu.Lock()
	defer s.tokenLifecycleMu.Unlock()

	now := float64(time.Now().Unix())
	tokens := s.TokenMgr.ListFull()
	ids := make([]string, 0, len(tokens))
	for _, info := range tokens {
		tokenID := strings.TrimSpace(toString(info["id"]))
		if tokenID == "" {
			continue
		}
		tokenStatus := strings.ToLower(strings.TrimSpace(toString(info["status"])))
		expiresAt := toFloat64(info["expires_at"])

		matched := false
		switch status {
		case "invalid":
			if tokenStatus == "invalid" || (expiresAt > 0 && now >= expiresAt) {
				matched = true
			}
		case "exhausted":
			if tokenStatus == "exhausted" {
				matched = true
			}
		case "abnormal":
			if tokenStatus == "abnormal" {
				matched = true
			}
		}
		if !matched {
			continue
		}
		result.MatchedCount++
		if s.tokenHasActiveGenerationWork(tokenID) {
			result.SkippedRunningCount++
			result.SkippedRunningIDs = append(result.SkippedRunningIDs, tokenID)
			continue
		}
		ids = append(ids, tokenID)
	}

	deletedIDs, failed := s.TokenMgr.RemoveMany(ids)
	result.DeletedCount = len(deletedIDs)
	result.FailedCount = failed
	result.DeletedIDs = deletedIDs
	return result
}

// tokenHasActiveGenerationWork protects tokens that are being prepared, have
// an in-progress generation, or are waiting for a failed-generation credit
// refund. The preparation lease closes the gap before the request log is written.
func (s *Server) tokenHasActiveGenerationWork(tokenID string) bool {
	return s.tokenHasPreparationLease(tokenID) ||
		s.tokenRunningGenerationCount(tokenID) > 0 ||
		s.tokenHasPendingSettlement(tokenID)
}

func isTokenCleanupStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "invalid", "exhausted", "abnormal":
		return true
	default:
		return false
	}
}

func (s *Server) exhaustedTokenAutoCleanupEnabled() bool {
	if s == nil || s.Config == nil {
		return false
	}
	return s.Config.GetBool("exhausted_token_auto_cleanup_enabled", false)
}

func (s *Server) exhaustedTokenAutoCleanupInterval() time.Duration {
	hours := 24
	if s != nil && s.Config != nil {
		hours = s.Config.GetInt("exhausted_token_auto_cleanup_interval_hours", 24)
	}
	if hours < 1 {
		hours = 1
	}
	if hours > 8760 {
		hours = 8760
	}
	return time.Duration(hours) * time.Hour
}
