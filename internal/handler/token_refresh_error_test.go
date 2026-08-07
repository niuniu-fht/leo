package handler

import (
	"errors"
	"testing"

	"leo-go/internal/token"
)

// memoryTokenStore is a minimal in-memory TokenStore used by tests in this
// package. It mirrors the one defined in internal/token for isolation.
type memoryTokenStore struct {
	rows []map[string]interface{}
}

func (s *memoryTokenStore) LoadTokens() ([]map[string]interface{}, error) {
	return append([]map[string]interface{}(nil), s.rows...), nil
}

func (s *memoryTokenStore) ReplaceTokens(tokens []map[string]interface{}) error {
	s.rows = append([]map[string]interface{}(nil), tokens...)
	return nil
}

func TestNoJWTSessionRefreshErrorIsAbnormal(t *testing.T) {
	err := errors.New("token validation failed: ensure JWT: no JWT found in session response, body keys: [session user]")

	if !shouldMarkTokenAbnormalOnRefreshError(err) {
		t.Fatal("expected no JWT session response to be treated as abnormal")
	}
	if !isAbnormalLeonardoTokenError(err) {
		t.Fatal("expected no JWT session response to be an abnormal Leonardo token error")
	}
	if isInvalidLeonardoTokenError(err) {
		t.Fatal("expected no JWT session response not to be treated as invalid")
	}
}

func TestNoJWTSessionRefreshErrorRetriesBeforeQuarantine(t *testing.T) {
	err := errors.New("token validation failed: no JWT found in session response, body keys []")
	mgr := token.NewManager(&memoryTokenStore{})
	info, _, addErr := mgr.Add("cookie-a", "leonardo", "session_token", "", "", "")
	if addErr != nil {
		t.Fatalf("add token: %v", addErr)
	}
	tokenID := toString(info["id"])
	if setErr := mgr.SetAutoRefresh(tokenID, true); setErr != nil {
		t.Fatalf("enable auto refresh: %v", setErr)
	}
	srv := &Server{TokenMgr: mgr}
	first := srv.recordLeonardoRefreshFailure(tokenID, err)
	if first.failCount != 1 {
		t.Fatalf("first failCount = %d, want 1", first.failCount)
	}
	if first.finalStatus != "" {
		t.Fatalf("first finalStatus = %q, want empty", first.finalStatus)
	}
	if got := toString(mgr.GetByID(tokenID)["status"]); got != "active" {
		t.Fatalf("status after first failure = %q, want active", got)
	}
	if !toBool(mgr.GetByID(tokenID)["auto_refresh"]) {
		t.Fatal("auto_refresh should remain enabled for retry")
	}

	result := srv.recordLeonardoRefreshFailure(tokenID, err)
	if result.failCount != 2 {
		t.Fatalf("failCount = %d, want 2", result.failCount)
	}
	if result.finalStatus != "abnormal" {
		t.Fatalf("finalStatus = %q, want abnormal", result.finalStatus)
	}
	updated := mgr.GetByID(tokenID)
	if got := toString(updated["status"]); got != "abnormal" {
		t.Fatalf("status = %q, want abnormal", got)
	}
	if toBool(updated["auto_refresh"]) {
		t.Fatal("auto_refresh should be disabled")
	}
}
