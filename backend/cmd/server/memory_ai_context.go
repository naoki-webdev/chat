package main

import (
	"context"
	"sort"
)

func (r *memoryRepository) ListAIContextMessages(_ context.Context, channelID string, limit int) ([]Message, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if !r.hasChannelLocked(channelID) {
		return nil, ErrNotFound
	}
	items := make([]Message, 0, len(r.messages[channelID]))
	for _, message := range r.messages[channelID] {
		message.Sequence = r.messageSequences[message.ID]
		items = append(items, cloneMessage(message))
	}
	sort.SliceStable(items, func(left, right int) bool { return items[left].Sequence < items[right].Sequence })
	if limit > 0 && len(items) > limit {
		items = items[len(items)-limit:]
	}
	return items, nil
}

func (r *memoryRepository) ListUnreadMessages(_ context.Context, userID, channelID string) ([]Message, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if !r.hasChannelLocked(channelID) {
		return nil, ErrNotFound
	}
	readAt := int64(0)
	if states := r.readStates[userID]; states != nil {
		readAt = states[channelID]
	}
	if readAt == 0 {
		if states := r.readMessageIDs[userID]; states != nil {
			readAt = r.messageSequences[states[channelID]]
		}
	}
	items := make([]Message, 0)
	for _, message := range r.messages[channelID] {
		message.Sequence = r.messageSequences[message.ID]
		if message.Sequence > readAt {
			items = append(items, cloneMessage(message))
		}
	}
	sort.SliceStable(items, func(left, right int) bool { return items[left].Sequence < items[right].Sequence })
	return items, nil
}
