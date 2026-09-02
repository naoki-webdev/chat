package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealthAndAuthentication(t *testing.T) {
	server := newServer()
	handler := server.handler()
	healthRequest := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	healthRecorder := httptest.NewRecorder()
	handler.ServeHTTP(healthRecorder, healthRequest)
	if healthRecorder.Code != http.StatusOK {
		t.Fatalf("health status = %d, want %d", healthRecorder.Code, http.StatusOK)
	}

	unauthorizedRequest := httptest.NewRequest(http.MethodGet, "/api/channels", nil)
	unauthorizedRecorder := httptest.NewRecorder()
	handler.ServeHTTP(unauthorizedRecorder, unauthorizedRequest)
	if unauthorizedRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d, want %d", unauthorizedRecorder.Code, http.StatusUnauthorized)
	}

	cookie := registerTestUser(t, handler, "test-auth@example.com")
	botLoginPayload, _ := json.Marshal(loginRequest{Email: "orbit-ai@local", Password: "demo-password"})
	botLoginRecorder := httptest.NewRecorder()
	handler.ServeHTTP(botLoginRecorder, httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(botLoginPayload)))
	if botLoginRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("bot login status = %d, want %d", botLoginRecorder.Code, http.StatusUnauthorized)
	}

	authorizedRecorder := httptest.NewRecorder()
	handler.ServeHTTP(authorizedRecorder, authorizedRequest(http.MethodGet, "/api/channels", nil, cookie))
	if authorizedRecorder.Code != http.StatusOK {
		t.Fatalf("authorized status = %d, want %d", authorizedRecorder.Code, http.StatusOK)
	}
}

func TestUpdateUserProfile(t *testing.T) {
	server := newServer()
	handler := server.handler()
	cookie := registerTestUser(t, handler, "profile@example.com")
	payload, _ := json.Marshal(updateProfileRequest{Name: "Updated Profile"})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, authorizedRequest(http.MethodPatch, "/api/auth/me", payload, cookie))
	if recorder.Code != http.StatusOK {
		t.Fatalf("profile update status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var response map[string]User
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode updated profile: %v", err)
	}
	if response["user"].Name != "Updated Profile" || response["user"].Initials != "UP" {
		t.Fatalf("unexpected updated profile: %+v", response["user"])
	}

	meRecorder := httptest.NewRecorder()
	handler.ServeHTTP(meRecorder, authorizedRequest(http.MethodGet, "/api/auth/me", nil, cookie))
	var meResponse map[string]User
	if err := json.NewDecoder(meRecorder.Body).Decode(&meResponse); err != nil {
		t.Fatalf("decode current profile: %v", err)
	}
	if meResponse["user"].Name != "Updated Profile" {
		t.Fatalf("updated profile was not persisted: %+v", meResponse["user"])
	}
}

func TestMemoryRegistrationAndProfileUpdateKeepHandlesUnique(t *testing.T) {
	server := newServer()
	first := registerTestUser(t, server.handler(), "same-name-1@example.com")
	second := registerTestUser(t, server.handler(), "same-name-2@example.com")

	firstUser, err := server.repository.FindUserBySession(context.Background(), first.Value)
	if err != nil {
		t.Fatalf("find first user: %v", err)
	}
	secondUser, err := server.repository.FindUserBySession(context.Background(), second.Value)
	if err != nil {
		t.Fatalf("find second user: %v", err)
	}
	if firstUser.Handle == secondUser.Handle {
		t.Fatalf("registered users share handle %q", firstUser.Handle)
	}

	updated, err := server.repository.UpdateUserProfile(context.Background(), secondUser.ID, updateProfileRequest{Name: firstUser.Name})
	if err != nil {
		t.Fatalf("update profile: %v", err)
	}
	if updated.Handle == firstUser.Handle {
		t.Fatalf("profile update reused handle %q", updated.Handle)
	}
}

func TestMemoryProfileRenameKeepsDMIdentityStable(t *testing.T) {
	repository := newMemoryRepository()
	ctx := context.Background()

	if _, err := repository.UpdateUserProfile(ctx, "u-ayaka", updateProfileRequest{Name: "Ayaka One"}); err != nil {
		t.Fatalf("first profile update: %v", err)
	}
	if _, err := repository.UpdateUserProfile(ctx, "u-ayaka", updateProfileRequest{Name: "Ayaka Two"}); err != nil {
		t.Fatalf("second profile update: %v", err)
	}
	if _, err := repository.UpdateUserProfile(ctx, "u-naoki", updateProfileRequest{Name: "Taro Renamed"}); err != nil {
		t.Fatalf("own profile update: %v", err)
	}

	channels, _, err := repository.ListChannels(ctx, "u-naoki")
	if err != nil {
		t.Fatalf("list channels: %v", err)
	}
	byID := make(map[string]Channel, len(channels))
	for _, channel := range channels {
		byID[channel.ID] = channel
	}
	if byID["ayaka"].Name != "Ayaka Two" || byID["ayaka"].PeerUserID != "u-ayaka" {
		t.Fatalf("renamed Ayaka DM = %+v", byID["ayaka"])
	}
	if byID["ken"].Name != "Ken Ito" || byID["orbit-ai"].Name != "Orbit AI" {
		t.Fatalf("unrelated DMs changed after profile rename: ken=%+v orbit=%+v", byID["ken"], byID["orbit-ai"])
	}
}

