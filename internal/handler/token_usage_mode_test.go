package handler

import (
	"testing"
	"time"

	"leo2api/internal/config"
	"leo2api/internal/token"
)

func TestTokenMaxRunningTasksDefaultsToTwo(t *testing.T) {
	cfg := config.Global()
	original := cfg.GetAll()
	cfg.SetAll(map[string]interface{}{})
	t.Cleanup(func() {
		cfg.SetAll(original)
	})

	srv := &Server{Config: cfg}
	if got := srv.tokenMaxRunningTasks(); got != defaultTokenMaxRunningTasks {
		t.Fatalf("tokenMaxRunningTasks() = %d, want %d", got, defaultTokenMaxRunningTasks)
	}
	if got := srv.tokenExhaustionCreditThreshold(); got != float64(defaultTokenExhaustionCreditThreshold) {
		t.Fatalf("tokenExhaustionCreditThreshold() = %.0f, want %.0f", got, float64(defaultTokenExhaustionCreditThreshold))
	}
}

func TestTokenExhaustionCreditThresholdUsesConfiguredValue(t *testing.T) {
	cfg := config.Global()
	original := cfg.GetAll()
	cfg.SetAll(map[string]interface{}{"token_exhaustion_credit_threshold": 25})
	t.Cleanup(func() {
		cfg.SetAll(original)
	})

	srv := &Server{Config: cfg}
	if got := srv.tokenExhaustionCreditThreshold(); got != 25 {
		t.Fatalf("tokenExhaustionCreditThreshold() = %.0f, want 25", got)
	}

	cfg.Set("token_exhaustion_credit_threshold", -5)
	if got := srv.tokenExhaustionCreditThreshold(); got != 0 {
		t.Fatalf("negative tokenExhaustionCreditThreshold() = %.0f, want 0", got)
	}
}

func TestTokenMaxRunningTasksUsesConfiguredRange(t *testing.T) {
	cfg := config.Global()
	original := cfg.GetAll()
	t.Cleanup(func() {
		cfg.SetAll(original)
	})

	srv := &Server{Config: cfg}
	cfg.SetAll(map[string]interface{}{"token_max_running_tasks": 1})
	if got := srv.tokenMaxRunningTasks(); got != 1 {
		t.Fatalf("tokenMaxRunningTasks() = %d, want 1", got)
	}
	cfg.SetAll(map[string]interface{}{"token_max_running_tasks": 7})
	if got := srv.tokenMaxRunningTasks(); got != 7 {
		t.Fatalf("tokenMaxRunningTasks() = %d, want 7", got)
	}
	cfg.SetAll(map[string]interface{}{"token_max_running_tasks": 0})
	if got := srv.tokenMaxRunningTasks(); got != 1 {
		t.Fatalf("tokenMaxRunningTasks() = %d, want 1", got)
	}
	cfg.SetAll(map[string]interface{}{"token_max_running_tasks": 11})
	if got := srv.tokenMaxRunningTasks(); got != 10 {
		t.Fatalf("tokenMaxRunningTasks() = %d, want 10", got)
	}
}

func TestTokenCanRunSora2UsesModeThreshold(t *testing.T) {
	cfg := config.Global()
	original := cfg.GetAll()
	t.Cleanup(func() {
		cfg.SetAll(original)
	})

	mgr := token.NewManager(nil)
	info, _, err := mgr.Add("sora2-threshold-token", "leonardo", "session_token", "", "", "test")
	if err != nil {
		t.Fatalf("add token: %v", err)
	}
	tokenID := toString(info["id"])
	if err := mgr.UpdateCredits(tokenID, 2000, 7200); err != nil {
		t.Fatalf("update credits: %v", err)
	}

	cfg.SetAll(map[string]interface{}{})
	srv := &Server{Config: cfg, TokenMgr: mgr}
	if !srv.tokenCanRunModelByCredits(mgr.GetByID(tokenID), "sora2", "", false) {
		t.Fatalf("expected 2000-credit token to be accepted for sora2")
	}
	if status := toString(mgr.GetByID(tokenID)["status"]); status != "active" {
		t.Fatalf("status = %q, want active", status)
	}
}

