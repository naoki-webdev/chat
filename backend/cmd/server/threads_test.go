package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

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

func TestDeletedThreadRootRemainsDiscoverable(t *testing.T) {
	repository := newMemoryRepository()
	if _, _, err := repository.DeleteMessage(context.Background(), "ds-1", "u-ayaka"); err != nil {
		t.Fatalf("delete seeded thread root: %v", err)
	}

	page, err := repository.ListThreadRoots(context.Background(), "u-naoki", 50)
	if err != nil {
		t.Fatalf("list thread roots after root deletion: %v", err)
	}
	var root *Message
	for _, message := range page.Messages {
		if message.ID == "ds-1" {
			messageCopy := message
			root = &messageCopy
			break
		}
	}
	if root == nil || !root.Deleted {
		t.Fatalf("deleted thread root was not discoverable as a tombstone: %+v", root)
	}
}

func TestThreadRootsEndpointListsRootsAcrossAccessibleChannels(t *testing.T) {
	server := newServer()
	cookie := registerTestUser(t, server.handler(), "thread-index@example.com")
	recorder := httptest.NewRecorder()
	server.handler().ServeHTTP(recorder, authorizedRequest(http.MethodGet, "/api/threads?limit=100", nil, cookie))
	if recorder.Code != http.StatusOK {
		t.Fatalf("thread roots status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var page ThreadRootPage
	if err := json.NewDecoder(recorder.Body).Decode(&page); err != nil {
		t.Fatalf("decode thread roots: %v", err)
	}
	if page.Total < 1 || !containsMessageID(page.Messages, "ds-1") {
		t.Fatalf("thread roots = %+v, total = %d", page.Messages, page.Total)
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
