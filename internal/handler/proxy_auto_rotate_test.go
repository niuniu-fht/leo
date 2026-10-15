package handler

import (
	"strings"
	"testing"
	"time"

	"leo2api/internal/config"
)

func TestRotateProxySidReplacesExisting(t *testing.T) {
	newURL, err := rotateProxySid("http://leo2imguse-zone-custom-region-US-sid-t137a:secret@us.rrp.bestgo.work:10000")
	if err != nil {
		t.Fatalf("rotate failed: %v", err)
	}
	if !strings.Contains(newURL, "-sid-auto") {
		t.Fatalf("expected new auto sid in %q", newURL)
	}
	if strings.Contains(newURL, "sid-t137a") {
		t.Fatalf("old sid should be replaced: %q", newURL)
	}
	if !strings.Contains(newURL, "secret") {
		t.Fatalf("password must be preserved: %q", newURL)
	}
}

func TestRotateProxySidAppendsWhenMissing(t *testing.T) {
	newURL, err := rotateProxySid("http://leo2imguse-zone-custom-region-US:secret@gw:10000")
	if err != nil {
		t.Fatalf("rotate failed: %v", err)
	}
	if !strings.Contains(newURL, "region-US-sid-auto") {
		t.Fatalf("expected appended sid: %q", newURL)
	}
}

func TestRotateProxySidKeepsTTLTail(t *testing.T) {
	newURL, err := rotateProxySid("http://u-region-us-sid-old-t-60:pw@gw:1")
	if err != nil {
		t.Fatalf("rotate failed: %v", err)
	}
	if !strings.Contains(newURL, "-sid-auto") || !strings.Contains(newURL, "-t-60") {
		t.Fatalf("sid replaced but ttl tail lost: %q", newURL)
	}
}

func TestRotateProxySidRefusesNonVendorUsername(t *testing.T) {
	if _, err := rotateProxySid("http://plainuser:pw@gw:1"); err == nil {
		t.Fatal("expected refusal for username without -region-")
	}
	if _, err := rotateProxySid(""); err == nil {
		t.Fatal("expected refusal for empty proxy")
	}
}

func TestIsRequestDeniedErrorClassification(t *testing.T) {
	denied := "image generate error: Request denied (upstream_body: {\"details\":{\"message\":\"Access denied.\",\"statusCode\":403}})"
	if !isRequestDeniedError(&upstreamDetailErrStub{msg: denied}) {
		t.Fatal("Request denied should match")
	}
	blocked := "image generate error: User is blocked"
	if isRequestDeniedError(&upstreamDetailErrStub{msg: blocked}) {
		t.Fatal("User is blocked must NOT match (it is an account ban)")
	}
	// success recorder must be nil-safe
	s := &Server{Config: config.New()}
	s.recordRequestSuccess()
}

func TestRecordDeniedTriggersRotationOnThreshold(t *testing.T) {
	cfg := config.New()
	cfg.Set("use_proxy", true)
	cfg.Set("proxy", "http://u-region-us:pw@gw.test:1000")
	s := &Server{Config: cfg}

	proxyRotateState.mu.Lock()
	proxyRotateState.deniedHits = nil
	proxyRotateState.successHits = nil
	proxyRotateState.lastRotateAt = time.Time{}
	proxyRotateState.mu.Unlock()

	for i := 0; i < requestDeniedThreshold; i++ {
		s.recordRequestDeniedAndMaybeRotate()
	}
	// rotation consumed the hits and stamped the cooldown
	proxyRotateState.mu.Lock()
	hits := len(proxyRotateState.deniedHits)
	last := proxyRotateState.lastRotateAt
	proxyRotateState.mu.Unlock()
	if hits != 0 {
		t.Fatalf("hits should be cleared after rotation, got %d", hits)
	}
	if last.IsZero() {
		t.Fatal("lastRotateAt should be set")
	}
	if got := cfg.GetString("proxy", ""); !strings.Contains(got, "-sid-auto") {
		t.Fatalf("config proxy should carry new sid, got %q", got)
	}
}

func TestSuccessesDoNotMaskDenialDominance(t *testing.T) {
	cfg := config.New()
	cfg.Set("use_proxy", true)
	cfg.Set("proxy", "http://u-region-us:pw@gw.test:1000")
	s := &Server{Config: cfg}
	proxyRotateState.mu.Lock()
	proxyRotateState.deniedHits = nil
	proxyRotateState.successHits = nil
	proxyRotateState.lastRotateAt = time.Time{}
	proxyRotateState.mu.Unlock()

	// 5 successes interleaved with 6 denials: denials dominate -> rotate.
	for i := 0; i < 5; i++ {
		s.recordRequestSuccess()
		s.recordRequestDeniedAndMaybeRotate()
	}
	s.recordRequestDeniedAndMaybeRotate()

	proxyRotateState.mu.Lock()
	got := cfg.GetString("proxy", "")
	proxyRotateState.deniedHits = nil
	proxyRotateState.successHits = nil
	proxyRotateState.mu.Unlock()
	if !strings.Contains(got, "-sid-auto") {
		t.Fatalf("denial-dominant window should rotate, got %q", got)
	}
}

func TestSuccessDominanceDoesNotRotate(t *testing.T) {
	cfg := config.New()
	cfg.Set("use_proxy", true)
	cfg.Set("proxy", "http://u-region-us:pw@gw.test:1000")
	s := &Server{Config: cfg}
	proxyRotateState.mu.Lock()
	proxyRotateState.deniedHits = nil
	proxyRotateState.successHits = nil
	proxyRotateState.lastRotateAt = time.Now().Add(-proxyRotateCooldown) // allow rotation
	proxyRotateState.mu.Unlock()

	// 8 successes vs 3 denials: healthy exit with a doomed account or two —
	// rotating would be wrong.
	for i := 0; i < 8; i++ {
		s.recordRequestSuccess()
	}
	for i := 0; i < 3; i++ {
		s.recordRequestDeniedAndMaybeRotate()
	}
	proxyRotateState.mu.Lock()
	got := cfg.GetString("proxy", "")
	proxyRotateState.mu.Unlock()
	if strings.Contains(got, "-sid-auto") {
		t.Fatalf("success-dominant window must not rotate, got %q", got)
	}
}
