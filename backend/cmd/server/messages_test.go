package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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

func TestSearchMessagesReturnsOnlyTopLevelMessagesAndMatchesAuthors(t *testing.T) {
	repository := newMemoryRepository()
	ctx := context.Background()
	root, _, err := repository.CreateMessage(ctx, "general", "u-ken", messageRequest{Body: "searchable root"})
	if err != nil {
		t.Fatalf("create root: %v", err)
	}
	if _, _, err := repository.CreateMessage(ctx, "general", "u-ken", messageRequest{Body: "searchable reply", ParentMessageID: root.ID}); err != nil {
		t.Fatalf("create reply: %v", err)
	}

	results, err := repository.SearchMessages(ctx, "general", "searchable", 50)
	if err != nil {
		t.Fatalf("search messages: %v", err)
	}
	if len(results) != 1 || results[0].ID != root.ID {
		t.Fatalf("search returned thread replies as roots: %+v", results)
	}

	results, err = repository.SearchMessages(ctx, "general", "ken", 50)
	if err != nil {
		t.Fatalf("search author: %v", err)
	}
	if len(results) == 0 || results[0].AuthorID != "u-ken" {
		t.Fatalf("author search returned unexpected results: %+v", results)
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
	otherCookie := registerTestUser(t, handler, "read-cursor-delete-other@example.com")

	create := func(cookie *http.Cookie, body string) Message {
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

	readMessage := create(cookie, "read then delete")
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
	create(otherCookie, "new unread message")

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