func TestPasswordValidationUsesBcryptByteLimit(t *testing.T) {
	tests := []struct {
		name     string
		password string
		valid    bool
	}{
		{name: "minimum length", password: "password", valid: true},
		{name: "ascii at limit", password: strings.Repeat("a", maxPasswordBytes), valid: true},
		{name: "ascii over limit", password: strings.Repeat("a", maxPasswordBytes+1), valid: false},
		{name: "utf8 at limit", password: strings.Repeat("あ", 24), valid: true},
		{name: "utf8 over limit", password: strings.Repeat("あ", 25), valid: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := validPassword(test.password); got != test.valid {
				t.Fatalf("validPassword(%q) = %t, want %t", test.password, got, test.valid)
			}
		})
	}
}

func TestRegistrationRejectsPasswordOverBcryptByteLimit(t *testing.T) {
	server := newServer()
	recorder := httptest.NewRecorder()
	payload, err := json.Marshal(registerRequest{
		Name:     "Long Password",
		Email:    "long-password@example.com",
		Password: strings.Repeat("あ", 25),
	})
	if err != nil {
		t.Fatalf("marshal registration payload: %v", err)
	}
	server.handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/auth/register", bytes.NewReader(payload)))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("registration status = %d, body = %s, want %d", recorder.Code, recorder.Body.String(), http.StatusBadRequest)
	}
	if !strings.Contains(recorder.Body.String(), "72 bytes") {
		t.Fatalf("registration error = %s, want byte-limit message", recorder.Body.String())
	}
}

func TestAuthenticationRateLimit(t *testing.T) {
	server := newServer()
	handler := server.handler()
	payload, _ := json.Marshal(loginRequest{Email: "unknown@example.com", Password: "wrong-password"})
	for attempt := 0; attempt < authRateLimitMaxAttempts; attempt++ {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(payload)))
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("failed login attempt %d status = %d", attempt+1, recorder.Code)
		}
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(payload)))
	if recorder.Code != http.StatusTooManyRequests || recorder.Header().Get("Retry-After") == "" {
		t.Fatalf("rate limit response = %d, retry-after = %q", recorder.Code, recorder.Header().Get("Retry-After"))
	}
}

func TestRequestClientIPOnlyTrustsForwardedHeadersWhenConfigured(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.RemoteAddr = "192.0.2.10:1234"
	request.Header.Set("X-Forwarded-For", "198.51.100.20")
	t.Setenv("TRUST_PROXY_HEADERS", "false")
	if got := requestClientIP(request); got != "192.0.2.10" {
		t.Fatalf("untrusted forwarded address = %q, want remote address", got)
	}
	t.Setenv("TRUST_PROXY_HEADERS", "true")
	t.Setenv("TRUSTED_PROXY_CIDRS", "203.0.113.0/24")
	t.Setenv("TRUSTED_PROXY_HOPS", "1")
	if got := requestClientIP(request); got != "192.0.2.10" {
		t.Fatalf("forwarded address from an untrusted peer = %q, want remote address", got)
	}

	request.RemoteAddr = "203.0.113.10:1234"
	request.Header.Set("X-Forwarded-For", "192.0.2.99, 198.51.100.20")
	if got := requestClientIP(request); got != "198.51.100.20" {
		t.Fatalf("trusted forwarded address = %q, want rightmost forwarded address", got)
	}

	request.Header.Set("X-Forwarded-For", "198.51.100.20, 203.0.113.11")
	t.Setenv("TRUSTED_PROXY_HOPS", "2")
	if got := requestClientIP(request); got != "198.51.100.20" {
		t.Fatalf("multi-hop forwarded address = %q, want client address", got)
	}

	request.Header.Set("X-Forwarded-For", "not-an-ip")
	if got := requestClientIP(request); got != "203.0.113.10" {
		t.Fatalf("malformed forwarded address = %q, want remote address", got)
	}

	request.Header.Set("X-Forwarded-For", "")
	request.Header.Set("X-Real-IP", "198.51.100.20")
	if got := requestClientIP(request); got != "198.51.100.20" {
		t.Fatalf("trusted real address = %q, want real-ip address", got)
	}
}

func TestProductionRequiresTrustedProxyConfiguration(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("TRUST_PROXY_HEADERS", "true")
	t.Setenv("TRUSTED_PROXY_CIDRS", "")
	t.Setenv("TRUSTED_PROXY_HOPS", "")
	if err := validateProxyHeaderConfig(); err == nil {
		t.Fatal("proxy headers should require trusted proxy CIDRs")
	}

	t.Setenv("TRUSTED_PROXY_CIDRS", "not-a-cidr")
	t.Setenv("TRUSTED_PROXY_HOPS", "1")
	if err := validateProxyHeaderConfig(); err == nil {
		t.Fatal("invalid trusted proxy CIDRs should be rejected")
	}

	t.Setenv("TRUSTED_PROXY_CIDRS", "203.0.113.0/24, 2001:db8::1")
	t.Setenv("TRUSTED_PROXY_HOPS", "1")
	if err := validateProxyHeaderConfig(); err != nil {
		t.Fatalf("valid proxy configuration rejected: %v", err)
	}
}
