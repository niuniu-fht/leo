package proxyfmt

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// NormalizeHTTPProxyURL accepts regular URLs and compact proxy forms:
//   - http://user:pass@host:port
//   - https://user:pass@host:port
//   - host:port
//   - host:port:user:pass
//   - user:pass@host:port
//
// Compact forms default to http://.
func NormalizeHTTPProxyURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}

	lower := strings.ToLower(raw)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return validateHTTPProxyURL(raw)
	}
	if strings.Contains(raw, "@") {
		return validateHTTPProxyURL("http://" + raw)
	}

	parts := strings.Split(raw, ":")
	switch {
	case len(parts) == 2:
		host := strings.TrimSpace(parts[0])
		port := strings.TrimSpace(parts[1])
		if err := validateHostPortParts(host, port); err != nil {
			return "", err
		}
		return (&url.URL{Scheme: "http", Host: net.JoinHostPort(host, port)}).String(), nil
	case len(parts) >= 4:
		host := strings.TrimSpace(parts[0])
		port := strings.TrimSpace(parts[1])
		username := strings.TrimSpace(parts[2])
		password := strings.Join(parts[3:], ":")
		if err := validateHostPortParts(host, port); err != nil {
			return "", err
		}
		if username == "" {
			return "", fmt.Errorf("proxy username is empty")
		}
		return (&url.URL{
			Scheme: "http",
			Host:   net.JoinHostPort(host, port),
			User:   url.UserPassword(username, password),
		}).String(), nil
	default:
		return "", fmt.Errorf("invalid proxy format")
	}
}

func validateHTTPProxyURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", fmt.Errorf("invalid proxy url: %w", err)
	}
	scheme := strings.ToLower(strings.TrimSpace(parsed.Scheme))
	if scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("proxy scheme must be http or https")
	}
	if strings.TrimSpace(parsed.Host) == "" {
		return "", fmt.Errorf("proxy host is empty")
	}
	return parsed.String(), nil
}

func validateHostPortParts(host string, port string) error {
	if host == "" {
		return fmt.Errorf("proxy host is empty")
	}
	if port == "" {
		return fmt.Errorf("proxy port is empty")
	}
	p, err := strconv.Atoi(port)
	if err != nil || p < 1 || p > 65535 {
		return fmt.Errorf("invalid proxy port")
	}
	return nil
}
