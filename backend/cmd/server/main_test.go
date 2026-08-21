package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
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

func TestHealthAndAuthentication(t *testing.T) {
	server := newServer()
	handler := server.handler()
	healthRequest := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	healthRecorder := httptest.NewRecorder()
	handler.ServeHTTP(healthRecorder, healthRequest)
	if healthRecorder.Code != http.StatusOK {
		t.Fatalf("health status = %d, want %d", healthRecorder.Code, http.StatusOK)
	}

	unauthorizedRequest := httptest.NewRequest(http.MethodGet, "/api/channels", nil)
	unauthorizedRecorder := httptest.NewRecorder()
	handler.ServeHTTP(unauthorizedRecorder, unauthorizedRequest)
	if unauthorizedRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d, want %d", unauthorizedRecorder.Code, http.StatusUnauthorized)
	}

	cookie := registerTestUser(t, handler, "test-auth@example.com")
	botLoginPayload, _ := json.Marshal(loginRequest{Email: "orbit-ai@local", Password: "demo-password"})
	botLoginRecorder := httptest.NewRecorder()
	handler.ServeHTTP(botLoginRecorder, httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(botLoginPayload)))
	if botLoginRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("bot login status = %d, want %d", botLoginRecorder.Code, http.StatusUnauthorized)
	}

	authorizedRecorder := httptest.NewRecorder()
	handler.ServeHTTP(authorizedRecorder, authorizedRequest(http.MethodGet, "/api/channels", nil, cookie))
	if authorizedRecorder.Code != http.StatusOK {
		t.Fatalf("authorized status = %d, want %d", authorizedRecorder.Code, http.StatusOK)
	}
}

func TestSeededThreadCountMatchesReplies(t *testing.T) {
	repository := newMemoryRepository()
	thread, err := repository.ListThreadPage(context.Background(), "ds-1", "", 50)
	if err != nil {
		t.Fatalf("list seeded thread: %v", err)
	}
	if len(thread.Messages) != 3 {
		t.Fatalf("seeded reply count = %d, want 3", len(thread.Messages))
	}
	for _, message := range repository.messages["design-system"] {
		if message.ID == "ds-1" && message.ThreadCount != len(thread.Messages) {
			t.Fatalf("seeded thread count = %d, want %d", message.ThreadCount, len(thread.Messages))
		}
	}
}

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

