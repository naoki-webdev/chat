package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func openPostgresAuthTestRepository(t *testing.T) (*postgresRepository, context.Context) {
	t.Helper()
	t.Setenv("APP_ENV", "test")
	databaseURL := strings.TrimSpace(os.Getenv("POSTGRES_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("POSTGRES_TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	repository, err := newPostgresRepository(ctx, databaseURL)
	if err != nil {
		cancel()
		t.Fatalf("connect to PostgreSQL: %v", err)
	}
	t.Cleanup(func() {
		cancel()
		repository.Close()
	})
	return repository, ctx
}

func TestPostgresAuthenticateReturnsDatabaseError(t *testing.T) {
	repository, _ := openPostgresAuthTestRepository(t)
	requestContext, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := repository.AuthenticateUser(requestContext, "demo@example.com", "demo-password")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("authentication error = %v, want context canceled", err)
	}
}

func TestPostgresLoginDoesNotPresentDatabaseErrorAsInvalidCredentials(t *testing.T) {
	repository, _ := openPostgresAuthTestRepository(t)
	application := newServerWithRepository(repository)
	requestContext, cancel := context.WithCancel(context.Background())
	cancel()
	request := httptest.NewRequestWithContext(requestContext, http.MethodPost, "/api/auth/login", strings.NewReader(`{"email":"demo@example.com","password":"demo-password"}`))
	recorder := httptest.NewRecorder()

	application.handleLogin(recorder, request)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("login status = %d, want %d; body = %s", recorder.Code, http.StatusInternalServerError, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "INVALID_CREDENTIALS") {
		t.Fatalf("database error was presented as invalid credentials: %s", recorder.Body.String())
	}
}

func TestPostgresConcurrentHandleGeneration(t *testing.T) {
	repository, ctx := openPostgresAuthTestRepository(t)
	emailPrefix := "pg-handle-race-" + randomID()
	name := "Handle Race " + randomID()
	cleanup := func() {
		cleanupContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = repository.pool.Exec(cleanupContext, `DELETE FROM users WHERE email LIKE $1`, emailPrefix+"-%@example.com")
	}
	t.Cleanup(cleanup)

	const registrations = 16
	start := make(chan struct{})
	results := make(chan struct {
		user User
		err  error
	}, registrations)
	var waitGroup sync.WaitGroup
	for index := 0; index < registrations; index++ {
		waitGroup.Add(1)
		go func(index int) {
			defer waitGroup.Done()
			<-start
			user, err := repository.RegisterUser(ctx, registerRequest{
				Name:     name,
				Email:    fmt.Sprintf("%s-%d@example.com", emailPrefix, index),
				Password: "test-password",
			})
			results <- struct {
				user User
				err  error
			}{user: user, err: err}
		}(index)
	}
	close(start)
	waitGroup.Wait()
	close(results)

	users := make([]User, 0, registrations)
	for result := range results {
		if result.err != nil {
			t.Fatalf("concurrent registration failed: %v", result.err)
		}
		users = append(users, result.user)
	}
	if len(users) != registrations {
		t.Fatalf("registered users = %d, want %d", len(users), registrations)
	}
	assertUniqueUserHandles(t, users)

	profileName := "Profile Race " + randomID()
	start = make(chan struct{})
	profileResults := make(chan error, 2)
	var profileWaitGroup sync.WaitGroup
	for _, user := range users[:2] {
		profileWaitGroup.Add(1)
		go func(userID string) {
			defer profileWaitGroup.Done()
			<-start
			_, err := repository.UpdateUserProfile(ctx, userID, updateProfileRequest{Name: profileName})
			profileResults <- err
		}(user.ID)
	}
	close(start)
	profileWaitGroup.Wait()
	close(profileResults)
	for err := range profileResults {
		if err != nil {
			t.Fatalf("concurrent profile update failed: %v", err)
		}
	}
}

func assertUniqueUserHandles(t *testing.T, users []User) {
	t.Helper()
	handles := make(map[string]struct{}, len(users))
	for _, user := range users {
		if _, exists := handles[user.Handle]; exists {
			t.Fatalf("duplicate user handle generated: %q", user.Handle)
		}
		handles[user.Handle] = struct{}{}
	}
}
