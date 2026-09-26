package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"realtime-chat/backend/internal/ai"
)

type immediateAIService struct{}

func (immediateAIService) Stream(_ context.Context, _ []ai.Message, _ string, onDelta func(string) error) (string, error) {
	response := "AI thread response"
	if err := onDelta(response); err != nil {
		return "", err
	}
	return response, nil
}

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

func TestOrbitAIReplyKeepsTheOriginalThread(t *testing.T) {
	server := newServer()
	server.aiService = immediateAIService{}
	ctx := context.Background()
	root, _, err := server.repository.CreateMessage(ctx, "general", "u-ken", messageRequest{Body: "thread root"})
	if err != nil {
		t.Fatal(err)
	}
	prompt, _, err := server.repository.CreateMessage(ctx, "general", "u-naoki", messageRequest{Body: "ask AI", ParentMessageID: root.ID})
	if err != nil {
		t.Fatal(err)
	}

	server.startAIReply("request", "general", "u-naoki", prompt)

	thread, err := server.repository.ListThreadPage(ctx, root.ID, "", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(thread.Messages) != 2 {
		t.Fatalf("thread messages = %d, want 2", len(thread.Messages))
	}
	answer := thread.Messages[1]
	if answer.AuthorID != orbitAIUserID || answer.ParentMessageID != root.ID || answer.Body != "AI thread response" {
		t.Fatalf("unexpected AI thread message: %+v", answer)
	}
}
