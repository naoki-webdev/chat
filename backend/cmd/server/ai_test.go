package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAIRequestDailyLimit(t *testing.T) {
	server := newServer()
	server.aiDailyLimit = 1
	allowed, err := server.acquireAI(context.Background(), "u-daily:general")
	if err != nil || !allowed {
		t.Fatal("first AI request should be allowed")
	}
	server.releaseAI("u-daily:general")
	server.aiLastRun["u-daily:other"] = time.Now().Add(-2 * aiMinInterval)
	allowed, err = server.acquireAI(context.Background(), "u-daily:other")
	if err != nil {
		t.Fatalf("daily quota check failed: %v", err)
	}
	if allowed {
		t.Fatal("second AI request should be blocked by the daily user limit")
	}
}

func TestOrbitAIStreamsAndPersistsResponse(t *testing.T) {
	server := newServer()
	handler := server.handler()
	cookie := loginTestUser(t, handler, "demo@example.com")
	payload, _ := json.Marshal(messageRequest{Body: "今日の会話をまとめて"})
	createRecorder := httptest.NewRecorder()
	handler.ServeHTTP(createRecorder, authorizedRequest(http.MethodPost, "/api/channels/orbit-ai/messages", payload, cookie))
	if createRecorder.Code != http.StatusCreated {
		t.Fatalf("create Orbit AI prompt status = %d, body = %s", createRecorder.Code, createRecorder.Body.String())
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		messagesRecorder := httptest.NewRecorder()
		handler.ServeHTTP(messagesRecorder, authorizedRequest(http.MethodGet, "/api/channels/orbit-ai/messages", nil, cookie))
		if messagesRecorder.Code != http.StatusOK {
			t.Fatalf("list Orbit AI messages status = %d", messagesRecorder.Code)
		}
		var page MessagePage
		if err := json.NewDecoder(messagesRecorder.Body).Decode(&page); err != nil {
			t.Fatal(err)
		}
		for _, message := range page.Messages {
			if message.Author == "Orbit AI" && strings.Contains(message.Body, "Orbit AI（デモ）") {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("Orbit AI response was not persisted")
}