func TestAuthenticationRateLimit(t *testing.T) {
	server := newServer()
	handler := server.handler()
	payload, _ := json.Marshal(loginRequest{Email: "unknown@example.com", Password: "wrong-password"})
	for attempt := 0; attempt < authRateLimitMaxAttempts; attempt++ {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(payload)))
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("failed login attempt %d status = %d", attempt+1, recorder.Code)
		}
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(payload)))
	if recorder.Code != http.StatusTooManyRequests || recorder.Header().Get("Retry-After") == "" {
		t.Fatalf("rate limit response = %d, retry-after = %q", recorder.Code, recorder.Header().Get("Retry-After"))
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

func TestInputLimitsAndThreadDepth(t *testing.T) {
	server := newServer()
	handler := server.handler()
	cookie := registerTestUser(t, handler, "limits@example.com")

	longMessage, _ := json.Marshal(messageRequest{Body: strings.Repeat("a", maxMessageBodyLength+1)})
	messageRecorder := httptest.NewRecorder()
	handler.ServeHTTP(messageRecorder, authorizedRequest(http.MethodPost, "/api/channels/general/messages", longMessage, cookie))
	if messageRecorder.Code != http.StatusBadRequest {
		t.Fatalf("long message status = %d, want %d", messageRecorder.Code, http.StatusBadRequest)
	}

	longChannel, _ := json.Marshal(channelRequest{Name: strings.Repeat("a", maxChannelNameLength+1)})
	channelRecorder := httptest.NewRecorder()
	handler.ServeHTTP(channelRecorder, authorizedRequest(http.MethodPost, "/api/channels", longChannel, cookie))
	if channelRecorder.Code != http.StatusBadRequest {
		t.Fatalf("long channel status = %d, want %d", channelRecorder.Code, http.StatusBadRequest)
	}

	rootPayload, _ := json.Marshal(messageRequest{Body: "thread root"})
	rootRecorder := httptest.NewRecorder()
	handler.ServeHTTP(rootRecorder, authorizedRequest(http.MethodPost, "/api/channels/general/messages", rootPayload, cookie))
	var root Message
	if rootRecorder.Code != http.StatusCreated || json.NewDecoder(rootRecorder.Body).Decode(&root) != nil {
		t.Fatalf("create root status = %d, body = %s", rootRecorder.Code, rootRecorder.Body.String())
	}

	replyPayload, _ := json.Marshal(messageRequest{Body: "thread reply", ParentMessageID: root.ID})
	replyRecorder := httptest.NewRecorder()
	handler.ServeHTTP(replyRecorder, authorizedRequest(http.MethodPost, "/api/channels/general/messages", replyPayload, cookie))
	var reply Message
	if replyRecorder.Code != http.StatusCreated || json.NewDecoder(replyRecorder.Body).Decode(&reply) != nil {
		t.Fatalf("create reply status = %d, body = %s", replyRecorder.Code, replyRecorder.Body.String())
	}

	nestedPayload, _ := json.Marshal(messageRequest{Body: "nested reply", ParentMessageID: reply.ID})
	nestedRecorder := httptest.NewRecorder()
	handler.ServeHTTP(nestedRecorder, authorizedRequest(http.MethodPost, "/api/channels/general/messages", nestedPayload, cookie))
	if nestedRecorder.Code != http.StatusConflict {
		t.Fatalf("nested reply status = %d, want %d", nestedRecorder.Code, http.StatusConflict)
	}

	longReaction, _ := json.Marshal(reactionRequest{Emoji: strings.Repeat("x", maxReactionLength+1)})
	reactionRecorder := httptest.NewRecorder()
	handler.ServeHTTP(reactionRecorder, authorizedRequest(http.MethodPost, "/api/messages/"+root.ID+"/reactions", longReaction, cookie))
	if reactionRecorder.Code != http.StatusBadRequest {
		t.Fatalf("long reaction status = %d, want %d", reactionRecorder.Code, http.StatusBadRequest)
	}
}

func TestRepositoryErrorDoesNotExposeInternalDetails(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeRepositoryError(recorder, errors.New("postgres password leaked"))
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("internal error status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
	if strings.Contains(recorder.Body.String(), "postgres password leaked") {
		t.Fatalf("internal error leaked in response: %s", recorder.Body.String())
	}
}

func TestMessageLifecycleAndOwnership(t *testing.T) {
	server := newServer()
	handler := server.handler()
	ownerCookie := registerTestUser(t, handler, "owner@example.com")
	otherCookie := registerTestUser(t, handler, "other@example.com")

	createPayload, _ := json.Marshal(messageRequest{Body: "first message"})
	createRecorder := httptest.NewRecorder()
	handler.ServeHTTP(createRecorder, authorizedRequest(http.MethodPost, "/api/channels/general/messages", createPayload, ownerCookie))
	if createRecorder.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", createRecorder.Code, createRecorder.Body.String())
	}
	var created Message
	if err := json.NewDecoder(createRecorder.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.Author != "Test User" {
		t.Fatalf("author = %q, want Test User", created.Author)
	}

	updatePayload, _ := json.Marshal(updateMessageRequest{Body: "edited message"})
	otherUpdateRecorder := httptest.NewRecorder()
	handler.ServeHTTP(otherUpdateRecorder, authorizedRequest(http.MethodPatch, "/api/messages/"+created.ID, updatePayload, otherCookie))
	if otherUpdateRecorder.Code != http.StatusForbidden {
		t.Fatalf("other user update status = %d, want %d", otherUpdateRecorder.Code, http.StatusForbidden)
	}

	updateRecorder := httptest.NewRecorder()
	handler.ServeHTTP(updateRecorder, authorizedRequest(http.MethodPatch, "/api/messages/"+created.ID, updatePayload, ownerCookie))
	if updateRecorder.Code != http.StatusOK {
		t.Fatalf("update status = %d, want %d", updateRecorder.Code, http.StatusOK)
	}

	deleteRecorder := httptest.NewRecorder()
	handler.ServeHTTP(deleteRecorder, authorizedRequest(http.MethodDelete, "/api/messages/"+created.ID, nil, ownerCookie))
	if deleteRecorder.Code != http.StatusOK {
		t.Fatalf("delete status = %d, want %d", deleteRecorder.Code, http.StatusOK)
	}
}

func TestReadCursorSurvivesReadMessageDeletion(t *testing.T) {
	server := newServer()
	handler := server.handler()
	cookie := registerTestUser(t, handler, "read-cursor-delete@example.com")

	create := func(body string) Message {
		payload, _ := json.Marshal(messageRequest{Body: body})
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, authorizedRequest(http.MethodPost, "/api/channels/general/messages", payload, cookie))
		if recorder.Code != http.StatusCreated {
			t.Fatalf("create %q status = %d, body = %s", body, recorder.Code, recorder.Body.String())
		}
		var message Message
		if err := json.NewDecoder(recorder.Body).Decode(&message); err != nil {
			t.Fatal(err)
		}
		return message
	}

	readMessage := create("read then delete")
	readRecorder := httptest.NewRecorder()
	handler.ServeHTTP(readRecorder, authorizedRequest(http.MethodPost, "/api/channels/general/read", nil, cookie))
	if readRecorder.Code != http.StatusOK {
		t.Fatalf("mark read status = %d", readRecorder.Code)
	}

	deleteRecorder := httptest.NewRecorder()
	handler.ServeHTTP(deleteRecorder, authorizedRequest(http.MethodDelete, "/api/messages/"+readMessage.ID, nil, cookie))
	if deleteRecorder.Code != http.StatusOK {
		t.Fatalf("delete read message status = %d", deleteRecorder.Code)
	}
	create("new unread message")

	channelsRecorder := httptest.NewRecorder()
	handler.ServeHTTP(channelsRecorder, authorizedRequest(http.MethodGet, "/api/channels", nil, cookie))
	if channelsRecorder.Code != http.StatusOK {
		t.Fatalf("list channels status = %d", channelsRecorder.Code)
	}
	var response struct {
		Channels []Channel `json:"channels"`
	}
	if err := json.NewDecoder(channelsRecorder.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	foundGeneral := false
	for _, channel := range response.Channels {
		if channel.ID == "general" && channel.Unread != 1 {
			t.Fatalf("general unread = %d, want 1 after deleting read message", channel.Unread)
		}
		if channel.ID == "general" {
			foundGeneral = true
		}
	}
	if !foundGeneral {
		t.Fatal("general channel was not returned")
	}
}

func TestCreateChannel(t *testing.T) {
	server := newServer()
	cookie := registerTestUser(t, server.handler(), "channel@example.com")
	payload, _ := json.Marshal(channelRequest{Name: "new-room", Group: "Product", Description: "created in test"})
	request := authorizedRequest(http.MethodPost, "/api/channels", payload, cookie)
	recorder := httptest.NewRecorder()
	server.handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("create channel status = %d, want %d", recorder.Code, http.StatusCreated)
	}
	var channel Channel
	if err := json.NewDecoder(recorder.Body).Decode(&channel); err != nil {
		t.Fatal(err)
	}
	if channel.ID != "new-room" || channel.Name != "new-room" {
		t.Fatalf("unexpected channel: %+v", channel)
	}
}

func TestChannelMembershipProtectsPrivateChannelsAndEvents(t *testing.T) {
	server := newServer()
	handler := server.handler()
	ownerCookie := registerTestUser(t, handler, "private-owner@example.com")
	otherCookie := registerTestUser(t, handler, "private-other@example.com")

	channelPayload, _ := json.Marshal(channelRequest{Name: "private-room", Group: "Product"})
	channelRecorder := httptest.NewRecorder()
	handler.ServeHTTP(channelRecorder, authorizedRequest(http.MethodPost, "/api/channels", channelPayload, ownerCookie))
	if channelRecorder.Code != http.StatusCreated {
		t.Fatalf("create private channel status = %d, body = %s", channelRecorder.Code, channelRecorder.Body.String())
	}
	var privateChannel Channel
	if err := json.NewDecoder(channelRecorder.Body).Decode(&privateChannel); err != nil {
		t.Fatal(err)
	}

	messagePayload, _ := json.Marshal(messageRequest{Body: "private message"})
	ownerMessageRecorder := httptest.NewRecorder()
	handler.ServeHTTP(ownerMessageRecorder, authorizedRequest(http.MethodPost, "/api/channels/"+privateChannel.ID+"/messages", messagePayload, ownerCookie))
	if ownerMessageRecorder.Code != http.StatusCreated {
		t.Fatalf("owner message status = %d, body = %s", ownerMessageRecorder.Code, ownerMessageRecorder.Body.String())
	}
	var privateMessage Message
	if err := json.NewDecoder(ownerMessageRecorder.Body).Decode(&privateMessage); err != nil {
		t.Fatal(err)
	}

	unauthorizedPaths := []string{
		"/api/channels/" + privateChannel.ID + "/messages",
		"/api/messages/" + privateMessage.ID + "/replies",
		"/api/messages/" + privateMessage.ID + "/reactions",
		"/api/messages/" + privateMessage.ID,
	}
	for _, path := range unauthorizedPaths {
		recorder := httptest.NewRecorder()
		method := http.MethodGet
		body := []byte(nil)
		if strings.HasSuffix(path, "/reactions") {
			method = http.MethodPost
			body, _ = json.Marshal(reactionRequest{Emoji: "👍"})
		} else if strings.HasSuffix(path, privateMessage.ID) {
			method = http.MethodPatch
			body, _ = json.Marshal(updateMessageRequest{Body: "should be blocked"})
		}
		handler.ServeHTTP(recorder, authorizedRequest(method, path, body, otherCookie))
		if recorder.Code != http.StatusForbidden {
			t.Fatalf("unauthorized %s status = %d, want %d, body = %s", path, recorder.Code, http.StatusForbidden, recorder.Body.String())
		}
	}

	channelsRecorder := httptest.NewRecorder()
	handler.ServeHTTP(channelsRecorder, authorizedRequest(http.MethodGet, "/api/channels", nil, otherCookie))
	if channelsRecorder.Code != http.StatusOK || strings.Contains(channelsRecorder.Body.String(), privateChannel.ID) {
		t.Fatalf("private channel leaked from channel list: status=%d body=%s", channelsRecorder.Code, channelsRecorder.Body.String())
	}

	eventsRecorder := httptest.NewRecorder()
	handler.ServeHTTP(eventsRecorder, authorizedRequest(http.MethodGet, "/api/events?after=0", nil, otherCookie))
	if eventsRecorder.Code != http.StatusOK || strings.Contains(eventsRecorder.Body.String(), "private message") {
		t.Fatalf("private event leaked: status=%d body=%s", eventsRecorder.Code, eventsRecorder.Body.String())
	}

	httpServer := httptest.NewServer(handler)
	defer httpServer.Close()
	websocketURL := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/ws?channel_id=" + privateChannel.ID
	websocketHeaders := http.Header{"Cookie": []string{otherCookie.String()}}
	websocketHeaders.Set("Origin", "http://127.0.0.1:4174")
	connection, response, err := websocket.DefaultDialer.Dial(websocketURL, websocketHeaders)
	if err == nil {
		connection.Close()
		t.Fatal("unauthorized user connected to private channel websocket")
	}
	if response == nil || response.StatusCode != http.StatusForbidden {
		status := 0
		if response != nil {
			status = response.StatusCode
		}
		t.Fatalf("private websocket status = %d, want %d, err = %v", status, http.StatusForbidden, err)
	}
}

func TestReactionsAreIdempotentAndThreadsPersist(t *testing.T) {
	server := newServer()
	handler := server.handler()
	cookie := registerTestUser(t, handler, "reaction-thread@example.com")

	rootPayload, _ := json.Marshal(messageRequest{Body: "root message"})
	rootRecorder := httptest.NewRecorder()
	handler.ServeHTTP(rootRecorder, authorizedRequest(http.MethodPost, "/api/channels/general/messages", rootPayload, cookie))
	if rootRecorder.Code != http.StatusCreated {
		t.Fatalf("root status = %d", rootRecorder.Code)
	}
	var root Message
	if err := json.NewDecoder(rootRecorder.Body).Decode(&root); err != nil {
		t.Fatal(err)
	}

	reactionPayload, _ := json.Marshal(reactionRequest{Emoji: "👍"})
	for attempt := 0; attempt < 2; attempt++ {
		reactionRecorder := httptest.NewRecorder()
		handler.ServeHTTP(reactionRecorder, authorizedRequest(http.MethodPost, "/api/messages/"+root.ID+"/reactions", reactionPayload, cookie))
		if reactionRecorder.Code != http.StatusOK {
			t.Fatalf("reaction status = %d", reactionRecorder.Code)
		}
		var reacted Message
		if err := json.NewDecoder(reactionRecorder.Body).Decode(&reacted); err != nil {
			t.Fatal(err)
		}
		if len(reacted.Reactions) != 1 || reacted.Reactions[0].Count != 1 || !reacted.Reactions[0].Reacted {
			t.Fatalf("unexpected idempotent reaction: %+v", reacted.Reactions)
		}
	}

	replyPayload, _ := json.Marshal(messageRequest{Body: "thread reply", ParentMessageID: root.ID})
	replyRecorder := httptest.NewRecorder()
	handler.ServeHTTP(replyRecorder, authorizedRequest(http.MethodPost, "/api/channels/general/messages", replyPayload, cookie))
	if replyRecorder.Code != http.StatusCreated {
		t.Fatalf("reply status = %d, body = %s", replyRecorder.Code, replyRecorder.Body.String())
	}
	var reply Message
	if err := json.NewDecoder(replyRecorder.Body).Decode(&reply); err != nil {
		t.Fatal(err)
	}
	if reply.ParentMessageID != root.ID {
		t.Fatalf("parent message id = %q, want %q", reply.ParentMessageID, root.ID)
	}

	repliesRecorder := httptest.NewRecorder()
	handler.ServeHTTP(repliesRecorder, authorizedRequest(http.MethodGet, "/api/messages/"+root.ID+"/replies", nil, cookie))
	if repliesRecorder.Code != http.StatusOK {
		t.Fatalf("replies status = %d", repliesRecorder.Code)
	}
	var replies MessagePage
	if err := json.NewDecoder(repliesRecorder.Body).Decode(&replies); err != nil {
		t.Fatal(err)
	}
	if len(replies.Messages) != 1 || replies.Messages[0].Body != "thread reply" {
		t.Fatalf("unexpected thread replies: %+v", replies.Messages)
	}

	deleteReactionRecorder := httptest.NewRecorder()
	handler.ServeHTTP(deleteReactionRecorder, authorizedRequest(http.MethodDelete, "/api/messages/"+root.ID+"/reactions?emoji=%F0%9F%91%8D", nil, cookie))
	if deleteReactionRecorder.Code != http.StatusOK {
		t.Fatalf("delete reaction status = %d", deleteReactionRecorder.Code)
	}
	var unreacted Message
	if err := json.NewDecoder(deleteReactionRecorder.Body).Decode(&unreacted); err != nil {
		t.Fatal(err)
	}
	if len(unreacted.Reactions) != 0 {
		t.Fatalf("reactions after delete = %+v", unreacted.Reactions)
	}
}

func TestDeletingThreadRootPreservesReplies(t *testing.T) {
	server := newServer()
	handler := server.handler()
	cookie := registerTestUser(t, handler, "thread-root-delete@example.com")

	createMessage := func(request messageRequest) Message {
		payload, _ := json.Marshal(request)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, authorizedRequest(http.MethodPost, "/api/channels/general/messages", payload, cookie))
		if recorder.Code != http.StatusCreated {
			t.Fatalf("create message status = %d, body = %s", recorder.Code, recorder.Body.String())
		}
		var message Message
		if err := json.NewDecoder(recorder.Body).Decode(&message); err != nil {
			t.Fatal(err)
		}
		return message
	}

	root := createMessage(messageRequest{Body: "thread root to delete"})
	replyA := createMessage(messageRequest{Body: "reply A", ParentMessageID: root.ID})
	replyB := createMessage(messageRequest{Body: "reply B", ParentMessageID: root.ID})

	deleteRecorder := httptest.NewRecorder()
	handler.ServeHTTP(deleteRecorder, authorizedRequest(http.MethodDelete, "/api/messages/"+root.ID, nil, cookie))
	if deleteRecorder.Code != http.StatusOK {
		t.Fatalf("delete thread root status = %d, body = %s", deleteRecorder.Code, deleteRecorder.Body.String())
	}

	listRecorder := httptest.NewRecorder()
	handler.ServeHTTP(listRecorder, authorizedRequest(http.MethodGet, "/api/channels/general/messages", nil, cookie))
	if listRecorder.Code != http.StatusOK {
		t.Fatalf("list messages status = %d, body = %s", listRecorder.Code, listRecorder.Body.String())
	}
	var page MessagePage
	if err := json.NewDecoder(listRecorder.Body).Decode(&page); err != nil {
		t.Fatal(err)
	}
	var deletedRoot *Message
	for _, message := range page.Messages {
		if message.ID == root.ID {
			messageCopy := message
			deletedRoot = &messageCopy
		}
		if message.ID == replyA.ID || message.ID == replyB.ID {
			t.Fatalf("reply was promoted to channel root list: %+v", message)
		}
	}
	if deletedRoot == nil || !deletedRoot.Deleted || deletedRoot.Body != deletedMessageBody {
		t.Fatalf("deleted root was not preserved as tombstone: %+v", deletedRoot)
	}

	repliesRecorder := httptest.NewRecorder()
	handler.ServeHTTP(repliesRecorder, authorizedRequest(http.MethodGet, "/api/messages/"+root.ID+"/replies", nil, cookie))
	if repliesRecorder.Code != http.StatusOK {
		t.Fatalf("list replies status = %d, body = %s", repliesRecorder.Code, repliesRecorder.Body.String())
	}
	var replies MessagePage
	if err := json.NewDecoder(repliesRecorder.Body).Decode(&replies); err != nil {
		t.Fatal(err)
	}
	if len(replies.Messages) != 2 || replies.Messages[0].ParentMessageID != root.ID || replies.Messages[1].ParentMessageID != root.ID {
		t.Fatalf("replies were not preserved under deleted root: %+v", replies.Messages)
	}
}

