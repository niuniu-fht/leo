package handler

import (
	"errors"
	"testing"
	"time"

	"leo2api/internal/token"
)

type temporaryUnavailableStore struct {
	rows []map[string]interface{}
}

func (s *temporaryUnavailableStore) LoadTokens() ([]map[string]interface{}, error) {
	return append([]map[string]interface{}(nil), s.rows...), nil
}

func (s *temporaryUnavailableStore) ReplaceTokens(rows []map[string]interface{}) error {
	s.rows = append([]map[string]interface{}(nil), rows...)
	return nil
}

func TestRateLimitedRefreshBecomesTemporaryUnavailable(t *testing.T) {
	mgr := token.NewManager(&temporaryUnavailableStore{})
	info, _, err := mgr.Add("cookie-rate-limited", "leonardo", "session_token", "", "", "")
	if err != nil {
		t.Fatalf("add token: %v", err)
	}
	tokenID := toString(info["id"])
	if err := mgr.SetAutoRefresh(tokenID, true); err != nil {
		t.Fatalf("enable auto refresh: %v", err)
	}
	if err := mgr.UpdateExpiry(tokenID, float64(time.Now().Add(-time.Minute).Unix())); err != nil {
		t.Fatalf("expire token: %v", err)
	}

	srv := &Server{TokenMgr: mgr}
	result := srv.recordLeonardoRefreshFailure(tokenID, errors.New("Leonardo rate limited get-session (429)"))
	if result.finalStatus != token.StatusTemporaryUnavailable {
		t.Fatalf("finalStatus = %q, want %q", result.finalStatus, token.StatusTemporaryUnavailable)
	}

	updated := mgr.GetByID(tokenID)
	if got := toString(updated["status"]); got != token.StatusTemporaryUnavailable {
		t.Fatalf("status = %q, want %q", got, token.StatusTemporaryUnavailable)
	}
	if !toBool(updated["auto_refresh"]) {
		t.Fatal("auto_refresh should remain enabled")
	}
	if candidates := mgr.AvailableTokensForPlatform("leonardo", "round_robin"); len(candidates) != 0 {
		t.Fatalf("temporary token should not be selectable, got %d candidates", len(candidates))
	}
}

func TestTemporaryUnavailableRecoversAfterRefreshSuccess(t *testing.T) {
	mgr := token.NewManager(&temporaryUnavailableStore{})
	info, _, err := mgr.Add("cookie-recovered", "leonardo", "session_token", "", "", "")
	if err != nil {
		t.Fatalf("add token: %v", err)
	}
	tokenID := toString(info["id"])
	if err := mgr.SetAutoRefresh(tokenID, true); err != nil {
		t.Fatalf("enable auto refresh: %v", err)
	}
	if err := mgr.SetStatus(tokenID, token.StatusTemporaryUnavailable); err != nil {
		t.Fatalf("set temporary status: %v", err)
	}
	mgr.ReportRefreshFailure(tokenID, "rate limited")

	srv := &Server{TokenMgr: mgr}
	srv.restoreTokenAfterSuccessfulRefresh(tokenID)

	updated := mgr.GetByID(tokenID)
	if got := toString(updated["status"]); got != "active" {
		t.Fatalf("status = %q, want active", got)
	}
	if got := int(toFloat64(updated["refresh_fail_count"])); got != 0 {
		t.Fatalf("refresh_fail_count = %d, want 0", got)
	}
	if !toBool(updated["auto_refresh"]) {
		t.Fatal("auto_refresh should remain enabled")
	}
}

func TestRateLimitedRefreshKeepsUnexpiredTokenActive(t *testing.T) {
	mgr := token.NewManager(&temporaryUnavailableStore{})
	info, _, err := mgr.Add("cookie-still-valid", "leonardo", "session_token", "", "", "")
	if err != nil {
		t.Fatalf("add token: %v", err)
	}
	tokenID := toString(info["id"])
	if err := mgr.SetAutoRefresh(tokenID, true); err != nil {
		t.Fatalf("enable auto refresh: %v", err)
	}
	if err := mgr.UpdateExpiry(tokenID, float64(time.Now().Add(30*time.Minute).Unix())); err != nil {
		t.Fatalf("set expiry: %v", err)
	}

	srv := &Server{TokenMgr: mgr}
	result := srv.recordLeonardoRefreshFailure(tokenID, errors.New("Leonardo rate limited get-session (429)"))
	if result.finalStatus != "" {
		t.Fatalf("finalStatus = %q, want empty", result.finalStatus)
	}
	updated := mgr.GetByID(tokenID)
	if got := toString(updated["status"]); got != "active" {
		t.Fatalf("status = %q, want active", got)
	}
	if candidates := mgr.AvailableTokensForPlatform("leonardo", "round_robin"); len(candidates) != 1 {
		t.Fatalf("unexpired token should remain selectable, got %d candidates", len(candidates))
	}
}
