package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

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