func TestTokenCanRunLowerCostSeedanceModelsByCredits(t *testing.T) {
	cfg := config.Global()
	original := cfg.GetAll()
	cfg.SetAll(map[string]interface{}{})
	t.Cleanup(func() {
		cfg.SetAll(original)
	})

	mgr := token.NewManager(nil)
	info, _, err := mgr.Add("low-credit-token", "leonardo", "session_token", "", "", "test")
	if err != nil {
		t.Fatalf("add token: %v", err)
	}
	tokenID := toString(info["id"])
	if err := mgr.UpdateCredits(tokenID, 1300, 7200); err != nil {
		t.Fatalf("update credits: %v", err)
	}

	cfg.Set("token_exhaustion_credit_threshold", videoKo3ExhaustionCredits)
	srv := &Server{Config: cfg, TokenMgr: mgr}
	if !srv.tokenCanRunModelByCredits(mgr.GetByID(tokenID), "video-2.0-mini-480p", "", false) {
		t.Fatalf("expected 1300-credit token to run mini 480p")
	}
	if srv.tokenCanRunModelByCredits(mgr.GetByID(tokenID), "video-2.0-fast-480p", "", false) {
		t.Fatalf("expected 1300-credit token to be skipped for fast 480p")
	}
	if status := toString(mgr.GetByID(tokenID)["status"]); status != "active" {
		t.Fatalf("status = %q, want active", status)
	}

	if err := mgr.UpdateCredits(tokenID, 1100, 7200); err != nil {
		t.Fatalf("update credits below minimum: %v", err)
	}
	if srv.tokenCanRunModelByCredits(mgr.GetByID(tokenID), "video-2.0-mini-480p", "", false) {
		t.Fatalf("expected 1100-credit token to be rejected")
	}
	if status := toString(mgr.GetByID(tokenID)["status"]); status != "exhausted" {
		t.Fatalf("status = %q, want exhausted", status)
	}
}

func TestFailedGenerationRefundRestoresExhaustedToken(t *testing.T) {
	mgr := token.NewManager(nil)
	info, _, err := mgr.Add("failed-generation-refund", "leonardo", "session_token", "", "", "test")
	if err != nil {
		t.Fatalf("add token: %v", err)
	}
	tokenID := toString(info["id"])
	if err := mgr.SetAutoRefresh(tokenID, true); err != nil {
		t.Fatalf("enable auto refresh: %v", err)
	}
	if err := mgr.UpdateCredits(tokenID, 4300, 7200); err != nil {
		t.Fatalf("update credits: %v", err)
	}

	cfg := config.New()
	cfg.Set("token_exhaustion_credit_threshold", videoKo3ExhaustionCredits)
	srv := &Server{Config: cfg, TokenMgr: mgr}
	srv.applyTokenCreditCost(tokenID, 3400)
	if status := toString(mgr.GetByID(tokenID)["status"]); status != "exhausted" {
		t.Fatalf("status after local deduction = %q, want exhausted", status)
	}

	srv.applyFailedGenerationCredits(tokenID, 4300, 7200, "generation-refunded")
	updated := mgr.GetByID(tokenID)
	if status := toString(updated["status"]); status != "active" {
		t.Fatalf("status after refund = %q, want active", status)
	}
	if credits := toFloat64(updated["credits"]); credits != 4300 {
		t.Fatalf("credits after refund = %.0f, want 4300", credits)
	}
	if !toBool(updated["auto_refresh"]) {
		t.Fatal("auto_refresh should be restored after failed-generation refund")
	}
}

