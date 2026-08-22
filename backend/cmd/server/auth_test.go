package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	if got := requestClientIP(request); got != "198.51.100.20" {
		t.Fatalf("trusted forwarded address = %q, want forwarded address", got)
	}
}
