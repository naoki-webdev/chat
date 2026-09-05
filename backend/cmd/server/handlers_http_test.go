package main

import (
	"encoding/json"
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

func TestMissingResourceRoutesReturnJSONErrors(t *testing.T) {
	server := newServer()
	handler := server.handler()
	cookie := loginTestUser(t, handler, "demo@example.com")

	for _, path := range []string{"/api/channels/missing/messages", "/api/messages/"} {
		request := authorizedRequest(http.MethodGet, path, nil, cookie)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusNotFound {
			t.Fatalf("%s status = %d, want %d", path, recorder.Code, http.StatusNotFound)
		}
		if got := recorder.Header().Get("Content-Type"); got != "application/json" {
			t.Fatalf("%s content type = %q, want application/json", path, got)
		}
		var payload struct {
			Code string `json:"code"`
		}
		if err := json.NewDecoder(recorder.Body).Decode(&payload); err != nil {
			t.Fatalf("%s response is not JSON: %v", path, err)
		}
		if payload.Code != "NOT_FOUND" {
			t.Fatalf("%s error code = %q, want NOT_FOUND", path, payload.Code)
		}
	}
}