func TestLateRegisteredUserDoesNotJoinCreatedChannel(t *testing.T) {
	server := newServer()
	handler := server.handler()
	ownerCookie := registerTestUser(t, handler, "late-channel-owner@example.com")

	payload, _ := json.Marshal(channelRequest{Name: "late-private-room", Group: "Product"})
	channelRecorder := httptest.NewRecorder()
	handler.ServeHTTP(channelRecorder, authorizedRequest(http.MethodPost, "/api/channels", payload, ownerCookie))
	if channelRecorder.Code != http.StatusCreated {
		t.Fatalf("create channel status = %d, body = %s", channelRecorder.Code, channelRecorder.Body.String())
	}
	var privateChannel Channel
	if err := json.NewDecoder(channelRecorder.Body).Decode(&privateChannel); err != nil {
		t.Fatal(err)
	}

	lateCookie := registerTestUser(t, handler, "late-channel-user@example.com")
	channelsRecorder := httptest.NewRecorder()
	handler.ServeHTTP(channelsRecorder, authorizedRequest(http.MethodGet, "/api/channels", nil, lateCookie))
	if channelsRecorder.Code != http.StatusOK {
		t.Fatalf("late user channel list status = %d, body = %s", channelsRecorder.Code, channelsRecorder.Body.String())
	}
	if strings.Contains(channelsRecorder.Body.String(), privateChannel.ID) {
		t.Fatalf("late user was added to user-created channel: %s", channelsRecorder.Body.String())
	}
}

