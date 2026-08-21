package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestShouldSeedDemoData(t *testing.T) {
	t.Setenv("SEED_DEMO_DATA", "false")
	t.Setenv("APP_ENV", "production")
	if shouldSeedDemoData() {
		t.Fatal("production should not seed demo data by default")
	}

	t.Setenv("APP_ENV", "development")
	if !shouldSeedDemoData() {
		t.Fatal("development should seed demo data")
	}

	t.Setenv("APP_ENV", "production")
	t.Setenv("SEED_DEMO_DATA", "true")
	if !shouldSeedDemoData() {
		t.Fatal("explicit seed flag should enable demo data")
	}
}

func TestPostgresChannelMembershipIntegration(t *testing.T) {
	t.Setenv("APP_ENV", "test")
	databaseURL := strings.TrimSpace(os.Getenv("POSTGRES_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("POSTGRES_TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	repository, err := newPostgresRepository(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect to PostgreSQL: %v", err)
	}
	defer repository.Close()

	channel, _, err := repository.CreateChannel(ctx, "u-naoki", channelRequest{Name: "pg-private-" + randomID(), Group: "Product", MemberIDs: []string{"u-ken"}})
	if err != nil {
		t.Fatalf("create private channel: %v", err)
	}
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_, _ = repository.pool.Exec(cleanupCtx, `DELETE FROM channels WHERE id=$1`, channel.ID)
	}()

	ownerMember, err := repository.IsChannelMember(ctx, "u-naoki", channel.ID)
	if err != nil || !ownerMember {
		t.Fatalf("owner membership = %v, err = %v", ownerMember, err)
	}
	otherMember, err := repository.IsChannelMember(ctx, "u-ayaka", channel.ID)
	if err != nil {
		t.Fatalf("other membership lookup: %v", err)
	}
	if otherMember {
		t.Fatal("uninvited user was added to private channel")
	}
	selectedMember, err := repository.IsChannelMember(ctx, "u-ken", channel.ID)
	if err != nil {
		t.Fatalf("selected member lookup: %v", err)
	}
	if !selectedMember {
		t.Fatal("selected PostgreSQL member was not added to private channel")
	}

	application := newServerWithRepository(repository)
	handler := application.handler()
	ownerCookie := loginTestUser(t, handler, "demo@example.com")
	otherCookie := loginTestUser(t, handler, "ayaka@example.com")

	messagePayload := []byte(`{"body":"postgres private message"}`)
	ownerMessageRecorder := httptest.NewRecorder()
	handler.ServeHTTP(ownerMessageRecorder, authorizedRequest(http.MethodPost, "/api/channels/"+channel.ID+"/messages", messagePayload, ownerCookie))
	if ownerMessageRecorder.Code != http.StatusCreated {
		t.Fatalf("owner message status = %d, body = %s", ownerMessageRecorder.Code, ownerMessageRecorder.Body.String())
	}

	otherMessagesRecorder := httptest.NewRecorder()
	handler.ServeHTTP(otherMessagesRecorder, authorizedRequest(http.MethodGet, "/api/channels/"+channel.ID+"/messages", nil, otherCookie))
	if otherMessagesRecorder.Code != http.StatusForbidden {
		t.Fatalf("other message list status = %d, want %d", otherMessagesRecorder.Code, http.StatusForbidden)
	}

	otherChannelsRecorder := httptest.NewRecorder()
	handler.ServeHTTP(otherChannelsRecorder, authorizedRequest(http.MethodGet, "/api/channels", nil, otherCookie))
	if otherChannelsRecorder.Code != http.StatusOK || strings.Contains(otherChannelsRecorder.Body.String(), channel.ID) {
		t.Fatalf("private channel leaked from PostgreSQL channel list: status=%d body=%s", otherChannelsRecorder.Code, otherChannelsRecorder.Body.String())
	}

	otherEventsRecorder := httptest.NewRecorder()
	handler.ServeHTTP(otherEventsRecorder, authorizedRequest(http.MethodGet, "/api/events?after=0", nil, otherCookie))
	if otherEventsRecorder.Code != http.StatusOK || strings.Contains(otherEventsRecorder.Body.String(), "postgres private message") {
		t.Fatalf("private PostgreSQL event leaked: status=%d body=%s", otherEventsRecorder.Code, otherEventsRecorder.Body.String())
	}

	httpServer := httptest.NewServer(handler)
	defer httpServer.Close()
	websocketHeaders := http.Header{"Cookie": []string{otherCookie.String()}}
	websocketHeaders.Set("Origin", "http://127.0.0.1:4174")
	websocketURL := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/ws?channel_id=" + channel.ID
	connection, response, err := websocket.DefaultDialer.Dial(websocketURL, websocketHeaders)
	if err == nil {
		connection.Close()
		t.Fatal("uninvited PostgreSQL user connected to private channel websocket")
	}
	if response == nil || response.StatusCode != http.StatusForbidden {
		status := 0
		if response != nil {
			status = response.StatusCode
		}
		t.Fatalf("private PostgreSQL websocket status = %d, want %d, err = %v", status, http.StatusForbidden, err)
	}

	ownerHeaders := http.Header{"Cookie": []string{ownerCookie.String()}}
	ownerHeaders.Set("Origin", "http://127.0.0.1:4174")
	ownerConnection, _, err := websocket.DefaultDialer.Dial(websocketURL, ownerHeaders)
	if err != nil {
		t.Fatalf("owner websocket connection: %v", err)
	}
	_ = ownerConnection.Close()

	if _, _, err := repository.UpdateChannel(ctx, channel.ID, "u-naoki", channelUpdateRequest{Name: channel.Name, Description: channel.Description, MemberIDs: []string{"u-ayaka"}}); err != nil {
		t.Fatalf("update PostgreSQL channel members: %v", err)
	}
	updatedMember, err := repository.IsChannelMember(ctx, "u-ayaka", channel.ID)
	if err != nil || !updatedMember {
		t.Fatalf("updated PostgreSQL member = %v, err = %v", updatedMember, err)
	}
	removedMember, err := repository.IsChannelMember(ctx, "u-ken", channel.ID)
	if err != nil || removedMember {
		t.Fatalf("removed PostgreSQL member = %v, err = %v", removedMember, err)
	}
}

func TestPostgresThreadRootDeletionAndLateMembership(t *testing.T) {
	t.Setenv("APP_ENV", "test")
	databaseURL := strings.TrimSpace(os.Getenv("POSTGRES_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("POSTGRES_TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	repository, err := newPostgresRepository(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect to PostgreSQL: %v", err)
	}
	defer repository.Close()

	channel, _, err := repository.CreateChannel(ctx, "u-naoki", channelRequest{Name: "pg-late-private-" + randomID(), Group: "Product"})
	if err != nil {
		t.Fatalf("create private channel: %v", err)
	}
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_, _ = repository.pool.Exec(cleanupCtx, `DELETE FROM channels WHERE id=$1`, channel.ID)
	}()

	lateUser, err := repository.RegisterUser(ctx, registerRequest{Name: "Late PostgreSQL User", Email: "late-" + randomID() + "@example.com", Password: "test-password"})
	if err != nil {
		t.Fatalf("register late user: %v", err)
	}
	member, err := repository.IsChannelMember(ctx, lateUser.ID, channel.ID)
	if err != nil {
		t.Fatalf("late membership lookup: %v", err)
	}
	if member {
		t.Fatal("late user was automatically added to user-created channel")
	}

	root, _, err := repository.CreateMessage(ctx, "general", "u-naoki", messageRequest{Body: "postgres thread root"})
	if err != nil {
		t.Fatalf("create root: %v", err)
	}
	replyA, _, err := repository.CreateMessage(ctx, "general", "u-naoki", messageRequest{Body: "postgres reply A", ParentMessageID: root.ID})
	if err != nil {
		t.Fatalf("create reply A: %v", err)
	}
	replyB, _, err := repository.CreateMessage(ctx, "general", "u-naoki", messageRequest{Body: "postgres reply B", ParentMessageID: root.ID})
	if err != nil {
		t.Fatalf("create reply B: %v", err)
	}
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_, _ = repository.pool.Exec(cleanupCtx, `DELETE FROM messages WHERE id=$1`, replyA.ID)
		_, _ = repository.pool.Exec(cleanupCtx, `DELETE FROM messages WHERE id=$1`, replyB.ID)
		_, _ = repository.pool.Exec(cleanupCtx, `DELETE FROM messages WHERE id=$1`, root.ID)
	}()

	_, record, err := repository.DeleteMessage(ctx, root.ID, "u-naoki")
	if err != nil {
		t.Fatalf("delete root: %v", err)
	}
	if record.Event.Message == nil || !record.Event.Message.Deleted {
		t.Fatalf("delete event did not contain tombstone: %+v", record.Event)
	}

	topLevel, err := repository.ListMessagePage(ctx, "general", "", 100)
	if err != nil {
		t.Fatalf("list top-level messages: %v", err)
	}
	var foundRoot *Message
	for _, message := range topLevel.Messages {
		if message.ID == root.ID {
			messageCopy := message
			foundRoot = &messageCopy
		}
		if message.ID == replyA.ID || message.ID == replyB.ID {
			t.Fatalf("PostgreSQL promoted reply to top-level list: %+v", message)
		}
	}
	if foundRoot == nil || !foundRoot.Deleted || foundRoot.Body != deletedMessageBody {
		t.Fatalf("PostgreSQL root tombstone missing: %+v", foundRoot)
	}

	thread, err := repository.ListThreadPage(ctx, root.ID, "", 100)
	if err != nil {
		t.Fatalf("list replies after root deletion: %v", err)
	}
	if len(thread.Messages) != 2 || thread.Messages[0].ParentMessageID != root.ID || thread.Messages[1].ParentMessageID != root.ID {
		t.Fatalf("PostgreSQL replies were not preserved: %+v", thread.Messages)
	}

	contextRoot, _, err := repository.CreateMessage(ctx, channel.ID, "u-naoki", messageRequest{Body: "postgres AI context root"})
	if err != nil {
		t.Fatalf("create AI context root: %v", err)
	}
	contextReply, _, err := repository.CreateMessage(ctx, channel.ID, "u-naoki", messageRequest{Body: "postgres AI context thread reply", ParentMessageID: contextRoot.ID})
	if err != nil {
		t.Fatalf("create AI context reply: %v", err)
	}
	aiContext, err := repository.ListAIContextMessages(ctx, channel.ID, 100)
	if err != nil {
		t.Fatalf("list PostgreSQL AI context: %v", err)
	}
	if !containsMessageID(aiContext, contextReply.ID) {
		t.Fatalf("PostgreSQL AI context omitted thread reply: %+v", aiContext)
	}
	unread, err := repository.ListUnreadMessages(ctx, "u-naoki", channel.ID)
	if err != nil {
		t.Fatalf("list PostgreSQL unread messages: %v", err)
	}
	if !containsMessageID(unread, contextReply.ID) {
		t.Fatalf("PostgreSQL unread context omitted thread reply: %+v", unread)
	}
	if _, err := repository.MarkChannelRead(ctx, "u-naoki", channel.ID); err != nil {
		t.Fatalf("mark PostgreSQL channel read: %v", err)
	}
	unread, err = repository.ListUnreadMessages(ctx, "u-naoki", channel.ID)
	if err != nil {
		t.Fatalf("list PostgreSQL unread messages after read: %v", err)
	}
	if len(unread) != 0 {
		t.Fatalf("PostgreSQL unread context was not cleared: %+v", unread)
	}
}
