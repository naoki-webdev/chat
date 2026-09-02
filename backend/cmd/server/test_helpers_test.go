package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func registerTestUser(t *testing.T, handler http.Handler, email string) *http.Cookie {
	t.Helper()
	payload, _ := json.Marshal(registerRequest{Name: "Test User", Email: email, Password: "test-password"})
	request := httptest.NewRequest(http.MethodPost, "/api/auth/register", bytes.NewReader(payload))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("register status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	cookies := recorder.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected one session cookie, got %d", len(cookies))
	}
	return cookies[0]
}

func loginTestUser(t *testing.T, handler http.Handler, email string) *http.Cookie {
	t.Helper()
	payload, _ := json.Marshal(loginRequest{Email: email, Password: "demo-password"})
	request := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(payload))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("login status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	cookies := recorder.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected one session cookie, got %d", len(cookies))
	}
	return cookies[0]
}

func authorizedRequest(method, path string, body []byte, cookie *http.Cookie) *http.Request {
	request := httptest.NewRequest(method, path, bytes.NewReader(body))
	request.AddCookie(cookie)
	return request
}