func TestFailedGenerationRefundKeepsActuallyExhaustedToken(t *testing.T) {
	mgr := token.NewManager(nil)
	info, _, err := mgr.Add("failed-generation-exhausted", "leonardo", "session_token", "", "", "test")
	if err != nil {
		t.Fatalf("add token: %v", err)
	}
	tokenID := toString(info["id"])
	if err := mgr.SetStatus(tokenID, "exhausted"); err != nil {
		t.Fatalf("set exhausted status: %v", err)
	}

	cfg := config.New()
	cfg.Set("token_exhaustion_credit_threshold", videoKo3ExhaustionCredits)
	srv := &Server{Config: cfg, TokenMgr: mgr}
	srv.applyFailedGenerationCredits(tokenID, 900, 7200, "generation-still-exhausted")
	updated := mgr.GetByID(tokenID)
	if status := toString(updated["status"]); status != "exhausted" {
		t.Fatalf("status after low refund balance = %q, want exhausted", status)
	}
	if credits := toFloat64(updated["credits"]); credits != 900 {
		t.Fatalf("credits after low refund balance = %.0f, want 900", credits)
	}
}

func TestGenerationTokenCandidatesPreferLowestSufficientCredits(t *testing.T) {
	cfg := config.Global()
	original := cfg.GetAll()
	cfg.SetAll(map[string]interface{}{})
	t.Cleanup(func() {
		cfg.SetAll(original)
	})

	srv := &Server{Config: cfg}
	candidates := []map[string]interface{}{
		{"id": "high", "credits_available": 5000},
		{"id": "tight", "credits_available": 1300},
		{"id": "middle", "credits_available": 2400},
	}

	got := srv.generationTokenCandidates(candidates, nil, "video-2.0-mini-480p", "", false, "round_robin")
	if len(got) != 3 {
		t.Fatalf("candidate count = %d, want 3", len(got))
	}
	if first := toString(got[0]["id"]); first != "tight" {
		t.Fatalf("first candidate = %q, want tight", first)
	}
}

func TestGenerationTokenCandidatesFromStartPreservesImportOrder(t *testing.T) {
	srv := &Server{}
	candidates := []map[string]interface{}{
		{"id": "old-high-balance", "credits_available": 9000},
		{"id": "new-tight-balance", "credits_available": 1356},
	}

	got := srv.generationTokenCandidates(candidates, nil, "video-2.0-mini-480p", "", false, "round_robin_from_start")
	if len(got) != 2 {
		t.Fatalf("candidate count = %d, want 2", len(got))
	}
	if first := toString(got[0]["id"]); first != "old-high-balance" {
		t.Fatalf("first candidate = %q, want old-high-balance", first)
	}
}

func TestGenerationTokenCandidatesFromStartPreservesManagerImportOrder(t *testing.T) {
	mgr := token.NewManager(nil)
	oldInfo, _, err := mgr.Add("old-imported-token", "leonardo", "session_token", "", "", "test")
	if err != nil {
		t.Fatalf("add old token: %v", err)
	}
	newInfo, _, err := mgr.Add("new-imported-token", "leonardo", "session_token", "", "", "test")
	if err != nil {
		t.Fatalf("add new token: %v", err)
	}
	oldID := toString(oldInfo["id"])
	newID := toString(newInfo["id"])
	if err := mgr.UpdateCredits(oldID, 9000, 9000); err != nil {
		t.Fatalf("update old token credits: %v", err)
	}
	if err := mgr.UpdateCredits(newID, 1356, 9000); err != nil {
		t.Fatalf("update new token credits: %v", err)
	}

	srv := &Server{TokenMgr: mgr}
	candidates := mgr.AvailableTokensForPlatform("leonardo", "round_robin_from_start")
	got := srv.generationTokenCandidates(candidates, nil, "video-2.0-mini-480p", "", false, "round_robin_from_start")
	if len(got) != 2 {
		t.Fatalf("candidate count = %d, want 2", len(got))
	}
	if first := toString(got[0]["id"]); first != oldID {
		t.Fatalf("first candidate = %q, want oldest imported token %q", first, oldID)
	}
}

