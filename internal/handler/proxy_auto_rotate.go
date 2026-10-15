package handler

import (
	crypto_rand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
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
	mu                    sync.Mutex
	deniedHits            []time.Time
	successHits           []time.Time
	lastRotateAt          time.Time
	hunting               bool
	rotationsSinceSuccess int
}

var proxyRotateState = &proxyAutoRotate{}

const (
	sidHuntMaxCandidates   = 6
	sidHuntValidateTimeout = 60 * time.Second
)

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
	proxyRotateState.rotationsSinceSuccess++
	hunting := proxyRotateState.hunting
	proxyRotateState.mu.Unlock()

	if hunting {
		return // a fast hunt is already cycling candidates
	}
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

// rotateBasicProxySid swaps the session id inside the configured basic proxy
// for a fresh random one and hot-reloads the runtime clients. After two
// session swaps without recovery it flips the region (GB<->US) — the whole
// sub-pool is likely dirty — and launches a fast hunt that validates each
// candidate with a real generation. Generator mode skips (the basic proxy is
// overridden there anyway).
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
	proxyRotateState.mu.Lock()
	proxyRotateState.rotationsSinceSuccess++
	flipRegion := proxyRotateState.rotationsSinceSuccess >= 2
	if flipRegion {
		proxyRotateState.rotationsSinceSuccess = 0
	}
	proxyRotateState.mu.Unlock()
	if flipRegion {
		newURL = toggleProxyRegion(newURL)
		log.Printf("[proxy_rotate] two sessions failed without recovery, flipping region: %s", proxyfmt_maskPassword(newURL))
	}
	s.Config.Set("proxy", newURL)
	if err := s.Config.Save(); err != nil {
		log.Printf("[proxy_rotate] save config failed: %v", err)
		return
	}
	s.reloadRuntimeClients()
	log.Printf("[proxy_rotate] rotated session: %s -> %s", proxyfmt_maskPassword(raw), proxyfmt_maskPassword(newURL))

	go s.huntWorkingExit()
}

// toggleProxyRegion flips region-GB <-> region-US in the proxy username.
func toggleProxyRegion(raw string) string {
	if strings.Contains(raw, "-region-gb") {
		return strings.Replace(raw, "-region-gb", "-region-us", 1)
	}
	if strings.Contains(raw, "-region-us") {
		return strings.Replace(raw, "-region-us", "-region-gb", 1)
	}
	return raw
}

// huntWorkingExit cycles fresh session ids (flipping the region every other
// candidate) until a test generation succeeds or candidates run out.
func (s *Server) huntWorkingExit() {
	proxyRotateState.mu.Lock()
	if proxyRotateState.hunting {
		proxyRotateState.mu.Unlock()
		return
	}
	proxyRotateState.hunting = true
	proxyRotateState.mu.Unlock()
	defer func() {
		proxyRotateState.mu.Lock()
		proxyRotateState.hunting = false
		proxyRotateState.mu.Unlock()
	}()

	for candidate := 1; candidate <= sidHuntMaxCandidates; candidate++ {
		time.Sleep(2 * time.Second) // let the reloaded clients settle
		if s.validateExitByGeneration(sidHuntValidateTimeout) {
			proxyRotateState.mu.Lock()
			proxyRotateState.rotationsSinceSuccess = 0
			proxyRotateState.mu.Unlock()
			log.Printf("[proxy_rotate] candidate %d/%d works, exit pinned", candidate, sidHuntMaxCandidates)
			return
		}
		log.Printf("[proxy_rotate] candidate %d/%d failed validation", candidate, sidHuntMaxCandidates)
		if candidate >= sidHuntMaxCandidates {
			break
		}
		raw := strings.TrimSpace(s.Config.GetString("proxy", ""))
		newURL, err := rotateProxySid(raw)
		if err != nil {
			log.Printf("[proxy_rotate] hunt stopped: %v", err)
			return
		}
		if candidate%2 == 0 {
			newURL = toggleProxyRegion(newURL)
		}
		s.Config.Set("proxy", newURL)
		if err := s.Config.Save(); err != nil {
			log.Printf("[proxy_rotate] hunt save failed: %v", err)
			return
		}
		s.reloadRuntimeClients()
		log.Printf("[proxy_rotate] hunt: switched to %s", proxyfmt_maskPassword(newURL))
	}
	log.Printf("[proxy_rotate] no working exit found after %d candidates; keeping last", sidHuntMaxCandidates)
}

// validateExitByGeneration fires one real generation through the project and
// reports whether it was accepted (HTTP 200).
func (s *Server) validateExitByGeneration(timeout time.Duration) bool {
	if s.Config == nil {
		return false
	}
	user := s.Config.GetString("admin_username", "admin")
	pass := s.Config.GetString("admin_password", "admin")
	base := "http://127.0.0.1:8787"

	loginBody, _ := json.Marshal(map[string]interface{}{"username": user, "password": pass})
	op := &http.Client{Timeout: timeout}
	resp, err := op.Post(base+"/api/v1/auth/login", "application/json", strings.NewReader(string(loginBody)))
	if err != nil {
		log.Printf("[proxy_rotate] validate login failed: %v", err)
		return false
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	resp2, err := op.Get(base + "/api/v1/tokens/export")
	if err != nil {
		log.Printf("[proxy_rotate] validate export failed: %v", err)
		return false
	}
	raw, _ := io.ReadAll(io.LimitReader(resp2.Body, 8<<20))
	resp2.Body.Close()
	var exported struct {
		Tokens []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"tokens"`
	}
	if json.Unmarshal(raw, &exported) != nil {
		return false
	}
	var tokenID string
	for _, tok := range exported.Tokens {
		if strings.EqualFold(tok.Status, "active") {
			tokenID = tok.ID
			break
		}
	}
	if tokenID == "" {
		log.Printf("[proxy_rotate] validate skipped: no active token to test with")
		return false
	}
	genBody, _ := json.Marshal(map[string]interface{}{
		"token_id": tokenID, "model": "gpt-image-2.5-sunburst",
		"aspect_ratio": "1:1", "prompt": "a red apple, product photo",
	})
	resp3, err := op.Post(base+"/api/v1/test-image", "application/json", strings.NewReader(string(genBody)))
	if err != nil {
		log.Printf("[proxy_rotate] validate generation failed: %v", err)
		return false
	}
	defer resp3.Body.Close()
	log.Printf("[proxy_rotate] validate generation status=%d", resp3.StatusCode)
	return resp3.StatusCode == 200
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
	if _, err := crypto_rand.Read(buf); err != nil {
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
