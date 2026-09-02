package main

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestProductionServerRequiresDatabaseURL(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("DATABASE_URL", "")
	if _, err := newProductionServer(context.Background()); err == nil {
		t.Fatal("production server should fail when DATABASE_URL is missing")
	}
}

func TestProductionRequiresSecureCookies(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("COOKIE_SECURE", "")
	if err := validateCookieConfig(); err == nil {
		t.Fatal("production should require secure cookies")
	}

	t.Setenv("COOKIE_SECURE", "true")
	if err := validateCookieConfig(); err != nil {
		t.Fatalf("secure cookie configuration rejected: %v", err)
	}
	if !sessionCookie("token").Secure {
		t.Fatal("session cookie should be secure")
	}
}

func TestProductionRequiresFrontendOrigin(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("FRONTEND_ORIGIN", "")
	if err := validateFrontendOrigin(); err == nil {
		t.Fatal("production should require a frontend origin")
	}

	t.Setenv("FRONTEND_ORIGIN", "https://chat.example.com/app")
	if err := validateFrontendOrigin(); err == nil {
		t.Fatal("frontend origin with a path should be rejected")
	}

	t.Setenv("FRONTEND_ORIGIN", "https://chat.example.com")
	if err := validateFrontendOrigin(); err != nil {
		t.Fatalf("valid frontend origin rejected: %v", err)
	}
}

func TestHTTPServerTimeouts(t *testing.T) {
	server := newHTTPServer("8080", http.NotFoundHandler())
	if server.ReadHeaderTimeout != 5*time.Second {
		t.Fatalf("read header timeout = %s, want 5s", server.ReadHeaderTimeout)
	}
	if server.ReadTimeout != 15*time.Second {
		t.Fatalf("read timeout = %s, want 15s", server.ReadTimeout)
	}
	if server.IdleTimeout != 60*time.Second {
		t.Fatalf("idle timeout = %s, want 60s", server.IdleTimeout)
	}
	if server.WriteTimeout != 0 {
		t.Fatalf("write timeout = %s, want zero for route-specific/WebSocket deadlines", server.WriteTimeout)
	}
}
