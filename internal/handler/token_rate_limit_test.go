package handler

import (
	"strings"
	"testing"

	"leo2api/internal/token"
)

func TestConcurrentLimitMarksRateLimited(t *testing.T) {
	s := &Server{TokenMgr: token.NewManager(nil)}
	info, _, err := s.TokenMgr.Add("cookie-x", "leonardo", "session_token", "acct", "acct@test.com", "test")
	if err != nil {
		t.Fatalf("add token: %v", err)
	}
	id, _ := info["id"].(string)
	if id == "" {
		t.Fatal("missing token id")
	}

	for i := 0; i < concurrentLimitHitThreshold-1; i++ {
		s.recordTokenConcurrentLimitHit(id)
	}
	if got := toString(s.TokenMgr.GetByID(id)["status"]); got != "active" {
		t.Fatalf("below threshold should stay active, got %q", got)
	}

	s.recordTokenConcurrentLimitHit(id)
	if got := toString(s.TokenMgr.GetByID(id)["status"]); got != token.StatusRateLimited {
		t.Fatalf("threshold reached should mark rate_limited, got %q", got)
	}

	// already marked accounts are not re-marked and stay rate_limited
	s.recordTokenConcurrentLimitHit(id)
	if got := toString(s.TokenMgr.GetByID(id)["status"]); got != token.StatusRateLimited {
		t.Fatalf("rate_limited must persist, got %q", got)
	}
}

func TestConcurrentLimitResetOnSuccess(t *testing.T) {
	s := &Server{TokenMgr: token.NewManager(nil)}
	info, _, _ := s.TokenMgr.Add("cookie-y", "leonardo", "session_token", "acct", "acct2@test.com", "test")
	id, _ := info["id"].(string)

	for i := 0; i < concurrentLimitHitThreshold-1; i++ {
		s.recordTokenConcurrentLimitHit(id)
	}
	s.resetTokenConcurrentLimitHits(id)
	// a fresh counter needs a full threshold again
	for i := 0; i < concurrentLimitHitThreshold-1; i++ {
		s.recordTokenConcurrentLimitHit(id)
	}
	if got := toString(s.TokenMgr.GetByID(id)["status"]); got != "active" {
		t.Fatalf("reset counter should not have reached threshold, got %q", got)
	}
}

func TestRateLimitedExcludedFromScheduling(t *testing.T) {
	// non-active statuses never pass the selectability gate
	m := token.NewManager(nil)
	info, _, _ := m.Add("cookie-z", "leonardo", "session_token", "acct", "acct3@test.com", "test")
	id, _ := info["id"].(string)
	if err := m.SetStatus(id, token.StatusRateLimited); err != nil {
		t.Fatalf("set status: %v", err)
	}
	for _, info := range m.AvailableTokensForPlatform("leonardo", "round_robin") {
		if strings.EqualFold(toString(info["id"]), id) {
			t.Fatal("rate_limited token must not be selectable")
		}
	}
}

func TestUserBlockedMarksAbnormal(t *testing.T) {
	s := &Server{TokenMgr: token.NewManager(nil)}
	info, _, _ := s.TokenMgr.Add("cookie-b", "leonardo", "session_token", "acct", "blocked@test.com", "test")
	id, _ := info["id"].(string)
	s.markTokenBlocked(id)
	got := toString(s.TokenMgr.GetByID(id)["status"])
	if got != "abnormal" {
		t.Fatalf("blocked token should be abnormal, got %q", got)
	}
	if enabled, _ := s.TokenMgr.GetByID(id)["auto_refresh"].(bool); enabled {
		t.Fatal("blocked token auto-refresh should be disabled")
	}
}

func TestIsAccountBannedError(t *testing.T) {
	if isAccountBannedError(nil) {
		t.Fatal("nil must not match")
	}
	blocked := &upstreamDetailErrStub{msg: "image generate error: User is blocked", detail: ""}
	if !isAccountBannedError(blocked) {
		t.Fatal("blocked message must match")
	}
	suspended := &upstreamDetailErrStub{msg: "upload init failed: upload init error: You are suspended for violating our user agreement.", detail: ""}
	if !isAccountBannedError(suspended) {
		t.Fatal("suspension message must match")
	}
	inBody := &upstreamDetailErrStub{
		msg:    "image generate error: An error occurred.",
		detail: `{"errors":[{"extensions":{"code":"BadRequestException","details":{"errors":[{"message":"User is blocked"}]}}}]}`,
	}
	if !isAccountBannedError(inBody) {
		t.Fatal("upstream detail must match")
	}
	plain := &upstreamDetailErrStub{msg: "image generate error: An error occurred.", detail: ""}
	if isAccountBannedError(plain) {
		t.Fatal("plain error must not match")
	}
	if code, ok := explicitStatusCodeFromGenerationError(errString("image generate error: You are suspended for violating our user agreement.")); !ok || code != 502 {
		t.Fatalf("banned error should map to 502, got %d ok=%v", code, ok)
	}
}
