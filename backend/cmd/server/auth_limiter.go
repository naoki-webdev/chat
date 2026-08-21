package main

import (
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	authRateLimitMaxAttempts = 5
	authRateLimitWindow      = 15 * time.Minute
)

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
	if trustProxyHeaders() {
		if forwarded := strings.TrimSpace(strings.Split(request.Header.Get("X-Forwarded-For"), ",")[0]); net.ParseIP(forwarded) != nil {
			return forwarded
		}
		if realIP := strings.TrimSpace(request.Header.Get("X-Real-IP")); net.ParseIP(realIP) != nil {
			return realIP
		}
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(request.RemoteAddr))
	if err == nil && host != "" {
		return host
	}
	return strings.TrimSpace(request.RemoteAddr)
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
