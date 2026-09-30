// Package proxygen builds residential proxy URLs from provider credentials
// (username/password + gateway), with a stable per-account sticky session id
// so every Leonardo account keeps its own exit IP.
package proxygen

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// Provider describes a residential proxy vendor preset. All listed vendors
// share the "username-<ccKey>-CC-<sidKey>-SID-<ttlKey>-MINUTES" convention
// (1024Proxy documents it as user-region-DE-sid-xxx-t-5).
type Provider struct {
	ID                string
	Label             string
	CountryKey        string
	SidKey            string
	TTLKey            string
	DefaultTTLMinutes int
}

var providerPresets = map[string]Provider{
	"1024proxy": {ID: "1024proxy", Label: "1024Proxy", CountryKey: "region", SidKey: "sid", TTLKey: "t", DefaultTTLMinutes: 30},
	"b2proxy":   {ID: "b2proxy", Label: "B2Proxy", CountryKey: "region", SidKey: "sid", TTLKey: "t", DefaultTTLMinutes: 30},
	"cliproxy":  {ID: "cliproxy", Label: "CliProxy", CountryKey: "region", SidKey: "sid", TTLKey: "t", DefaultTTLMinutes: 30},
	"iprocket":  {ID: "iprocket", Label: "IPRocket", CountryKey: "region", SidKey: "sid", TTLKey: "t", DefaultTTLMinutes: 5},
	"custom":    {ID: "custom", Label: "自定义", CountryKey: "region", SidKey: "sid", TTLKey: "t", DefaultTTLMinutes: 30},
}

// ProviderIDs returns the preset ids in a stable order (for UI dropdowns).
func ProviderIDs() []string {
	return []string{"1024proxy", "b2proxy", "cliproxy", "iprocket", "custom"}
}

// ProviderByID returns the preset for id, falling back to "custom".
func ProviderByID(id string) Provider {
	id = strings.ToLower(strings.TrimSpace(id))
	if p, ok := providerPresets[id]; ok {
		return p
	}
	return providerPresets["custom"]
}

// Settings is the resolved generator configuration.
type Settings struct {
	ProviderID    string
	Host          string
	Port          int
	Username      string
	Password      string
	Country       string // lowercase ISO-3166 alpha-2, "" = any
	SessionMode   string // "sticky" | "rotating"
	StickyMinutes int
}

// NormalizeProviderID validates the provider id.
func NormalizeProviderID(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if _, ok := providerPresets[raw]; ok {
		return raw
	}
	return "custom"
}

// NormalizeCountry lowercases and validates a 2-letter country code.
func NormalizeCountry(raw string) string {
	cc := strings.ToLower(strings.TrimSpace(raw))
	if len(cc) == 2 {
		for _, r := range cc {
			if r < 'a' || r > 'z' {
				return ""
			}
		}
		return cc
	}
	return ""
}

// NormalizeSessionMode maps free-form input to sticky/rotating.
func NormalizeSessionMode(raw string) string {
	if strings.EqualFold(strings.TrimSpace(raw), "rotating") {
		return "rotating"
	}
	return "sticky"
}

// Enabled reports whether the generator has enough input to build URLs.
func (st Settings) Enabled() bool {
	return strings.TrimSpace(st.Host) != "" &&
		st.Port > 0 && st.Port <= 65535 &&
		strings.TrimSpace(st.Username) != ""
}

// IsSticky reports whether sticky sessions are requested.
func (st Settings) IsSticky() bool {
	return NormalizeSessionMode(st.SessionMode) == "sticky"
}

// AccountSID derives a stable pseudo-random sticky session id for accountKey.
// Deterministic so a restart keeps the same account->exit-IP mapping.
func AccountSID(accountKey string) string {
	sum := sha256.Sum256([]byte("leo2api-sid-v1:" + accountKey))
	return hex.EncodeToString(sum[:])[:12]
}

// RandomSID returns a fresh random session id (connectivity tests).
func RandomSID() string {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		return AccountSID(strconv.FormatInt(randInt(), 10))
	}
	return hex.EncodeToString(buf)
}

func randInt() int64 {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return 42
	}
	var v int64
	for _, c := range b {
		v = v<<8 | int64(c)
	}
	if v < 0 {
		return -v
	}
	return v
}

