package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCORSRejectsDisallowedStateChangingOrigin(t *testing.T) {
	t.Setenv("FRONTEND_ORIGIN", "http://127.0.0.1:4174")
	nextCalled := false
	handler := withCORS(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		nextCalled = true
		writer.WriteHeader(http.StatusNoContent)
	}))

	request := httptest.NewRequest(http.MethodPost, "/api/messages", nil)
	request.Header.Set("Origin", "https://evil.example")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("disallowed origin status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
	if nextCalled {
		t.Fatal("disallowed state-changing request reached the handler")
	}
}

func TestCORSAllowsConfiguredOriginAndSameOriginRequests(t *testing.T) {
	t.Setenv("FRONTEND_ORIGIN", "http://127.0.0.1:4174")
	handler := withCORS(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	}))

	allowed := httptest.NewRequest(http.MethodPost, "/api/messages", nil)
	allowed.Header.Set("Origin", "http://127.0.0.1:4174")
	allowedRecorder := httptest.NewRecorder()
	handler.ServeHTTP(allowedRecorder, allowed)
	if allowedRecorder.Code != http.StatusNoContent || allowedRecorder.Header().Get("Access-Control-Allow-Origin") != "http://127.0.0.1:4174" {
		t.Fatalf("configured origin response = %d, allow-origin = %q", allowedRecorder.Code, allowedRecorder.Header().Get("Access-Control-Allow-Origin"))
	}

	sameOrigin := httptest.NewRequest(http.MethodPost, "/api/messages", nil)
	sameOriginRecorder := httptest.NewRecorder()
	handler.ServeHTTP(sameOriginRecorder, sameOrigin)
	if sameOriginRecorder.Code != http.StatusNoContent {
		t.Fatalf("request without Origin status = %d, want %d", sameOriginRecorder.Code, http.StatusNoContent)
	}
}
