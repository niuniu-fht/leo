package proxygen

import (
	"strings"
	"testing"
)

func TestBuildURLStickyPerAccount(t *testing.T) {
	st := Settings{
		ProviderID:    "1024proxy",
		Host:          "gate.example.com",
		Port:          8888,
		Username:      "myuser",
		Password:      "p@ss:word",
		Country:       "US",
		SessionMode:   "sticky",
		StickyMinutes: 30,
	}
	if !st.Enabled() {
		t.Fatal("expected settings to be enabled")
	}

	urlA := st.BuildURL("id:token-a")
	urlB := st.BuildURL("id:token-b")
	if urlA == "" || urlB == "" {
		t.Fatalf("expected non-empty urls, got %q / %q", urlA, urlB)
	}
	if urlA == urlB {
		t.Fatalf("expected different sticky urls per account: %q", urlA)
	}
	// Stability across calls.
	if urlA != st.BuildURL("id:token-a") {
		t.Fatalf("expected stable sid per account: %q vs %q", urlA, st.BuildURL("id:token-a"))
	}

	sid := AccountSID("id:token-a")
	if !strings.Contains(urlA, "myuser-region-us-sid-"+sid+"-t-30") {
		t.Fatalf("unexpected decorated username in %q", urlA)
	}
	if !strings.Contains(urlA, ":p%40ss%3Aword@gate.example.com:8888") {
		t.Fatalf("expected escaped credentials in %q", urlA)
	}
}

func TestBuildRotatingURL(t *testing.T) {
	st := Settings{
		ProviderID:  "b2proxy",
		Host:        "gate.b2proxy.com",
		Port:        7777,
		Username:    "u1",
		Password:    "pw",
		Country:     "DE",
		SessionMode: "sticky",
	}
	rotating := st.BuildRotatingURL()
	if rotating == "" {
		t.Fatal("expected rotating url")
	}
	if strings.Contains(rotating, "-sid-") || strings.Contains(rotating, "-t-") {
		t.Fatalf("rotating url must not carry sticky params: %q", rotating)
	}
	if !strings.Contains(rotating, "u1-region-de") {
		t.Fatalf("expected country flag in %q", rotating)
	}
}

func TestCountryOmittedWhenEmpty(t *testing.T) {
	st := Settings{ProviderID: "1024proxy", Host: "h", Port: 1, Username: "u", Password: "p", SessionMode: "rotating", Country: "zz!"}
	url := st.BuildRotatingURL()
	if strings.Contains(url, "region") {
		t.Fatalf("invalid country should be omitted: %q", url)
	}
}

func TestEnabledRequiresHostPortUser(t *testing.T) {
	st := Settings{ProviderID: "custom", Username: "u", Password: "p", Host: "", Port: 0}
	if st.Enabled() {
		t.Fatal("empty host must disable generator")
	}
	if st.BuildURL("k") != "" || st.BuildRotatingURL() != "" {
		t.Fatal("disabled generator must build nothing")
	}
}

func TestMaskProxyURL(t *testing.T) {
	masked := MaskProxyURL("http://user-region-us-sid-abc:secret@h:1")
	if strings.Contains(masked, "secret") || !strings.Contains(masked, "***") {
		t.Fatalf("password not masked: %q", masked)
	}
	if MaskProxyURL("http://h:1") != "http://h:1" {
		t.Fatalf("no-user url should be unchanged: %q", MaskProxyURL("http://h:1"))
	}
}

func TestProviderByIDFallback(t *testing.T) {
	if ProviderByID("nope").ID != "custom" {
		t.Fatal("unknown provider should fall back to custom")
	}
	if NormalizeProviderID("1024PROXY ") != "1024proxy" {
		t.Fatal("provider id should be normalized")
	}
}