// DecoratedUsername renders the parameterized proxy username. In sticky mode
// an empty sid is replaced with a random one.
func (st Settings) DecoratedUsername(sid string) string {
	preset := ProviderByID(st.ProviderID)
	parts := []string{strings.TrimSpace(st.Username)}
	if cc := NormalizeCountry(st.Country); cc != "" {
		parts = append(parts, preset.CountryKey+"-"+cc)
	}
	if st.IsSticky() {
		sid = strings.TrimSpace(sid)
		if sid == "" {
			sid = RandomSID()
		}
		parts = append(parts, preset.SidKey+"-"+sid)
		ttl := st.StickyMinutes
		if ttl <= 0 {
			ttl = preset.DefaultTTLMinutes
		}
		if ttl > 120 {
			ttl = 120
		}
		if ttl > 0 {
			parts = append(parts, preset.TTLKey+"-"+strconv.Itoa(ttl))
		}
	}
	return strings.Join(parts, "-")
}

// BuildURL returns the proxy URL for one account. accountKey seeds the stable
// sticky sid ("id:<tokenID>" for pool accounts). Returns "" when disabled.
func (st Settings) BuildURL(accountKey string) string {
	return st.buildURL(st.DecoratedUsername(AccountSID(accountKey)))
}

// BuildURLWithSID returns the proxy URL pinned to an explicit sid (tests).
func (st Settings) BuildURLWithSID(sid string) string {
	return st.buildURL(st.DecoratedUsername(sid))
}

// BuildRotatingURL returns a non-sticky proxy URL for gateway-global traffic
// (token import/validation, one-off business calls).
func (st Settings) BuildRotatingURL() string {
	if !st.Enabled() {
		return ""
	}
	rotating := st
	rotating.SessionMode = "rotating"
	return rotating.buildURL(rotating.DecoratedUsername(""))
}

func (st Settings) buildURL(username string) string {
	if !st.Enabled() {
		return ""
	}
	password := st.Password
	if password == "***" {
		password = ""
	}
	return (&url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort(strings.TrimSpace(st.Host), strconv.Itoa(st.Port)),
		User:   url.UserPassword(username, password),
	}).String()
}

// MaskProxyURL replaces the password in a proxy URL with *** for display.
func MaskProxyURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.User == nil {
		return raw
	}
	password, _ := parsed.User.Password()
	if password == "" {
		return raw
	}
	rest := &url.URL{Scheme: parsed.Scheme, Host: parsed.Host, Path: parsed.Path, RawQuery: parsed.RawQuery}
	return parsed.Scheme + "://" + parsed.User.Username() + ":***@" + rest.Host + rest.Path
}

// ConfigGetter is the config-manager surface SettingsFromConfig needs.
type ConfigGetter interface {
	GetString(key string, defaultVal ...string) string
	GetInt(key string, defaultVal ...int) int
	GetBool(key string, defaultVal ...bool) bool
}

// SettingsFromConfig reads the flat proxy_gen_* keys.
func SettingsFromConfig(cfg ConfigGetter) Settings {
	if cfg == nil {
		return Settings{}
	}
	return Settings{
		ProviderID:    NormalizeProviderID(cfg.GetString("proxy_gen_provider", "1024proxy")),
		Host:          strings.TrimSpace(cfg.GetString("proxy_gen_host", "")),
		Port:          cfg.GetInt("proxy_gen_port", 0),
		Username:      strings.TrimSpace(cfg.GetString("proxy_gen_username", "")),
		Password:      cfg.GetString("proxy_gen_password", ""),
		Country:       NormalizeCountry(cfg.GetString("proxy_gen_country", "")),
		SessionMode:   NormalizeSessionMode(cfg.GetString("proxy_gen_session_mode", "sticky")),
		StickyMinutes: cfg.GetInt("proxy_gen_sticky_minutes", 30),
	}
}

// GeneratorEnabled reports whether the generator is switched on and usable.
func GeneratorEnabled(cfg ConfigGetter) bool {
	if cfg == nil || !cfg.GetBool("proxy_gen_enabled", false) {
		return false
	}
	return SettingsFromConfig(cfg).Enabled()
}
