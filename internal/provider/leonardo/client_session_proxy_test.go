package leonardo

import (
	"errors"
	"strings"
	"testing"
)

func TestSessionProxyOverrideClientSelection(t *testing.T) {
	client := NewClient("")

	session := &TokenSession{FullCookie: "cookie=1"}
	if got := client.graphQLClientFor(session); got != client.httpClient {
		t.Fatalf("session without override must use global client")
	}

	session.SetProxyOverride("http://user-region-us-sid-abc-t-30:pass@127.0.0.1:58080")
	graphQL := client.graphQLClientFor(session)
	if graphQL == nil || graphQL == client.httpClient {
		t.Fatalf("session with override must get its own graphql client")
	}
	// Cached: same instance on subsequent calls.
	if client.graphQLClientFor(session) != graphQL {
		t.Fatalf("per-session graphql client should be cached")
	}
	imp := client.impersonatorFor(session)
	if imp == nil || imp == client.sessionImpersonatorClient {
		t.Fatalf("session with override must get its own impersonator")
	}
	if client.uploadInitClientFor(session) == client.uploadInitHTTPClient {
		t.Fatalf("basic upload mode must follow session override")
	}
	if client.uploadClientFor(session) == client.uploadHTTPClient {
		t.Fatalf("basic upload mode must follow session override for S3")
	}

	// Changing the override rebuilds the client set.
	session.SetProxyOverride("http://user-region-gb-sid-def-t-30:pass@127.0.0.1:58081")
	if client.graphQLClientFor(session) == graphQL {
		t.Fatalf("override change must drop cached clients")
	}
	// Clearing the override returns to the global clients.
	session.SetProxyOverride("")
	if client.graphQLClientFor(session) != client.httpClient {
		t.Fatalf("cleared override must fall back to global client")
	}
}

func TestUploadDirectModeIgnoresSessionOverride(t *testing.T) {
	client := NewClient("")
	if err := client.SetUploadProxyConfig("direct", ""); err != nil {
		t.Fatalf("direct mode: %v", err)
	}
	session := &TokenSession{FullCookie: "cookie=1"}
	session.SetProxyOverride("http://u:p@127.0.0.1:58080")
	if client.uploadInitClientFor(session) != client.uploadInitHTTPClient {
		t.Fatalf("direct upload mode must ignore session override")
	}
	if client.uploadClientFor(session) != client.uploadHTTPClient {
		t.Fatalf("direct upload mode must ignore session override for S3")
	}
}

func TestSessionProxyOverrideStableAndDistinct(t *testing.T) {
	a := &TokenSession{FullCookie: "a"}
	b := &TokenSession{FullCookie: "b"}
	a.SetProxyOverride("http://u:p@127.0.0.1:1")
	b.SetProxyOverride("http://u:p@127.0.0.1:1")
	client := NewClient("")
	if client.graphQLClientFor(a) == client.graphQLClientFor(b) {
		t.Fatalf("different sessions must not share per-session clients")
	}
	if !strings.Contains(a.ProxyOverride, "127.0.0.1:1") {
		t.Fatalf("override should be stored verbatim: %q", a.ProxyOverride)
	}
}

func TestIsProxyRateLimitedError(t *testing.T) {
	cases := []struct {
		msg  string
		want bool
	}{
		{`Get "https://app.leonardo.ai/api/auth/get-session": Too Many Requests`, true},
		{"proxyconnect tcp: dial tcp 127.0.0.1:7890: connection refused", false},
		{"graphql returned 429: {\"code\":\"challenge\"}", false},
		{"graphql returned 500: Too Many Requests in body", false},
		{"read tcp 1.2.3.4:1->5.6.7.8:2: wsarecv: connection reset", false},
		{"", false},
	}
	for _, tc := range cases {
		var err error
		if tc.msg != "" {
			err = errors.New(tc.msg)
		}
		if got := isProxyRateLimitedError(err); got != tc.want {
			t.Errorf("isProxyRateLimitedError(%q) = %v, want %v", tc.msg, got, tc.want)
		}
	}
}
