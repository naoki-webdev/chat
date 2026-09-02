package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
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
	if shouldSeedDemoData() {
		t.Fatal("production must not seed demo data even when the explicit flag is set")
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
	renamedChannel, renameEvents, err := repository.UpdateChannel(ctx, channel.ID, "u-naoki", channelUpdateRequest{Name: channel.Name + "-renamed", Description: channel.Description})
	if err != nil {
		t.Fatalf("rename PostgreSQL channel: %v", err)
	}
	if renamedChannel.Name != channel.Name+"-renamed" {
		t.Fatalf("renamed channel name = %q", renamedChannel.Name)
	}
	foundRenameEvent := false
	for _, event := range renameEvents {
		if event.Event.Type == "channel.updated" {
			foundRenameEvent = true
			break
		}
	}
	if !foundRenameEvent {
		t.Fatalf("channel rename did not emit channel.updated: %+v", renameEvents)
	}
	channel = renamedChannel

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

func TestPostgresDeletedMessagesAreExcludedFromUnreadAndAIContext(t *testing.T) {
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

	channel, _, err := repository.CreateChannel(ctx, "u-naoki", channelRequest{
		Name:      "pg-deleted-context-" + randomID(),
		Group:     "Product",
		MemberIDs: []string{"u-ken"},
	})
	if err != nil {
		t.Fatalf("create channel: %v", err)
	}
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_, _ = repository.pool.Exec(cleanupCtx, `DELETE FROM channels WHERE id=$1`, channel.ID)
	}()

	message, _, err := repository.CreateMessage(ctx, channel.ID, "u-ken", messageRequest{Body: "deleted PostgreSQL context"})
	if err != nil {
		t.Fatalf("create message: %v", err)
	}
	if _, _, err := repository.DeleteMessage(ctx, message.ID, "u-ken"); err != nil {
		t.Fatalf("delete message: %v", err)
	}
	unread, unreadCount, err := repository.ListUnreadMessageContext(ctx, "u-naoki", channel.ID, 0)
	if err != nil {
		t.Fatalf("list unread messages: %v", err)
	}
	if unreadCount != 0 || len(unread) != 0 {
		t.Fatalf("deleted message counted as unread: count=%d messages=%+v", unreadCount, unread)
	}
	contextMessages, err := repository.ListAIContextMessages(ctx, channel.ID, 100)
	if err != nil {
		t.Fatalf("list AI context: %v", err)
	}
	if containsMessageID(contextMessages, message.ID) {
		t.Fatalf("deleted message included in AI context: %+v", contextMessages)
	}
}

