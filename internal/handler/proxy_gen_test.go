package handler

import (
	"strings"
	"testing"

	"leo2api/internal/config"
	"leo2api/internal/provider/leonardo"
	"leo2api/internal/proxygen"
)

func setProxyGenConfig(cfg *config.Manager, enabled bool) {
	cfg.Set("proxy_gen_enabled", enabled)
	cfg.Set("proxy_gen_provider", "1024proxy")
	cfg.Set("proxy_gen_host", "gate.test.com")
	cfg.Set("proxy_gen_port", 8888)
	cfg.Set("proxy_gen_username", "myuser")
	cfg.Set("proxy_gen_password", "secret")
	cfg.Set("proxy_gen_country", "us")
	cfg.Set("proxy_gen_session_mode", "sticky")
	cfg.Set("proxy_gen_sticky_minutes", 45)
}

func TestSessionProxyOverrideForKey(t *testing.T) {
	cfg := config.New()
	s := &Server{Config: cfg}

	if got := s.sessionProxyOverrideForKey("id:t1"); got != "" {
		t.Fatalf("disabled generator must produce empty override, got %q", got)
	}

	setProxyGenConfig(cfg, true)
	a := s.sessionProxyOverrideForKey("id:token-a")
	b := s.sessionProxyOverrideForKey("id:token-b")
	if a == "" || b == "" || a == b {
		t.Fatalf("expected distinct sticky urls per account: %q / %q", a, b)
	}
	if !strings.Contains(a, "myuser-region-us-sid-"+proxygen.AccountSID("id:token-a")+"-t-45") {
		t.Fatalf("unexpected url: %q", a)
	}

	// Missing host disables the generator.
	cfg.Set("proxy_gen_host", "")
	if got := s.sessionProxyOverrideForKey("id:token-a"); got != "" {
		t.Fatalf("incomplete generator must produce empty override, got %q", got)
	}
}

func TestRefreshSessionProxyOverridesOnConfigChange(t *testing.T) {
	cfg := config.New()
	s := &Server{Config: cfg}
	session := &leonardo.TokenSession{FullCookie: "c"}
	s.leoSessions = map[string]*leonardo.TokenSession{"id:t1": session}

	s.refreshSessionProxyOverrides()
	if session.ProxyOverride != "" {
		t.Fatalf("disabled generator must clear overrides, got %q", session.ProxyOverride)
	}

	setProxyGenConfig(cfg, true)
	s.refreshSessionProxyOverrides()
	if !strings.Contains(session.ProxyOverride, "sid-"+proxygen.AccountSID("id:t1")) {
		t.Fatalf("refresh must apply per-account sticky override, got %q", session.ProxyOverride)
	}

	cfg.Set("proxy_gen_enabled", false)
	s.refreshSessionProxyOverrides()
	if session.ProxyOverride != "" {
		t.Fatalf("disabling generator must clear override, got %q", session.ProxyOverride)
	}
}
