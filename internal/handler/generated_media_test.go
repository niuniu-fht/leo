package handler

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"leo2api/internal/config"
)

func TestMaterializeGeneratedMediaUpstreamModeSkipsLocalFetch(t *testing.T) {
	cfg := config.Global()
	original := cfg.GetAll()
	cfg.SetAll(map[string]interface{}{})
	t.Cleanup(func() {
		cfg.SetAll(original)
	})

	sourceHits := 0
	sourceServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sourceHits++
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = w.Write([]byte("unexpected-fetch"))
	}))
	defer sourceServer.Close()

	cfg.SetAll(map[string]interface{}{
		"use_upstream_result_url": true,
	})

	srv := &Server{
		Config:       cfg,
		GeneratedDir: t.TempDir(),
	}
	resultURL, err := srv.materializeGeneratedMedia(sourceServer.URL+"/remote.mp4", "remote-mode", "video")
	if err != nil {
		t.Fatalf("materializeGeneratedMedia returned error: %v", err)
	}
	if resultURL != sourceServer.URL+"/remote.mp4" {
		t.Fatalf("expected upstream url passthrough, got %q", resultURL)
	}
	if sourceHits != 0 {
		t.Fatalf("expected source media not to be fetched locally, got %d hit(s)", sourceHits)
	}
}

func TestMaterializeGeneratedMediaSavesToLocal(t *testing.T) {
	cfg := config.Global()
	original := cfg.GetAll()
	cfg.SetAll(map[string]interface{}{})
	t.Cleanup(func() {
		cfg.SetAll(original)
	})

	sourceServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("fake-png-data"))
	}))
	defer sourceServer.Close()

	dir := t.TempDir()
	srv := &Server{
		Config:       cfg,
		GeneratedDir: dir,
	}
	resultURL, err := srv.materializeGeneratedMedia(sourceServer.URL+"/result.png", "demo-image", "image")
	if err != nil {
		t.Fatalf("materializeGeneratedMedia returned error: %v", err)
	}
	if resultURL != "/generated/demo-image.png" {
		t.Fatalf("expected local generated url, got %q", resultURL)
	}

	filePath := filepath.Join(dir, "demo-image.png")
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("expected local file to exist: %v", err)
	}
	if string(data) != "fake-png-data" {
		t.Fatalf("unexpected local payload: %q", string(data))
	}
}

func TestMaterializeGeneratedMediaFailsWhenFetchFails(t *testing.T) {
	cfg := config.Global()
	original := cfg.GetAll()
	cfg.SetAll(map[string]interface{}{})
	t.Cleanup(func() {
		cfg.SetAll(original)
	})

	sourceServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer sourceServer.Close()

	srv := &Server{
		Config:       cfg,
		GeneratedDir: t.TempDir(),
	}
	_, err := srv.materializeGeneratedMedia(sourceServer.URL+"/result.mp4", "demo-video", "video")
	if err == nil {
		t.Fatal("expected error when source media fetch fails")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Fatalf("expected status code in error, got %v", err)
	}
}

func TestSanitizeGeneratedFilename(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"Hello World", "hello-world"},
		{"a_b-c", "a-b-c"},
		{"中文名", ""},
		{"", ""},
		{"!!!", ""},
		{"Demo  Image.01", "demo-image-01"},
	}
	for _, c := range cases {
		if got := sanitizeGeneratedFilename(c.in); got != c.want {
			t.Fatalf("sanitizeGeneratedFilename(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
