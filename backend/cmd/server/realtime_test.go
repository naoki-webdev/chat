package main

import (
	"context"
	"testing"
)

type countingRepository struct {
	repository
	snapshotCalls int
	memberCalls   int
}

func (r *countingRepository) ListChannelMemberIDs(ctx context.Context, channelID string) (map[string]struct{}, error) {
	r.snapshotCalls++
	return r.repository.ListChannelMemberIDs(ctx, channelID)
}

func (r *countingRepository) IsChannelMember(ctx context.Context, userID, channelID string) (bool, error) {
	r.memberCalls++
	return r.repository.IsChannelMember(ctx, userID, channelID)
}

func TestBroadcastUsesOneMembershipSnapshotPerEvent(t *testing.T) {
	countingStore := &countingRepository{repository: newMemoryRepository()}
	server := newServerWithRepository(countingStore)

	server.broadcast(realtimeEvent{Type: "message.created", ChannelID: "general"})

	if countingStore.snapshotCalls != 1 {
		t.Fatalf("membership snapshot calls = %d, want 1", countingStore.snapshotCalls)
	}
	if countingStore.memberCalls != 0 {
		t.Fatalf("per-client membership calls = %d, want 0", countingStore.memberCalls)
	}
}

func TestHubDisconnectsSlowClientOnBufferOverflow(t *testing.T) {
	hub := newHub()
	slowClient := &client{
		channelID: "general",
		user:      User{ID: "u-naoki"},
		hub:       hub,
		send:      make(chan []byte, 1),
		done:      make(chan struct{}),
	}
	slowClient.send <- []byte("already buffered")
	hub.add(slowClient)

	hub.broadcast("general", []byte("next"), map[string]struct{}{"u-naoki": {}})

	select {
	case <-slowClient.done:
	default:
		t.Fatal("slow client should be disconnected when its send buffer is full")
	}
	hub.remove(slowClient)
}
