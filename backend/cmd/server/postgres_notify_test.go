package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestPostgresRealtimeNotificationDecoding(t *testing.T) {
	event := realtimeEvent{Type: "typing.started", ChannelID: "general", ActorID: "u-naoki"}
	payload, err := json.Marshal(postgresRealtimeNotification{Event: &event})
	if err != nil {
		t.Fatalf("marshal ephemeral notification: %v", err)
	}
	notification, err := decodePostgresRealtimeNotification(string(payload))
	if err != nil {
		t.Fatalf("decode ephemeral notification: %v", err)
	}
	if notification.Event == nil || notification.Event.Type != event.Type {
		t.Fatalf("decoded notification = %+v", notification)
	}

	payload, err = json.Marshal(postgresRealtimeNotification{Sequence: 42})
	if err != nil {
		t.Fatalf("marshal persisted notification: %v", err)
	}
	notification, err = decodePostgresRealtimeNotification(string(payload))
	if err != nil || notification.Sequence != 42 {
		t.Fatalf("decoded persisted notification = %+v, err = %v", notification, err)
	}
}

func TestPostgresRealtimeBroadcastAcrossRepositories(t *testing.T) {
	t.Setenv("APP_ENV", "test")
	databaseURL := strings.TrimSpace(os.Getenv("POSTGRES_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("POSTGRES_TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	repositoryOne, err := newPostgresRepository(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect first PostgreSQL repository: %v", err)
	}
	defer repositoryOne.Close()
	repositoryTwo, err := newPostgresRepository(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect second PostgreSQL repository: %v", err)
	}
	defer repositoryTwo.Close()

	serverOne := newServerWithRepository(repositoryOne)
	serverTwo := newServerWithRepository(repositoryTwo)
	listenerContext, listenerCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer listenerCancel()
	if err := repositoryOne.waitForEventListener(listenerContext); err != nil {
		t.Fatalf("first event listener was not ready: %v", err)
	}
	if err := repositoryTwo.waitForEventListener(listenerContext); err != nil {
		t.Fatalf("second event listener was not ready: %v", err)
	}

	channel, _, err := repositoryOne.CreateChannel(ctx, "u-naoki", channelRequest{
		Name:      "multi-server-" + randomID(),
		Group:     "Product",
		MemberIDs: []string{"u-ayaka"},
	})
	if err != nil {
		t.Fatalf("create test channel: %v", err)
	}
	defer func() {
		cleanupContext, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_, _ = repositoryOne.pool.Exec(cleanupContext, `DELETE FROM channels WHERE id=$1`, channel.ID)
	}()

	handlerOne := serverOne.handler()
	handlerTwo := serverTwo.handler()
	ownerCookie := loginTestUser(t, handlerOne, "demo@example.com")
	memberCookie := loginTestUser(t, handlerTwo, "ayaka@example.com")

	httpServer := httptest.NewServer(handlerTwo)
	defer httpServer.Close()
	websocketHeaders := http.Header{"Cookie": []string{memberCookie.String()}}
	websocketHeaders.Set("Origin", "http://127.0.0.1:4174")
	websocketURL := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/ws?channel_id=" + channel.ID
	connection, response, err := websocket.DefaultDialer.Dial(websocketURL, websocketHeaders)
	if err != nil {
		status := 0
		if response != nil {
			status = response.StatusCode
		}
		t.Fatalf("connect second-server websocket: status=%d err=%v", status, err)
	}
	defer connection.Close()

	if err := connection.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatalf("set websocket read deadline: %v", err)
	}
	serverOne.broadcast(realtimeEvent{
		Type:      "typing.started",
		ChannelID: channel.ID,
		ActorID:   "u-naoki",
		ActorName: "Taro Tanaka",
	})
	for {
		var event realtimeEvent
		if err := connection.ReadJSON(&event); err != nil {
			t.Fatalf("read cross-server ephemeral event: %v", err)
		}
		if event.Type == "typing.started" && event.ChannelID == channel.ID && event.ActorID == "u-naoki" {
			break
		}
	}

	messagePayload := []byte(`{"body":"message from the first server"}`)
	messageRecorder := httptest.NewRecorder()
	handlerOne.ServeHTTP(messageRecorder, authorizedRequest(http.MethodPost, "/api/channels/"+channel.ID+"/messages", messagePayload, ownerCookie))
	if messageRecorder.Code != http.StatusCreated {
		t.Fatalf("message status = %d, body = %s", messageRecorder.Code, messageRecorder.Body.String())
	}

	for {
		var event realtimeEvent
		if err := connection.ReadJSON(&event); err != nil {
			t.Fatalf("read cross-server event: %v", err)
		}
		if event.Type == "message.created" && event.Message != nil && event.Message.Body == "message from the first server" {
			return
		}
	}
}
