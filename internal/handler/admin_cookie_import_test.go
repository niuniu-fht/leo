package handler

import (
	"strings"
	"testing"
)

func TestNormalizeImportedCookieJSONPreservesAccessToken(t *testing.T) {
	raw := `{"accessToken":"jwt.header.payload","cookies":[{"name":"__Secure-better-auth.session_token","value":"session"},{"name":"_vcrcs","value":"challenge"}]}`
	got := normalizeImportedCookie(raw)
	if !strings.Contains(got, "__Secure-better-auth.session_token=session") {
		t.Fatalf("missing session cookie: %q", got)
	}
	if !strings.Contains(got, "_vcrcs=challenge") {
		t.Fatalf("missing challenge cookie: %q", got)
	}
	if !strings.Contains(got, "\ntoken=jwt.header.payload") {
		t.Fatalf("missing embedded access token: %q", got)
	}
}

func TestNormalizeImportedCookieStringKeepsPlainCookie(t *testing.T) {
	got := normalizeImportedCookie("Cookie: a=b; c=d")
	if got != "a=b; c=d" {
		t.Fatalf("cookie = %q", got)
	}
}

func TestNormalizeImportedCookieJSONKeepsRefreshableLeonardoCookies(t *testing.T) {
	raw := `{"cookies":[
		{"name":"__Secure-better-auth.session_token","value":"old","domain":".app.leonardo.ai","path":"/","hostOnly":false,"expirationDate":100},
		{"name":"__Secure-better-auth.session_token","value":"new","domain":"app.leonardo.ai","path":"/","hostOnly":true,"expirationDate":200},
		{"name":"__Secure-better-auth.session_data.0","value":"data-new","domain":"app.leonardo.ai","path":"/","hostOnly":true,"expirationDate":200},
		{"name":"external","value":"drop","domain":"example.com","path":"/","hostOnly":true}
	]}`
	got := normalizeImportedCookie(raw)
	if !strings.Contains(got, "__Secure-better-auth.session_token=new") {
		t.Fatalf("missing host-only session token: %q", got)
	}
	if strings.Contains(got, "__Secure-better-auth.session_token=old") {
		t.Fatalf("stale duplicate session token kept: %q", got)
	}
	if !strings.Contains(got, "__Secure-better-auth.session_data.0=data-new") {
		t.Fatalf("missing refreshable session_data shard: %q", got)
	}
	if strings.Contains(got, "external=drop") {
		t.Fatalf("external cookie kept: %q", got)
	}
}

func TestNormalizeImportedCookieJSONDoesNotTreatCookieValueAsAccessToken(t *testing.T) {
	raw := `{"cookies":[
		{"name":"__Secure-better-auth.session_token","value":"session","domain":"app.leonardo.ai","path":"/"},
		{"name":"cognito","value":"eyJhbGciOiJkaXIifQ.payload.signature","domain":"app.leonardo.ai","path":"/"}
	]}`
	got := normalizeImportedCookie(raw)
	if strings.Contains(got, "\ntoken=") {
		t.Fatalf("cookie value was incorrectly appended as token: %q", got)
	}
	if !strings.Contains(got, "__Secure-better-auth.session_token=session") {
		t.Fatalf("missing refreshable session cookie: %q", got)
	}
}