func TestWebSocketTypingAndPresenceEvents(t *testing.T) {
	server := newServer()
	httpServer := httptest.NewServer(server.handler())
	defer httpServer.Close()
	httpClient := http.Client{}
	payload, _ := json.Marshal(registerRequest{Name: "Typing Test", Email: "typing@example.com", Password: "test-password"})
	registerResponse, err := httpClient.Post(httpServer.URL+"/api/auth/register", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	defer registerResponse.Body.Close()
	cookie := registerResponse.Cookies()[0]
	websocketURL := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/ws?channel_id=all"
	websocketHeaders := http.Header{"Cookie": []string{cookie.String()}}
	websocketHeaders.Set("Origin", "http://127.0.0.1:4174")
	connection, _, err := websocket.DefaultDialer.Dial(websocketURL, websocketHeaders)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()

	if err := connection.WriteJSON(websocketCommand{Type: "typing.started", ChannelID: "general"}); err != nil {
		t.Fatal(err)
	}
	if err := connection.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	var typing realtimeEvent
	if err := connection.ReadJSON(&typing); err != nil {
		t.Fatal(err)
	}
	if typing.Type != "typing.started" || typing.ChannelID != "general" || typing.ActorName != "Typing Test" {
		t.Fatalf("unexpected typing event: %+v", typing)
	}

	if err := connection.WriteJSON(websocketCommand{Type: "presence.changed", Presence: "away"}); err != nil {
		t.Fatal(err)
	}
	var presence realtimeEvent
	if err := connection.ReadJSON(&presence); err != nil {
		t.Fatal(err)
	}
	if presence.Type != "presence.changed" || presence.Presence != "away" || presence.ChannelID != "*" {
		t.Fatalf("unexpected presence event: %+v", presence)
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

func TestCursorPaginationReadStateAndEventSync(t *testing.T) {
	server := newServer()
	handler := server.handler()
	cookie := registerTestUser(t, handler, "cursor@example.com")

	channelsRequest := authorizedRequest(http.MethodGet, "/api/channels", nil, cookie)
	channelsRecorder := httptest.NewRecorder()
	handler.ServeHTTP(channelsRecorder, channelsRequest)
	if channelsRecorder.Code != http.StatusOK {
		t.Fatalf("channels status = %d", channelsRecorder.Code)
	}
	var channelsResponse struct {
		Channels []Channel `json:"channels"`
		Cursor   int64     `json:"cursor"`
	}
	if err := json.NewDecoder(channelsRecorder.Body).Decode(&channelsResponse); err != nil {
		t.Fatal(err)
	}
	initialCursor := channelsResponse.Cursor

	for _, body := range []string{"one", "two", "three"} {
		payload, _ := json.Marshal(messageRequest{Body: body})
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, authorizedRequest(http.MethodPost, "/api/channels/general/messages", payload, cookie))
		if recorder.Code != http.StatusCreated {
			t.Fatalf("create status = %d", recorder.Code)
		}
	}

	pageRecorder := httptest.NewRecorder()
	handler.ServeHTTP(pageRecorder, authorizedRequest(http.MethodGet, "/api/channels/general/messages?limit=2", nil, cookie))
	if pageRecorder.Code != http.StatusOK {
		t.Fatalf("page status = %d", pageRecorder.Code)
	}
	var page MessagePage
	if err := json.NewDecoder(pageRecorder.Body).Decode(&page); err != nil {
		t.Fatal(err)
	}
	if len(page.Messages) != 2 || !page.HasMore || page.NextCursor == "" {
		t.Fatalf("unexpected first page: %+v", page)
	}

	olderRecorder := httptest.NewRecorder()
	handler.ServeHTTP(olderRecorder, authorizedRequest(http.MethodGet, "/api/channels/general/messages?limit=2&before="+page.NextCursor, nil, cookie))
	if olderRecorder.Code != http.StatusOK {
		t.Fatalf("older page status = %d", olderRecorder.Code)
	}
	var olderPage MessagePage
	if err := json.NewDecoder(olderRecorder.Body).Decode(&olderPage); err != nil {
		t.Fatal(err)
	}
	if len(olderPage.Messages) == 0 || olderPage.Messages[len(olderPage.Messages)-1].Body == page.Messages[0].Body {
		t.Fatalf("older page did not move cursor: first=%+v older=%+v", page.Messages, olderPage.Messages)
	}

	eventsRecorder := httptest.NewRecorder()
	handler.ServeHTTP(eventsRecorder, authorizedRequest(http.MethodGet, "/api/events?after="+strconv.FormatInt(initialCursor, 10)+"&limit=2", nil, cookie))
	if eventsRecorder.Code != http.StatusOK {
		t.Fatalf("events status = %d", eventsRecorder.Code)
	}
	var eventsPage EventPage
	if err := json.NewDecoder(eventsRecorder.Body).Decode(&eventsPage); err != nil {
		t.Fatal(err)
	}
	if len(eventsPage.Events) != 2 || !eventsPage.HasMore || eventsPage.NextCursor == "" {
		t.Fatalf("unexpected event page: %+v", eventsPage)
	}
	if eventsPage.Events[0].Sequence <= initialCursor || eventsPage.Events[1].Sequence <= eventsPage.Events[0].Sequence {
		t.Fatalf("events are not ordered: %+v", eventsPage.Events)
	}
	if eventsPage.Events[0].EventID != eventsPage.Events[0].Sequence || eventsPage.Events[1].EventID != eventsPage.Events[1].Sequence {
		t.Fatalf("event ids are not aligned with cursors: %+v", eventsPage.Events)
	}

	channelsBeforeRead := httptest.NewRecorder()
	handler.ServeHTTP(channelsBeforeRead, authorizedRequest(http.MethodGet, "/api/channels", nil, cookie))
	var beforeRead struct {
		Channels []Channel `json:"channels"`
	}
	if err := json.NewDecoder(channelsBeforeRead.Body).Decode(&beforeRead); err != nil {
		t.Fatal(err)
	}
	if beforeRead.Channels[0].Unread == 0 {
		t.Fatal("expected unread messages before marking the channel read")
	}

	readRecorder := httptest.NewRecorder()
	handler.ServeHTTP(readRecorder, authorizedRequest(http.MethodPost, "/api/channels/general/read", nil, cookie))
	if readRecorder.Code != http.StatusOK {
		t.Fatalf("mark read status = %d", readRecorder.Code)
	}
	channelsAfterRead := httptest.NewRecorder()
	handler.ServeHTTP(channelsAfterRead, authorizedRequest(http.MethodGet, "/api/channels", nil, cookie))
	var afterRead struct {
		Channels []Channel `json:"channels"`
	}
	if err := json.NewDecoder(channelsAfterRead.Body).Decode(&afterRead); err != nil {
		t.Fatal(err)
	}
	if afterRead.Channels[0].Unread != 0 {
		t.Fatalf("unread after mark read = %d, want 0", afterRead.Channels[0].Unread)
	}
}

func TestWebSocketReceivesCreatedMessage(t *testing.T) {
	server := newServer()
	httpServer := httptest.NewServer(server.handler())
	defer httpServer.Close()
	httpClient := http.Client{}
	payload, _ := json.Marshal(registerRequest{Name: "Realtime Test", Email: "realtime@example.com", Password: "test-password"})
	registerResponse, err := httpClient.Post(httpServer.URL+"/api/auth/register", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	defer registerResponse.Body.Close()
	if registerResponse.StatusCode != http.StatusCreated {
		t.Fatalf("register status = %d", registerResponse.StatusCode)
	}
	cookies := registerResponse.Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected session cookie, got %d", len(cookies))
	}

	websocketURL := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/ws?channel_id=general"
	header := http.Header{}
	header.Set("Cookie", cookies[0].String())
	header.Set("Origin", "http://127.0.0.1:4174")
	connection, _, err := websocket.DefaultDialer.Dial(websocketURL, header)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()

	createPayload, _ := json.Marshal(messageRequest{Body: "hello over websocket"})
	createRequest, err := http.NewRequest(http.MethodPost, httpServer.URL+"/api/channels/general/messages", bytes.NewReader(createPayload))
	if err != nil {
		t.Fatal(err)
	}
	createRequest.Header.Set("Content-Type", "application/json")
	createRequest.AddCookie(cookies[0])
	response, err := httpClient.Do(createRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d", response.StatusCode)
	}

	if err := connection.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	var event realtimeEvent
	if err := connection.ReadJSON(&event); err != nil {
		t.Fatal(err)
	}
	if event.Type != "message.created" {
		t.Fatalf("event type = %q, want message.created", event.Type)
	}
	if event.Message == nil || event.Message.Body != "hello over websocket" {
		t.Fatalf("unexpected event payload: %+v", event)
	}
}

func TestWebSocketOriginPolicy(t *testing.T) {
	t.Setenv("FRONTEND_ORIGIN", "http://127.0.0.1:4174")
	if !isAllowedOrigin("http://127.0.0.1:4174") {
		t.Fatal("configured frontend origin should be allowed")
	}
	if isAllowedOrigin("http://evil.example") {
		t.Fatal("unexpected origin should be rejected")
	}
	if isAllowedOrigin("") {
		t.Fatal("empty origin should be rejected")
	}
}
