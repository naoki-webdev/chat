package main

import "context"

func (r *memoryRepository) ListEvents(_ context.Context, userID string, after int64, limit int) (EventPage, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	limit = normalizeLimit(limit)
	events := make([]realtimeEvent, 0, limit)
	hasMore := false
	for _, record := range r.events {
		canReadChannel := record.Event.ChannelID == "*" || r.isChannelMemberLocked(userID, record.Event.ChannelID)
		isRemovedMember := record.Event.Type == "channel.member_removed" && record.Event.MemberID == userID
		if record.Sequence <= after || (!canReadChannel && !isRemovedMember) {
			continue
		}
		if len(events) >= limit {
			hasMore = true
			break
		}
		events = append(events, cloneEvent(record.Event))
	}
	nextCursor := ""
	if hasMore && len(events) > 0 {
		nextCursor = cursorString(events[len(events)-1].Sequence)
	}
	return EventPage{Events: events, NextCursor: nextCursor, HasMore: hasMore, Cursor: r.sequence}, nil
}
