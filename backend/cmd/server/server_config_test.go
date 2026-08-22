package main

import (
	"context"
	"testing"
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