func TestPostgresExpiredSessionsAreRemoved(t *testing.T) {
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

	expiredTokenHash := "expired-session-" + randomID()
	activeTokenHash := "active-session-" + randomID()
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_, _ = repository.pool.Exec(cleanupCtx, `DELETE FROM sessions WHERE token_hash IN ($1,$2)`, expiredTokenHash, activeTokenHash)
	}()

	_, err = repository.pool.Exec(ctx, `INSERT INTO sessions (token_hash,user_id,expires_at) VALUES ($1,'u-naoki',now()-interval '1 minute'),($2,'u-naoki',now()+interval '1 hour')`, expiredTokenHash, activeTokenHash)
	if err != nil {
		t.Fatalf("insert session cleanup fixtures: %v", err)
	}
	repository.deleteExpiredSessions(ctx)

	var expiredCount, activeCount int
	if err := repository.pool.QueryRow(ctx, `SELECT count(*) FROM sessions WHERE token_hash=$1`, expiredTokenHash).Scan(&expiredCount); err != nil {
		t.Fatalf("count expired session: %v", err)
	}
	if err := repository.pool.QueryRow(ctx, `SELECT count(*) FROM sessions WHERE token_hash=$1`, activeTokenHash).Scan(&activeCount); err != nil {
		t.Fatalf("count active session: %v", err)
	}
	if expiredCount != 0 || activeCount != 1 {
		t.Fatalf("session cleanup counts = expired:%d active:%d, want expired:0 active:1", expiredCount, activeCount)
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

	channel, _, err := repository.CreateChannel(ctx, "u-naoki", channelRequest{Name: "pg-late-private-" + randomID(), Group: "Product", MemberIDs: []string{"u-ken"}})
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

	contextRoot, _, err := repository.CreateMessage(ctx, channel.ID, "u-ken", messageRequest{Body: "postgres AI context root"})
	if err != nil {
		t.Fatalf("create AI context root: %v", err)
	}
	contextReply, _, err := repository.CreateMessage(ctx, channel.ID, "u-ken", messageRequest{Body: "postgres AI context thread reply", ParentMessageID: contextRoot.ID})
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

func TestPostgresReadCursorDoesNotMoveBack(t *testing.T) {
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

	channel, _, err := repository.CreateChannel(ctx, "u-naoki", channelRequest{Name: "pg-read-cursor-" + randomID(), Group: "Product"})
	if err != nil {
		t.Fatalf("create channel: %v", err)
	}
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_, _ = repository.pool.Exec(cleanupCtx, `DELETE FROM channels WHERE id=$1`, channel.ID)
	}()

	first, _, err := repository.CreateMessage(ctx, channel.ID, "u-naoki", messageRequest{Body: "first"})
	if err != nil {
		t.Fatalf("create first message: %v", err)
	}
	second, _, err := repository.CreateMessage(ctx, channel.ID, "u-naoki", messageRequest{Body: "second"})
	if err != nil {
		t.Fatalf("create second message: %v", err)
	}
	if second.Sequence <= first.Sequence {
		t.Fatalf("message sequences did not increase: first=%d second=%d", first.Sequence, second.Sequence)
	}

	if err := repository.upsertChannelReadState(ctx, "u-naoki", channel.ID, second.Sequence, second.ID); err != nil {
		t.Fatalf("write latest read cursor: %v", err)
	}
	if err := repository.upsertChannelReadState(ctx, "u-naoki", channel.ID, first.Sequence, first.ID); err != nil {
		t.Fatalf("write stale read cursor: %v", err)
	}

	var sequence int64
	var messageID string
	if err := repository.pool.QueryRow(ctx, `SELECT last_read_sequence, COALESCE(last_read_message_id,'') FROM channel_read_states WHERE user_id=$1 AND channel_id=$2`, "u-naoki", channel.ID).Scan(&sequence, &messageID); err != nil {
		t.Fatalf("read persisted cursor: %v", err)
	}
	if sequence != second.Sequence || messageID != second.ID {
		t.Fatalf("read cursor moved backwards: sequence=%d message_id=%q, want sequence=%d message_id=%q", sequence, messageID, second.Sequence, second.ID)
	}
}

