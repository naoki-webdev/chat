package main

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
)

func (r *postgresRepository) ListEvents(ctx context.Context, userID string, after int64, limit int) (EventPage, error) {
	limit = normalizeLimit(limit)
	rows, err := r.pool.Query(ctx, `SELECT e.sequence,e.event_type,e.channel_id,e.message_id,COALESCE(e.parent_message_id,''),e.payload FROM chat_events e JOIN channel_members cm ON cm.channel_id=e.channel_id AND cm.user_id=$1 WHERE e.sequence>$2 ORDER BY e.sequence LIMIT $3`, userID, after, limit+1)
	if err != nil {
		return EventPage{}, err
	}
	defer rows.Close()
	events := make([]realtimeEvent, 0, limit+1)
	for rows.Next() {
		var sequence int64
		var eventType, channelID, messageID, parentMessageID string
		var payload []byte
		if err := rows.Scan(&sequence, &eventType, &channelID, &messageID, &parentMessageID, &payload); err != nil {
			return EventPage{}, err
		}
		event := realtimeEvent{Type: eventType, ChannelID: channelID, EventID: sequence, Sequence: sequence, MessageID: messageID, ParentMessageID: parentMessageID}
		if len(payload) > 0 && string(payload) != "null" {
			if err := json.Unmarshal(payload, &event.Message); err != nil {
				return EventPage{}, err
			}
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return EventPage{}, err
	}
	hasMore := len(events) > limit
	if hasMore {
		events = events[:limit]
	}
	nextCursor := ""
	if hasMore && len(events) > 0 {
		nextCursor = cursorString(events[len(events)-1].Sequence)
	}
	var cursor int64
	if err := r.pool.QueryRow(ctx, `SELECT COALESCE(max(sequence),0) FROM chat_events`).Scan(&cursor); err != nil {
		return EventPage{}, err
	}
	return EventPage{Events: events, NextCursor: nextCursor, HasMore: hasMore, Cursor: cursor}, nil
}

type rowScannerSource interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func appendEventTx(ctx context.Context, transaction pgx.Tx, event realtimeEvent) (EventRecord, error) {
	payload := []byte(nil)
	messageID := event.MessageID
	if event.Message != nil {
		var err error
		payload, err = json.Marshal(event.Message)
		if err != nil {
			return EventRecord{}, err
		}
		messageID = event.Message.ID
	}
	parentMessageID := event.ParentMessageID
	if parentMessageID == "" && event.Message != nil {
		parentMessageID = event.Message.ParentMessageID
	}
	var sequence int64
	if err := transaction.QueryRow(ctx, `INSERT INTO chat_events (event_type,channel_id,message_id,parent_message_id,payload) VALUES ($1,$2,$3,$4,$5) RETURNING sequence`, event.Type, event.ChannelID, messageID, nullableString(parentMessageID), payload).Scan(&sequence); err != nil {
		return EventRecord{}, err
	}
	event.EventID = sequence
	event.Sequence = sequence
	return EventRecord{Sequence: sequence, Event: event}, nil
}
