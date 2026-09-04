package main

import "testing"

func TestDecodePersistedRealtimeEvent(t *testing.T) {
	event, err := decodePersistedRealtimeEvent(42, "message.created", "general", "m-1", "", "u-1", []byte(`{"id":"m-1","body":"hello"}`))
	if err != nil {
		t.Fatalf("decode event: %v", err)
	}
	if event.Sequence != 42 || event.EventID != 42 || event.Message == nil || event.Message.ID != "m-1" {
		t.Fatalf("unexpected event: %+v", event)
	}
}

func TestDecodePersistedRealtimeEventAllowsEmptyPayload(t *testing.T) {
	event, err := decodePersistedRealtimeEvent(42, "channel.updated", "general", "", "", "", nil)
	if err != nil {
		t.Fatalf("decode event without payload: %v", err)
	}
	if event.Message != nil {
		t.Fatalf("expected no message payload, got %+v", event.Message)
	}
}
