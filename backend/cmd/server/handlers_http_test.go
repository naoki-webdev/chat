package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLoginMethodNotAllowedAdvertisesOnlyPost(t *testing.T) {
	server := newServer()
	request := httptest.NewRequest(http.MethodGet, "/api/auth/login", nil)
	recorder := httptest.NewRecorder()

	server.handleLogin(recorder, request)

	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusMethodNotAllowed)
	}
	if got := recorder.Header().Get("Allow"); got != http.MethodPost {
		t.Fatalf("Allow = %q, want %q", got, http.MethodPost)
	}
}

func TestHealthMethodNotAllowedAdvertisesOnlyGet(t *testing.T) {
	server := newServer()
	request := httptest.NewRequest(http.MethodPost, "/api/health", nil)
	recorder := httptest.NewRecorder()

	server.handleHealth(recorder, request)

	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusMethodNotAllowed)
	}
	if got := recorder.Header().Get("Allow"); got != http.MethodGet {
		t.Fatalf("Allow = %q, want %q", got, http.MethodGet)
	}
}
