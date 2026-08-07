package leonardo

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestRefreshSessionPersistsRotatedCookieBeforeJWTValidation(t *testing.T) {
	client := NewClient("")
	client.httpClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			header := make(http.Header)
			header.Add("Set-Cookie", "__Secure-better-auth.session_token=new-session; Path=/; Secure; HttpOnly")
			header.Add("Set-Cookie", "__Secure-better-auth.session_data.0=new-data; Path=/; Secure; HttpOnly")
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     header,
				Body:       io.NopCloser(strings.NewReader(`{"session":{},"user":{}}`)),
				Request:    req,
			}, nil
		}),
	}

	session := &TokenSession{
		FullCookie:   "__Secure-better-auth.session_token=old-session; __Secure-better-auth.session_data.0=old-data",
		SourceCookie: "__Secure-better-auth.session_token=old-session; __Secure-better-auth.session_data.0=old-data",
	}
	persisted := ""
	session.SetCookiePersistHandler(func(cookie string) {
		persisted = cookie
		session.MarkCookiePersisted(cookie)
	})

	err := client.RefreshSession(session)
	if err == nil || !strings.Contains(err.Error(), "no JWT found") {
		t.Fatalf("RefreshSession() error = %v, want no JWT found", err)
	}
	if !strings.Contains(persisted, "__Secure-better-auth.session_token=new-session") {
		t.Fatalf("rotated session token was not persisted: %q", persisted)
	}
	if !strings.Contains(persisted, "__Secure-better-auth.session_data.0=new-data") {
		t.Fatalf("rotated session data was not persisted: %q", persisted)
	}
	if got := session.CookieSnapshot(); got != persisted {
		t.Fatalf("CookieSnapshot() = %q, want persisted cookie %q", got, persisted)
	}
	session.mu.RLock()
	sourceCookie := session.SourceCookie
	session.mu.RUnlock()
	if sourceCookie != persisted {
		t.Fatalf("SourceCookie = %q, want persisted cookie %q", sourceCookie, persisted)
	}
}

func TestRefreshSessionKeepsOriginalCookieWhenNoJWTResponseDeletesSessionCookies(t *testing.T) {
	client := NewClient("")
	client.httpClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			header := make(http.Header)
			header.Add("Set-Cookie", "__Secure-better-auth.session_token=; Path=/; Max-Age=0; Secure; HttpOnly")
			header.Add("Set-Cookie", "__Secure-better-auth.session_data.0=deleted; Path=/; Expires=Thu, 01 Jan 1970 00:00:00 GMT; Secure; HttpOnly")
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     header,
				Body:       io.NopCloser(strings.NewReader(`{"session":{},"user":{}}`)),
				Request:    req,
			}, nil
		}),
	}

	original := "__Secure-better-auth.session_token=old-session; __Secure-better-auth.session_data.0=old-data; CF_Access_Token=cf-token"
	session := &TokenSession{FullCookie: original, SourceCookie: original}
	persistCalls := 0
	session.SetCookiePersistHandler(func(cookie string) {
		persistCalls++
	})

	err := client.RefreshSession(session)
	if err == nil || !strings.Contains(err.Error(), "no JWT found") {
		t.Fatalf("RefreshSession() error = %v, want no JWT found", err)
	}
	if persistCalls != 0 {
		t.Fatalf("persist handler called %d times, want 0", persistCalls)
	}
	if got := session.CookieSnapshot(); got != original {
		t.Fatalf("CookieSnapshot() = %q, want original cookie %q", got, original)
	}
}

func TestRefreshSessionCommitsCriticalCookieDeletionAfterValidJWT(t *testing.T) {
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"user-1","exp":4102444800}`))
	jwt := "eyJhbGciOiJub25lIn0." + payload + ".signature"
	client := NewClient("")
	client.httpClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			header := make(http.Header)
			header.Add("Set-Cookie", "__Secure-better-auth.session_data.1=; Path=/; Max-Age=0; Secure; HttpOnly")
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     header,
				Body:       io.NopCloser(strings.NewReader(fmt.Sprintf(`{"session":{"token":%q}}`, jwt))),
				Request:    req,
			}, nil
		}),
	}

	original := "__Secure-better-auth.session_token=session; __Secure-better-auth.session_data.0=data-0; __Secure-better-auth.session_data.1=data-1; CF_Access_Token=cf-token"
	session := &TokenSession{FullCookie: original, SourceCookie: original}
	persisted := ""
	session.SetCookiePersistHandler(func(cookie string) {
		persisted = cookie
		session.MarkCookiePersisted(cookie)
	})

	if err := client.RefreshSession(session); err != nil {
		t.Fatalf("RefreshSession() error = %v", err)
	}
	if persisted == "" {
		t.Fatal("critical cookie deletion was not persisted after valid JWT")
	}
	if strings.Contains(persisted, "__Secure-better-auth.session_data.1=") {
		t.Fatalf("deleted session_data shard is still present: %q", persisted)
	}
	if !strings.Contains(persisted, "__Secure-better-auth.session_token=session") {
		t.Fatalf("session token was unexpectedly removed: %q", persisted)
	}
}

func TestDeletesCriticalSessionCookieUsesFinalResponseAction(t *testing.T) {
	name := "__Secure-better-auth.session_token"
	deleteThenReplace := []*http.Cookie{
		{Name: name, MaxAge: -1},
		{Name: name, Value: "replacement"},
	}
	if deletesCriticalSessionCookie(deleteThenReplace) {
		t.Fatal("final replacement should override an earlier deletion")
	}

	replaceThenDelete := []*http.Cookie{
		{Name: name, Value: "replacement"},
		{Name: name, MaxAge: -1},
	}
	if !deletesCriticalSessionCookie(replaceThenDelete) {
		t.Fatal("final deletion should be treated as destructive")
	}
}