func TestGenerationTokenCandidatesFromStartDoesNotUseTokenIDAsTieBreaker(t *testing.T) {
	srv := &Server{}
	candidates := []map[string]interface{}{
		{"id": "z-old-token", "credits_available": 9000},
		{"id": "a-new-token", "credits_available": 9000},
	}

	got := srv.generationTokenCandidates(candidates, nil, "video-2.0", "", false, "round_robin_from_start")
	if len(got) != 2 {
		t.Fatalf("candidate count = %d, want 2", len(got))
	}
	if first := toString(got[0]["id"]); first != "z-old-token" {
		t.Fatalf("first candidate = %q, want z-old-token", first)
	}
}

func TestGenerationTokenCandidatesFromStartSkipsIneligibleTokenWithoutReordering(t *testing.T) {
	srv := &Server{}
	candidates := []map[string]interface{}{
		{"id": "old-short-jwt", "credits_available": 9000, "expires_at": float64(time.Now().Add(4 * time.Minute).Unix())},
		{"id": "old-insufficient", "credits_available": 1100},
		{"id": "old-excluded", "credits_available": 9000},
		{"id": "next-eligible", "credits_available": 9000},
		{"id": "newer-eligible", "credits_available": 1356},
	}

	got := srv.generationTokenCandidates(candidates, map[string]bool{"old-excluded": true}, "video-2.0-mini-480p", "", false, "round_robin_from_start")
	if len(got) != 2 {
		t.Fatalf("candidate count = %d, want 2", len(got))
	}
	if first := toString(got[0]["id"]); first != "next-eligible" {
		t.Fatalf("first candidate = %q, want next-eligible", first)
	}
}

func TestImageTokenCreditThresholds(t *testing.T) {
	cfg := config.Global()
	original := cfg.GetAll()
	cfg.SetAll(map[string]interface{}{})
	t.Cleanup(func() {
		cfg.SetAll(original)
	})

	srv := &Server{Config: cfg}
	low := map[string]interface{}{"id": "low", "credits_available": float64(8)}
	mid := map[string]interface{}{"id": "mid", "credits_available": float64(31)}
	high := map[string]interface{}{"id": "high", "credits_available": float64(251)}

	if srv.tokenCanRunModelByCredits(low, "gpt-image-2", "1k", false) {
		t.Fatalf("expected equal 1k threshold credits to be skipped")
	}
	if !srv.tokenCanRunModelByCredits(mid, "gpt-image-2", "2k", false) {
		t.Fatalf("expected credits above gpt-image-2 2k threshold to pass")
	}
	if srv.tokenCanRunModelByCredits(mid, "gpt-image-2", "4k", false) {
		t.Fatalf("expected credits below gpt-image-2 4k threshold to be skipped")
	}
	if !srv.tokenCanRunModelByCredits(high, "gpt-image-gemini-3-pro-image", "4k", false) {
		t.Fatalf("expected credits above pro 4k threshold to pass")
	}
}

func TestImageTokenCreditCandidatesPreferSmallestSurplus(t *testing.T) {
	cfg := config.Global()
	original := cfg.GetAll()
	cfg.SetAll(map[string]interface{}{"token_max_running_tasks": 5})
	t.Cleanup(func() {
		cfg.SetAll(original)
	})

	srv := &Server{Config: cfg}
	candidates := []map[string]interface{}{
		{"id": "too-low", "credits_available": float64(120)},
		{"id": "large", "credits_available": float64(300)},
		{"id": "fit", "credits_available": float64(161)},
	}
	got := srv.generationTokenCandidates(candidates, nil, "banana2", "4k", false, "round_robin")
	if len(got) != 2 {
		t.Fatalf("expected 2 eligible candidates, got %d", len(got))
	}
	if toString(got[0]["id"]) != "fit" {
		t.Fatalf("expected smallest eligible surplus first, got %s", toString(got[0]["id"]))
	}
}
