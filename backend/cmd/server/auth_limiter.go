package main

import (
	"context"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	authRateLimitMaxAttempts = 5
	authRateLimitWindow      = 15 * time.Minute
	maxTrustedProxyHops      = 16
)

type proxyHeaderConfig struct {
	trustedNetworks []*net.IPNet
	trustedHops     int
}

type authRateLimitEntry struct {
	windowStart time.Time
	attempts    int
}

type authRateLimiter struct {
	mu      sync.Mutex
	entries map[string]authRateLimitEntry
}

func newAuthRateLimiter() *authRateLimiter {
	return &authRateLimiter{entries: make(map[string]authRateLimitEntry)}
}

func (s *server) allowAuth(ctx context.Context, keys ...string) (bool, error) {
	if postgres, ok := s.repository.(*postgresRepository); ok {
		return postgres.allowAuthAttempts(ctx, keys, time.Now(), authRateLimitMaxAttempts, authRateLimitWindow)
	}
	return s.authLimiter.allow(keys...), nil
}

func (s *server) resetAuth(ctx context.Context, keys ...string) error {
	if postgres, ok := s.repository.(*postgresRepository); ok {
		return postgres.resetAuthAttempts(ctx, keys)
	}
	s.authLimiter.reset(keys...)
	return nil
}

func (l *authRateLimiter) allow(keys ...string) bool {
	now := time.Now()
	keys = uniqueNonEmpty(keys)
	if len(keys) == 0 {
		return true
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	l.cleanupExpired(now)
	for _, key := range keys {
		entry, exists := l.entries[key]
		if exists && now.Sub(entry.windowStart) < authRateLimitWindow && entry.attempts >= authRateLimitMaxAttempts {
			return false
		}
	}
	for _, key := range keys {
		entry := l.entries[key]
		if entry.windowStart.IsZero() || now.Sub(entry.windowStart) >= authRateLimitWindow {
			entry = authRateLimitEntry{windowStart: now}
		}
		entry.attempts++
		l.entries[key] = entry
	}
	return true
}

func (l *authRateLimiter) reset(keys ...string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, key := range uniqueNonEmpty(keys) {
		delete(l.entries, key)
	}
}

func (l *authRateLimiter) cleanupExpired(now time.Time) {
	for key, entry := range l.entries {
		if now.Sub(entry.windowStart) >= authRateLimitWindow {
			delete(l.entries, key)
		}
	}
}

func uniqueNonEmpty(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func requestClientIP(request *http.Request) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(request.RemoteAddr))
	if err != nil || host == "" {
		host = strings.TrimSpace(request.RemoteAddr)
	}
	remoteIP := net.ParseIP(host)

	if trustProxyHeaders() {
		if config, configErr := proxyHeaderConfigFromEnv(); configErr == nil && remoteIP != nil && isTrustedProxy(remoteIP, config.trustedNetworks) {
			if forwarded, valid := parseForwardedIPs(request.Header.Get("X-Forwarded-For")); valid && len(forwarded) >= config.trustedHops {
				// X-Forwarded-For is ordered from the original client to the
				// nearest proxy. Select from the right so client-supplied values
				// cannot replace the address added by the trusted proxy.
				return forwarded[len(forwarded)-config.trustedHops].String()
			}
			if realIP := net.ParseIP(strings.TrimSpace(request.Header.Get("X-Real-IP"))); realIP != nil && strings.TrimSpace(request.Header.Get("X-Forwarded-For")) == "" {
				return realIP.String()
			}
		}
	}
	return host
}

func validateProxyHeaderConfig() error {
	if !trustProxyHeaders() {
		return nil
	}
	_, err := proxyHeaderConfigFromEnv()
	return err
}

func proxyHeaderConfigFromEnv() (proxyHeaderConfig, error) {
	if !trustProxyHeaders() {
		return proxyHeaderConfig{}, nil
	}

	rawNetworks := strings.TrimSpace(os.Getenv("TRUSTED_PROXY_CIDRS"))
	if rawNetworks == "" {
		return proxyHeaderConfig{}, &proxyConfigError{message: "TRUSTED_PROXY_CIDRS must be set when TRUST_PROXY_HEADERS is enabled"}
	}
	trustedNetworks, err := parseTrustedProxyNetworks(rawNetworks)
	if err != nil {
		return proxyHeaderConfig{}, err
	}

	rawHops := strings.TrimSpace(os.Getenv("TRUSTED_PROXY_HOPS"))
	trustedHops, err := strconv.Atoi(rawHops)
	if err != nil || trustedHops < 1 || trustedHops > maxTrustedProxyHops {
		return proxyHeaderConfig{}, &proxyConfigError{message: "TRUSTED_PROXY_HOPS must be between 1 and 16"}
	}
	return proxyHeaderConfig{trustedNetworks: trustedNetworks, trustedHops: trustedHops}, nil
}

type proxyConfigError struct {
	message string
}

func (e *proxyConfigError) Error() string {
	return e.message
}

func parseTrustedProxyNetworks(raw string) ([]*net.IPNet, error) {
	parts := strings.Split(raw, ",")
	networks := make([]*net.IPNet, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, &proxyConfigError{message: "TRUSTED_PROXY_CIDRS contains an empty entry"}
		}
		if ip := net.ParseIP(part); ip != nil {
			bits := 128
			if ipv4 := ip.To4(); ipv4 != nil {
				ip = ipv4
				bits = 32
			}
			networks = append(networks, &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)})
			continue
		}
		_, network, err := net.ParseCIDR(part)
		if err != nil {
			return nil, &proxyConfigError{message: "TRUSTED_PROXY_CIDRS contains an invalid IP or CIDR"}
		}
		networks = append(networks, network)
	}
	return networks, nil
}

func isTrustedProxy(ip net.IP, networks []*net.IPNet) bool {
	for _, network := range networks {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

func parseForwardedIPs(raw string) ([]net.IP, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, true
	}
	parts := strings.Split(raw, ",")
	addresses := make([]net.IP, 0, len(parts))
	for _, part := range parts {
		ip := net.ParseIP(strings.TrimSpace(part))
		if ip == nil {
			return nil, false
		}
		addresses = append(addresses, ip)
	}
	return addresses, true
}

func trustProxyHeaders() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("TRUST_PROXY_HEADERS"))) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}

func authRateLimitKeys(request *http.Request, email string) []string {
	return []string{"ip:" + requestClientIP(request), "email:" + normalizeEmail(email)}
}

func writeAuthRateLimitError(writer http.ResponseWriter) {
	writer.Header().Set("Retry-After", "900")
	writeError(writer, http.StatusTooManyRequests, "too many authentication attempts")
}
