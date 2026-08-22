package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"realtime-chat/backend/internal/ai"
)

type summaryTestService struct{}

func (summaryTestService) Stream(_ context.Context, history []ai.Message, _ string, onDelta func(string) error) (string, error) {
	sourceID := ""
	if len(history) > 0 {
		sourceID, _ = summarySource(history[0].Body)
	}
	response := fmt.Sprintf(`{"summary":"会話の要点を整理しました。","decisions":[{"text":"PostgreSQLを採用","source_message_id":"%s"}],"action_items":[{"text":"API仕様を確認","source_message_id":"%s"}],"unresolved":[{"text":"幻の話題","source_message_id":"hallucinated-message-id"}],"chatter_count":0}`, sourceID, sourceID)
	if err := onDelta(response); err != nil {
		return "", err
	}
	return response, nil
}

func TestUnreadAIContextKeepsNewestMessagesAndExactCount(t *testing.T) {
	repository := newMemoryRepository()
	ctx := context.Background()
	if _, _, err := repository.CreateMessage(ctx, "general", "u-naoki", messageRequest{Body: "unread context oldest"}); err != nil {
		t.Fatalf("create oldest unread message: %v", err)
	}
	if _, _, err := repository.CreateMessage(ctx, "general", "u-naoki", messageRequest{Body: "unread context newest"}); err != nil {
		t.Fatalf("create newest unread message: %v", err)
	}
	items, unreadCount, err := repository.ListUnreadMessageContext(ctx, "u-naoki", "general", 1)
	if err != nil {
		t.Fatalf("list unread AI context: %v", err)
	}
	if unreadCount != 4 {
		t.Fatalf("unread count = %d, want 4", unreadCount)
	}
	if len(items) != 1 || items[0].Body != "unread context newest" {
		t.Fatalf("bounded unread context = %+v", items)
	}
}

func TestChannelSummaryEndpoint(t *testing.T) {
	server := newServerWithRepositoryAndAI(newMemoryRepository(), summaryTestService{})
	handler := server.handler()
	cookie := registerTestUser(t, handler, "summary@example.com")

	for _, body := range []string{"PostgreSQLを採用することに決定しました。", "API仕様を確認してください。", "認証方式はまだ検討中です。"} {
		payload, _ := json.Marshal(messageRequest{Body: body})
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, authorizedRequest(http.MethodPost, "/api/channels/general/messages", payload, cookie))
		if recorder.Code != http.StatusCreated {
			t.Fatalf("message status = %d, body = %s", recorder.Code, recorder.Body.String())
		}
	}
	user, err := server.repository.FindUserBySession(context.Background(), cookie.Value)
	if err != nil {
		t.Fatalf("find summary test user: %v", err)
	}
	root, _, err := server.repository.CreateMessage(context.Background(), "general", user.ID, messageRequest{Body: "thread root: API設計"})
	if err != nil {
		t.Fatalf("create summary thread root: %v", err)
	}
	reply, _, err := server.repository.CreateMessage(context.Background(), "general", user.ID, messageRequest{Body: "thread reply: 認証方式はCookieに決定", ParentMessageID: root.ID})
	if err != nil {
		t.Fatalf("create summary thread reply: %v", err)
	}
	unread, err := server.repository.ListUnreadMessages(context.Background(), user.ID, "general")
	if err != nil {
		t.Fatalf("list unread summary messages: %v", err)
	}
	if !containsMessageID(unread, reply.ID) {
		t.Fatalf("unread summary context omitted thread reply: %+v", unread)
	}
	aiContext, err := server.repository.ListAIContextMessages(context.Background(), "general", 100)
	if err != nil {
		t.Fatalf("list AI context messages: %v", err)
	}
	if !containsMessageID(aiContext, reply.ID) {
		t.Fatalf("AI context omitted thread reply: %+v", aiContext)
	}

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, authorizedRequest(http.MethodPost, "/api/channels/general/summary", nil, cookie))
	if recorder.Code != http.StatusOK {
		t.Fatalf("summary status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var summary channelSummary
	if err := json.NewDecoder(recorder.Body).Decode(&summary); err != nil {
		t.Fatal(err)
	}
	if summary.Scope != "unread" || summary.MessageCount < 3 || summary.Summary == "" {
		t.Fatalf("unexpected summary metadata: %+v", summary)
	}
	if len(summary.Decisions) != 1 || len(summary.ActionItems) != 1 || len(summary.Unresolved) != 1 {
		t.Fatalf("unexpected summary categories: %+v", summary)
	}
	if summary.Decisions[0].SourceMessageID == "" {
		t.Fatal("summary item should retain its source message ID")
	}
	if summary.Unresolved[0].SourceMessageID != "" {
		t.Fatalf("hallucinated summary source ID was not removed: %+v", summary.Unresolved[0])
	}
}

func containsMessageID(messages []Message, messageID string) bool {
	for _, message := range messages {
		if message.ID == messageID {
			return true
		}
	}
	return false
}
