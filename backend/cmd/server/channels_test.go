package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

func TestChannelRequestRejectsUnsupportedTaxonomy(t *testing.T) {
	if err := validateChannelRequest(channelRequest{Name: "room", Group: "Unknown", Kind: "channel"}); err == nil {
		t.Fatal("unsupported channel group should be rejected")
	}
	if err := validateChannelRequest(channelRequest{Name: "room", Group: "Product", Kind: "broadcast"}); err == nil {
		t.Fatal("unsupported channel kind should be rejected")
	}
}

func TestCreateChannel(t *testing.T) {
	server := newServer()
	cookie := registerTestUser(t, server.handler(), "channel@example.com")
	beforeEvents, err := server.repository.ListEvents(context.Background(), "u-ayaka", 0, 100)
	if err != nil {
		t.Fatalf("list events before channel creation: %v", err)
	}
	payload, _ := json.Marshal(channelRequest{Name: "new-room", Group: "Product", Description: "created in test", MemberIDs: []string{"u-ayaka"}})
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
	if channel.ID == "" || channel.ID == "new-room" || channel.Name != "new-room" {
		t.Fatalf("unexpected channel: %+v", channel)
	}
	member, err := server.repository.IsChannelMember(context.Background(), "u-ayaka", channel.ID)
	if err != nil {
		t.Fatalf("check invited member: %v", err)
	}
	if !member {
		t.Fatal("selected member was not added to the channel")
	}
	createdEvents, err := server.repository.ListEvents(context.Background(), "u-ayaka", beforeEvents.Cursor, 100)
	if err != nil {
		t.Fatalf("list events after channel creation: %v", err)
	}
	if !containsChannelEvent(createdEvents.Events, "channel.created", channel.ID, "") {
		t.Fatalf("invited member did not receive channel.created event: %+v", createdEvents.Events)
	}

	renamePayload, _ := json.Marshal(channelUpdateRequest{Name: "team", Description: "renamed in test"})
	renameRecorder := httptest.NewRecorder()
	server.handler().ServeHTTP(renameRecorder, authorizedRequest(http.MethodPatch, "/api/channels/"+channel.ID, renamePayload, cookie))
	if renameRecorder.Code != http.StatusOK {
		t.Fatalf("rename channel status = %d, body = %s", renameRecorder.Code, renameRecorder.Body.String())
	}
	secondPayload, _ := json.Marshal(channelRequest{Name: "new-room", Group: "Product"})
	secondRecorder := httptest.NewRecorder()
	server.handler().ServeHTTP(secondRecorder, authorizedRequest(http.MethodPost, "/api/channels", secondPayload, cookie))
	if secondRecorder.Code != http.StatusCreated {
		t.Fatalf("create channel after rename status = %d, body = %s", secondRecorder.Code, secondRecorder.Body.String())
	}
	var secondChannel Channel
	if err := json.NewDecoder(secondRecorder.Body).Decode(&secondChannel); err != nil {
		t.Fatal(err)
	}
	if secondChannel.ID == channel.ID {
		t.Fatalf("renamed channel and new channel share ID: %q", channel.ID)
	}
}

func TestChannelMembersAndOwnerUpdate(t *testing.T) {
	server := newServer()
	handler := server.handler()
	ownerCookie := loginTestUser(t, handler, "demo@example.com")
	beforeEvents, err := server.repository.ListEvents(context.Background(), "u-ken", 0, 100)
	if err != nil {
		t.Fatalf("list events before channel update: %v", err)
	}

	membersRecorder := httptest.NewRecorder()
	handler.ServeHTTP(membersRecorder, authorizedRequest(http.MethodGet, "/api/channels/design-system/members", nil, ownerCookie))
	if membersRecorder.Code != http.StatusOK {
		t.Fatalf("list channel members status = %d", membersRecorder.Code)
	}
	var membersResponse struct {
		Members []ChannelMember `json:"members"`
	}
	if err := json.NewDecoder(membersRecorder.Body).Decode(&membersResponse); err != nil {
		t.Fatal(err)
	}
	if len(membersResponse.Members) != 5 {
		t.Fatalf("member count = %d, want 5", len(membersResponse.Members))
	}

	payload, _ := json.Marshal(channelUpdateRequest{Name: "design-system", Description: "updated channel description", MemberIDs: []string{"u-ayaka"}})
	updateRecorder := httptest.NewRecorder()
	handler.ServeHTTP(updateRecorder, authorizedRequest(http.MethodPatch, "/api/channels/design-system", payload, ownerCookie))
	if updateRecorder.Code != http.StatusOK {
		t.Fatalf("update channel status = %d, body = %s", updateRecorder.Code, updateRecorder.Body.String())
	}
	if member, err := server.repository.IsChannelMember(context.Background(), "u-ayaka", "design-system"); err != nil || !member {
		t.Fatalf("added member = %v, err = %v", member, err)
	}
	if member, err := server.repository.IsChannelMember(context.Background(), "u-ken", "design-system"); err != nil || member {
		t.Fatalf("removed member = %v, err = %v", member, err)
	}
	if member, err := server.repository.IsChannelMember(context.Background(), orbitAIUserID, "design-system"); err != nil || !member {
		t.Fatalf("Orbit AI membership = %v, err = %v", member, err)
	}
	updatedEvents, err := server.repository.ListEvents(context.Background(), "u-ken", beforeEvents.Cursor, 100)
	if err != nil {
		t.Fatalf("list events after channel update: %v", err)
	}
	if !containsChannelEvent(updatedEvents.Events, "channel.member_removed", "design-system", "u-ken") {
		t.Fatalf("removed member did not receive removal event: %+v", updatedEvents.Events)
	}
	currentMemberEvents, err := server.repository.ListEvents(context.Background(), "u-ayaka", beforeEvents.Cursor, 100)
	if err != nil {
		t.Fatalf("list events for current member after channel update: %v", err)
	}
	if !containsChannelEvent(currentMemberEvents.Events, "channel.updated", "design-system", "") {
		t.Fatalf("current member did not receive channel.updated event: %+v", currentMemberEvents.Events)
	}

	memberCookie := loginTestUser(t, handler, "ayaka@example.com")
	forbiddenRecorder := httptest.NewRecorder()
	handler.ServeHTTP(forbiddenRecorder, authorizedRequest(http.MethodPatch, "/api/channels/design-system", payload, memberCookie))
	if forbiddenRecorder.Code != http.StatusForbidden {
		t.Fatalf("member update status = %d, want %d", forbiddenRecorder.Code, http.StatusForbidden)
	}
	if !strings.Contains(forbiddenRecorder.Body.String(), "permission to manage this channel") {
		t.Fatalf("member update error = %s", forbiddenRecorder.Body.String())
	}
}

func containsChannelEvent(events []realtimeEvent, eventType, channelID, memberID string) bool {
	for _, event := range events {
		if event.Type == eventType && event.ChannelID == channelID && event.MemberID == memberID {
			return true
		}
	}
	return false
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
