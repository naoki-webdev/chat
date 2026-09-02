package main

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
)

func (r *postgresRepository) ListEvents(ctx context.Context, userID string, after int64, limit int) (EventPage, error) {
	limit = normalizeLimit(limit)
	transaction, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return EventPage{}, err
	}
	defer transaction.Rollback(ctx)
	var cursor int64
	if err := transaction.QueryRow(ctx, `SELECT COALESCE(max(sequence),0) FROM chat_events`).Scan(&cursor); err != nil {
		return EventPage{}, err
	}
	rows, err := transaction.Query(ctx, `SELECT e.sequence,e.event_type,e.channel_id,COALESCE(e.message_id,''),COALESCE(e.parent_message_id,''),COALESCE(e.member_id,''),e.payload FROM chat_events e LEFT JOIN channel_members cm ON cm.channel_id=e.channel_id AND cm.user_id=$1 WHERE e.sequence>$2 AND e.sequence<=$3 AND (cm.user_id IS NOT NULL OR (e.event_type='channel.member_removed' AND e.member_id=$1)) ORDER BY e.sequence LIMIT $4`, userID, after, cursor, limit+1)
	if err != nil {
		return EventPage{}, err
	}
	defer rows.Close()
	events := make([]realtimeEvent, 0, limit+1)
	for rows.Next() {
		var sequence int64
		var eventType, channelID, messageID, parentMessageID, memberID string
		var payload []byte
		if err := rows.Scan(&sequence, &eventType, &channelID, &messageID, &parentMessageID, &memberID, &payload); err != nil {
			return EventPage{}, err
		}
		event := realtimeEvent{Type: eventType, ChannelID: channelID, EventID: sequence, Sequence: sequence, MessageID: messageID, ParentMessageID: parentMessageID, MemberID: memberID}
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
	if err := transaction.Commit(ctx); err != nil {
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
	if err := transaction.QueryRow(ctx, `INSERT INTO chat_events (event_type,channel_id,message_id,parent_message_id,member_id,payload) VALUES ($1,$2,$3,$4,$5,$6) RETURNING sequence`, event.Type, event.ChannelID, nullableString(messageID), nullableString(parentMessageID), nullableString(event.MemberID), payload).Scan(&sequence); err != nil {
		return EventRecord{}, err
	}
	if err := notifyPersistedEvent(ctx, transaction, sequence); err != nil {
		return EventRecord{}, err
	}
	event.EventID = sequence
	event.Sequence = sequence
	return EventRecord{Sequence: sequence, Event: event}, nil
}