func TestPostgresConcurrentThreadReplyDeletionKeepsCount(t *testing.T) {
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

	channel, _, err := repository.CreateChannel(ctx, "u-naoki", channelRequest{Name: "pg-thread-delete-" + randomID(), Group: "Product"})
	if err != nil {
		t.Fatalf("create channel: %v", err)
	}
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_, _ = repository.pool.Exec(cleanupCtx, `DELETE FROM channels WHERE id=$1`, channel.ID)
	}()

	root, _, err := repository.CreateMessage(ctx, channel.ID, "u-naoki", messageRequest{Body: "thread root"})
	if err != nil {
		t.Fatalf("create root: %v", err)
	}
	replyA, _, err := repository.CreateMessage(ctx, channel.ID, "u-naoki", messageRequest{Body: "reply A", ParentMessageID: root.ID})
	if err != nil {
		t.Fatalf("create reply A: %v", err)
	}
	if _, _, err := repository.CreateMessage(ctx, channel.ID, "u-naoki", messageRequest{Body: "reply B", ParentMessageID: root.ID}); err != nil {
		t.Fatalf("create reply B: %v", err)
	}
	if _, _, err := repository.CreateMessage(ctx, channel.ID, "u-naoki", messageRequest{Body: "reply C", ParentMessageID: root.ID}); err != nil {
		t.Fatalf("create reply C: %v", err)
	}

	start := make(chan struct{})
	results := make(chan error, 2)
	var waitGroup sync.WaitGroup
	for i := 0; i < 2; i++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			<-start
			_, _, deleteErr := repository.DeleteMessage(ctx, replyA.ID, "u-naoki")
			results <- deleteErr
		}()
	}
	close(start)
	waitGroup.Wait()
	close(results)

	successes := 0
	for deleteErr := range results {
		if deleteErr == nil {
			successes++
			continue
		}
		if !errors.Is(deleteErr, ErrNotFound) {
			t.Fatalf("unexpected concurrent delete error: %v", deleteErr)
		}
	}
	if successes != 1 {
		t.Fatalf("successful deletes = %d, want 1", successes)
	}

	updatedRoot, err := repository.getMessage(ctx, root.ID)
	if err != nil {
		t.Fatalf("read root after concurrent delete: %v", err)
	}
	if updatedRoot.ThreadCount != 2 {
		t.Fatalf("thread count after concurrent delete = %d, want 2", updatedRoot.ThreadCount)
	}
}

func TestPostgresSearchAndThreadRootsUseTopLevelAccessibleMessages(t *testing.T) {
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

	channel, _, err := repository.CreateChannel(ctx, "u-naoki", channelRequest{Name: "pg-search-" + randomID(), Group: "Product", MemberIDs: []string{"u-ken"}})
	if err != nil {
		t.Fatalf("create channel: %v", err)
	}
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_, _ = repository.pool.Exec(cleanupCtx, `DELETE FROM channels WHERE id=$1`, channel.ID)
	}()

	root, _, err := repository.CreateMessage(ctx, channel.ID, "u-ken", messageRequest{Body: "pg-search-root"})
	if err != nil {
		t.Fatalf("create root: %v", err)
	}
	if _, _, err := repository.CreateMessage(ctx, channel.ID, "u-ken", messageRequest{Body: "pg-search-reply", ParentMessageID: root.ID}); err != nil {
		t.Fatalf("create reply: %v", err)
	}

	results, err := repository.SearchMessages(ctx, channel.ID, "pg-search", 50)
	if err != nil {
		t.Fatalf("search messages: %v", err)
	}
	if len(results) != 1 || results[0].ID != root.ID {
		t.Fatalf("PostgreSQL search returned thread reply as root: %+v", results)
	}
	threadRoots, err := repository.ListThreadRoots(ctx, "u-naoki", 50)
	if err != nil {
		t.Fatalf("list thread roots: %v", err)
	}
	if threadRoots.Total < 1 || !containsMessageID(threadRoots.Messages, root.ID) {
		t.Fatalf("PostgreSQL thread roots = %+v, total = %d", threadRoots.Messages, threadRoots.Total)
	}
	if _, _, err := repository.DeleteMessage(ctx, root.ID, "u-ken"); err != nil {
		t.Fatalf("delete PostgreSQL thread root: %v", err)
	}
	threadRoots, err = repository.ListThreadRoots(ctx, "u-naoki", 50)
	if err != nil {
		t.Fatalf("list thread roots after root deletion: %v", err)
	}
	var deletedRoot *Message
	for _, message := range threadRoots.Messages {
		if message.ID == root.ID {
			messageCopy := message
			deletedRoot = &messageCopy
			break
		}
	}
	if deletedRoot == nil || !deletedRoot.Deleted {
		t.Fatalf("PostgreSQL deleted thread root was not discoverable as a tombstone: %+v", deletedRoot)
	}
}
