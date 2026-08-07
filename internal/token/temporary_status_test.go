package token

import (
	"testing"
	"time"
)

func TestLoadMigratesLegacyRateLimitedAbnormalToken(t *testing.T) {
	store := &memoryTokenStore{rows: []map[string]interface{}{
		{
			"id":                  GenerateTokenID("cookie-rate-limited"),
			"value":               "cookie-rate-limited",
			"platform":            "leonardo",
			"token_type":          "session_token",
			"status":              "abnormal",
			"refresh_fail_count":  2,
			"refresh_fail_reason": "rate limited",
			"auto_refresh":        false,
			"expires_at":          float64(time.Now().Add(-time.Minute).Unix()),
		},
	}}

	mgr := NewManager(store)
	updated := mgr.GetByID(GenerateTokenID("cookie-rate-limited"))
	if got := updated["status"]; got != StatusTemporaryUnavailable {
		t.Fatalf("status = %v, want %s", got, StatusTemporaryUnavailable)
	}
	if got, _ := updated["auto_refresh"].(bool); !got {
		t.Fatal("auto_refresh should be re-enabled")
	}
	if store.saves == 0 {
		t.Fatal("migration should be persisted")
	}
}

func TestLoadDoesNotMigrateOtherAbnormalToken(t *testing.T) {
	store := &memoryTokenStore{rows: []map[string]interface{}{
		{
			"id":                  GenerateTokenID("cookie-no-jwt"),
			"value":               "cookie-no-jwt",
			"platform":            "leonardo",
			"token_type":          "session_token",
			"status":              "abnormal",
			"refresh_fail_count":  1,
			"refresh_fail_reason": "no JWT found",
			"auto_refresh":        false,
		},
	}}

	mgr := NewManager(store)
	updated := mgr.GetByID(GenerateTokenID("cookie-no-jwt"))
	if got := updated["status"]; got != "abnormal" {
		t.Fatalf("status = %v, want abnormal", got)
	}
	if got, _ := updated["auto_refresh"].(bool); got {
		t.Fatal("auto_refresh should remain disabled")
	}
}

func TestLoadRestoresUnexpiredLegacyRateLimitedTokenToActive(t *testing.T) {
	store := &memoryTokenStore{rows: []map[string]interface{}{
		{
			"id":                  GenerateTokenID("cookie-rate-limited-valid"),
			"value":               "cookie-rate-limited-valid",
			"platform":            "leonardo",
			"token_type":          "session_token",
			"status":              "abnormal",
			"refresh_fail_count":  2,
			"refresh_fail_reason": "rate limited",
			"auto_refresh":        false,
			"expires_at":          float64(time.Now().Add(30 * time.Minute).Unix()),
		},
	}}

	mgr := NewManager(store)
	updated := mgr.GetByID(GenerateTokenID("cookie-rate-limited-valid"))
	if got := updated["status"]; got != "active" {
		t.Fatalf("status = %v, want active", got)
	}
	if got, _ := updated["auto_refresh"].(bool); !got {
		t.Fatal("auto_refresh should be re-enabled")
	}
	if candidates := mgr.AvailableTokensForPlatform("leonardo", "round_robin"); len(candidates) != 1 {
		t.Fatalf("unexpired token should be selectable, got %d candidates", len(candidates))
	}
}
