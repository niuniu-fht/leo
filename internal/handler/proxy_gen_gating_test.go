package handler

import (
	"testing"

	"leo2api/internal/config"
)

func TestProxyGeneratorActiveToggle(t *testing.T) {
	cfg := config.New()
	s := &Server{Config: cfg}
	if s.proxyGeneratorActive() {
		t.Fatal("generator must be inactive by default")
	}

	setProxyGenConfig(cfg, true)
	if !s.proxyGeneratorActive() {
		t.Fatal("enabled + complete settings must activate generator")
	}

	// Missing gateway disables it again.
	cfg.Set("proxy_gen_host", "")
	if s.proxyGeneratorActive() {
		t.Fatal("missing host must deactivate generator")
	}
}
