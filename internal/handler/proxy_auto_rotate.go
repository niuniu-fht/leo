package handler

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Automatic exit-IP failover for the basic proxy.
//
// Shared residential gateways assign an exit per source IP (or per sid) and
// that exit can get flagged by upstream risk engines while the account stays
// healthy — every generate then dies with "Request denied" regardless of
// request quality. This module watches the denial rate and, when it crosses
// the threshold, rotates the sid inside the configured proxy URL (hot reload,
// persisted to config.json) to draw a fresh exit.

const (
	requestDeniedWindow    = 30 * time.Minute
	requestDeniedThreshold = 3
	proxyRotateCooldown    = 10 * time.Minute
)

// isRequestDeniedError matches the upstream soft-block where the risk engine
// refuses the generation request itself (HTTP 403 "Request denied" /
// "Access denied") — distinct from "User is blocked", which is a permanent
// account ban and must NOT trigger exit rotation.
func isRequestDeniedError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "request denied") ||
		(strings.Contains(msg, "access denied") && strings.Contains(msg, "httpexception"))
}

type proxyAutoRotate struct {
	mu           sync.Mutex
	deniedHits   []time.Time
	successHits  []time.Time
	lastRotateAt time.Time
}

var proxyRotateState = &proxyAutoRotate{}

// recordRequestDeniedAndMaybeRotate registers one Request-denied failure and
// rotates the basic-proxy sid once enough accumulate inside the window.
func (s *Server) recordRequestDeniedAndMaybeRotate() {
	if s == nil || s.Config == nil {
		return
	}
	proxyRotateState.mu.Lock()
	now := time.Now()
	proxyRotateState.deniedHits = append(proxyRotateState.deniedHits, now)
	kept := proxyRotateState.deniedHits[:0]
	for _, t := range proxyRotateState.deniedHits {
		if now.Sub(t) <= requestDeniedWindow {
			kept = append(kept, t)
		}
	}
	proxyRotateState.deniedHits = kept
	keptOK := proxyRotateState.successHits[:0]
	for _, t := range proxyRotateState.successHits {
		if now.Sub(t) <= requestDeniedWindow {
			keptOK = append(keptOK, t)
		}
	}
	proxyRotateState.successHits = keptOK
	// Denials must outnumber successes: on a healthy exit, production
	// successes would otherwise mask a deteriorating exit forever, while on
	// a flagged exit denials dominate regardless of a few stray successes.
	if len(kept) < requestDeniedThreshold ||
		len(kept) <= len(keptOK) ||
		now.Sub(proxyRotateState.lastRotateAt) < proxyRotateCooldown {
		proxyRotateState.mu.Unlock()
		return
	}
	proxyRotateState.lastRotateAt = now
	proxyRotateState.deniedHits = nil
	proxyRotateState.successHits = nil
	proxyRotateState.mu.Unlock()

	s.rotateBasicProxySid(now)
}

// recordRequestSuccess registers a successful submission in the window.
// Successes are not a reset: they are compared against denials so production
// traffic cannot mask a flagged exit (denials must dominate to rotate).
func (s *Server) recordRequestSuccess() {
	if s == nil {
		return
	}
	proxyRotateState.mu.Lock()
	proxyRotateState.successHits = append(proxyRotateState.successHits, time.Now())
	proxyRotateState.mu.Unlock()
}

// rotateBasicProxySid swaps the sid inside the configured basic proxy for a
// fresh random one and hot-reloads the runtime clients. Only vendor-style
// usernames (containing "-region-") are rotated; generator mode skips (the
// basic proxy is overridden there anyway).
func (s *Server) rotateBasicProxySid(now time.Time) {
	if s.proxyGeneratorActive() {
		log.Printf("[proxy_rotate] skipped: residential generator is active")
		return
	}
	if !s.Config.GetBool("use_proxy", false) {
		log.Printf("[proxy_rotate] skipped: basic proxy disabled")
		return
	}
	raw := strings.TrimSpace(s.Config.GetString("proxy", ""))
	newURL, err := rotateProxySid(raw)
	if err != nil {
		log.Printf("[proxy_rotate] skipped: %v", err)
		return
	}
	s.Config.Set("proxy", newURL)
	if err := s.Config.Save(); err != nil {
		log.Printf("[proxy_rotate] save config failed: %v", err)
		return
	}
	s.reloadRuntimeClients()
	log.Printf("[proxy_rotate] exit flagged (Request denied x%d in %s), rotated proxy sid: %s -> %s",
		requestDeniedThreshold, requestDeniedWindow, proxyfmt_maskPassword(raw), proxyfmt_maskPassword(newURL))
}

func proxyfmt_maskPassword(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.User == nil {
		return raw
	}
	if _, ok := u.User.Password(); ok {
		return u.Scheme + "://" + u.User.Username() + ":***@" + u.Host + u.Path
	}
	return raw
}

// rotateProxySid swaps the session id inside the proxy username for a fresh
// one, preserving every other parameter. Two vendor styles are understood:
//   - "...-sid-<id>[-t-<min>]"          (1024proxy style)
//   - "...-session-<id>-sessTime-<m>[-sessAuto-1]" (bestgo style)
//
// Usernames without a "-region-" marker are refused (unknown providers may
// not support session parameters at all).
func rotateProxySid(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("basic proxy is empty")
	}
	u, err := url.Parse(raw)
	if err != nil || u.User == nil {
		return "", fmt.Errorf("proxy url has no credentials")
	}
	username := u.User.Username()
	if !strings.Contains(username, "-region-") {
		return "", fmt.Errorf("proxy username does not look like a region-pool vendor (no -region-), refusing auto-rotate")
	}
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	newUsername := rotateSessionInUsername(username, hex.EncodeToString(buf))
	password, _ := u.User.Password()
	u.User = url.UserPassword(newUsername, password)
	return u.String(), nil
}

// rotateSessionInUsername replaces the token following the session marker
// ("sid" or "session"), keeping the remainder (sessTime/sessAuto/t params)
// untouched. Appends in bestgo style when no marker exists yet.
func rotateSessionInUsername(username string, newID string) string {
	parts := strings.Split(username, "-")
	for i, part := range parts {
		if (part == "sid" || part == "session") && i+1 < len(parts) {
			parts[i+1] = newID
			return strings.Join(parts, "-")
		}
	}
	return username + "-session-" + newID + "-sessTime-180-sessAuto-1"
}
