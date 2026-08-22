package main

import (
	"errors"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		origin := request.Header.Get("Origin")
		if isAllowedOrigin(origin) {
			writer.Header().Set("Access-Control-Allow-Origin", origin)
			writer.Header().Set("Vary", "Origin")
			writer.Header().Set("Access-Control-Allow-Credentials", "true")
		}
		writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		if request.Method == http.MethodOptions {
			writer.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(writer, request)
	})
}

func isAllowedOrigin(origin string) bool {
	if origin == "" {
		return false
	}
	configured := strings.TrimSpace(os.Getenv("FRONTEND_ORIGIN"))
	if configured != "" {
		return origin == configured
	}
	return origin == "http://127.0.0.1:4174" || origin == "http://localhost:4174" || origin == "http://127.0.0.1:5173" || origin == "http://localhost:5173"
}

func validateFrontendOrigin() error {
	origin := strings.TrimSpace(os.Getenv("FRONTEND_ORIGIN"))
	if origin == "" {
		return errors.New("FRONTEND_ORIGIN must be set outside development and test environments")
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.User != nil {
		return errors.New("FRONTEND_ORIGIN must be an absolute http or https origin without a path")
	}
	return nil
}

func isLocalEnvironment() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV"))) {
	case "development", "dev", "test":
		return true
	default:
		return false
	}
}

func withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		started := time.Now()
		next.ServeHTTP(writer, request)
		log.Printf("%s %s (%s)", request.Method, request.URL.Path, time.Since(started).Round(time.Millisecond))
	})
}
