package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const sessionCookieName = "orbit_session"

func randomToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func tokenHash(token string) string {
	hash := sha256.Sum256([]byte(token))
	return base64.RawURLEncoding.EncodeToString(hash[:])
}

func hashPassword(password string) (string, error) {
	result, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(result), err
}

func validPassword(password string) bool {
	return len([]rune(password)) >= 8
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func secureCookiesEnabled() bool {
	value := strings.TrimSpace(os.Getenv("COOKIE_SECURE"))
	return strings.EqualFold(value, "true") || value == "1"
}

func validateCookieConfig() error {
	if !isLocalEnvironment() && !secureCookiesEnabled() {
		return errors.New("COOKIE_SECURE must be true outside development and test environments")
	}
	return nil
}

func sessionCookie(token string) *http.Cookie {
	return &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secureCookiesEnabled(),
		MaxAge:   int((7 * 24 * time.Hour) / time.Second),
	}
}

func expiredSessionCookie() *http.Cookie {
	cookie := sessionCookie("")
	cookie.MaxAge = -1
	cookie.Expires = time.Unix(1, 0)
	return cookie
}

func (s *server) currentUser(request *http.Request) (User, error) {
	cookie, err := request.Cookie(sessionCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return User{}, ErrUnauthorized
	}
	return s.repository.FindUserBySession(request.Context(), cookie.Value)
}

func (s *server) requireUser(writer http.ResponseWriter, request *http.Request) (User, bool) {
	user, err := s.currentUser(request)
	if err != nil {
		writeError(writer, http.StatusUnauthorized, "authentication required")
		return User{}, false
	}
	return user, true
}

func validateRegistration(payload registerRequest) error {
	if strings.TrimSpace(payload.Name) == "" {
		return invalidInput("name is required")
	}
	if !strings.Contains(normalizeEmail(payload.Email), "@") {
		return invalidInput("a valid email is required")
	}
	if !validPassword(payload.Password) {
		return invalidInput("password must be at least 8 characters")
	}
	return nil
}
