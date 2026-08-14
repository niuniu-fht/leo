package proxyfmt

import "testing"

func TestNormalizeHTTPProxyURLHostPortUserPass(t *testing.T) {
	got, err := NormalizeHTTPProxyURL("proxy.example.com:10000:USER-zone-custom:PASSWORD")
	if err != nil {
		t.Fatalf("NormalizeHTTPProxyURL error = %v", err)
	}
	want := "http://USER-zone-custom:PASSWORD@proxy.example.com:10000"
	if got != want {
		t.Fatalf("NormalizeHTTPProxyURL = %q, want %q", got, want)
	}
}

func TestNormalizeHTTPProxyURLHostPort(t *testing.T) {
	got, err := NormalizeHTTPProxyURL("127.0.0.1:7890")
	if err != nil {
		t.Fatalf("NormalizeHTTPProxyURL error = %v", err)
	}
	want := "http://127.0.0.1:7890"
	if got != want {
		t.Fatalf("NormalizeHTTPProxyURL = %q, want %q", got, want)
	}
}

func TestNormalizeHTTPProxyURLUserPassAtHostPort(t *testing.T) {
	got, err := NormalizeHTTPProxyURL("user:pass@example.com:10000")
	if err != nil {
		t.Fatalf("NormalizeHTTPProxyURL error = %v", err)
	}
	want := "http://user:pass@example.com:10000"
	if got != want {
		t.Fatalf("NormalizeHTTPProxyURL = %q, want %q", got, want)
	}
}
