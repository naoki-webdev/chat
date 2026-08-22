package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
)

func (r *postgresRepository) MarkChannelRead(ctx context.Context, userID, channelID string) (int64, error) {
	var latest int64
	var latestMessageID string
	if err := r.pool.QueryRow(ctx, `SELECT COALESCE(max(created_sequence),0), COALESCE((SELECT id FROM messages WHERE channel_id=$1 ORDER BY created_sequence DESC, id DESC LIMIT 1),'') FROM messages WHERE channel_id=$1`, channelID).Scan(&latest, &latestMessageID); err != nil {
		return 0, err
	}
	_, err := r.pool.Exec(ctx, `INSERT INTO channel_read_states (user_id,channel_id,last_read_sequence,last_read_message_id) VALUES ($1,$2,$3,$4) ON CONFLICT (user_id,channel_id) DO UPDATE SET last_read_sequence=excluded.last_read_sequence, last_read_message_id=excluded.last_read_message_id, updated_at=now()`, userID, channelID, latest, nullableString(latestMessageID))
	return latest, err
}

func (r *postgresRepository) ListMessagePage(ctx context.Context, channelID, before string, limit int) (MessagePage, error) {
	beforeSequence, err := cursorValue(before)
	if err != nil {
		return MessagePage{}, invalidInput("invalid cursor")
	}
	limit = normalizeLimit(limit)
	transaction, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return MessagePage{}, err
	}
	defer transaction.Rollback(ctx)
	var cursor int64
	if err := transaction.QueryRow(ctx, `SELECT COALESCE(max(sequence),0) FROM chat_events`).Scan(&cursor); err != nil {
		return MessagePage{}, err
	}
	rows, err := transaction.Query(ctx, messageSelect+` WHERE m.channel_id=$1 AND m.parent_message_id IS NULL AND ($2=0 OR m.created_sequence<$2) ORDER BY m.created_sequence DESC LIMIT $3`, channelID, beforeSequence, limit+1)
	if err != nil {
		return MessagePage{}, err
	}
	defer rows.Close()
	messages := make([]Message, 0, limit+1)
	for rows.Next() {
		message, err := scanMessage(rows)
		if err != nil {
			return MessagePage{}, err
		}
		messages = append(messages, message)
	}
	if err := rows.Err(); err != nil {
		return MessagePage{}, err
	}
	hasMore := len(messages) > limit
	if hasMore {
		messages = messages[:limit]
	}
	for left, right := 0, len(messages)-1; left < right; left, right = left+1, right-1 {
		messages[left], messages[right] = messages[right], messages[left]
	}
	nextCursor := ""
	if hasMore && len(messages) > 0 {
		nextCursor = cursorString(messages[0].Sequence)
	}
	if err := transaction.Commit(ctx); err != nil {
		return MessagePage{}, err
	}
	return MessagePage{Messages: messages, NextCursor: nextCursor, HasMore: hasMore, Cursor: cursor}, nil
}

func (r *postgresRepository) ListThreadPage(ctx context.Context, parentMessageID, before string, limit int) (MessagePage, error) {
	beforeSequence, err := cursorValue(before)
	if err != nil {
		return MessagePage{}, invalidInput("invalid cursor")
	}
	transaction, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return MessagePage{}, err
	}
	defer transaction.Rollback(ctx)
	var cursor int64
	if err := transaction.QueryRow(ctx, `SELECT COALESCE(max(sequence),0) FROM chat_events`).Scan(&cursor); err != nil {
		return MessagePage{}, err
	}
	var exists bool
	if err := transaction.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM messages WHERE id=$1)`, parentMessageID).Scan(&exists); err != nil {
		return MessagePage{}, err
	}
	if !exists {
		return MessagePage{}, ErrNotFound
	}
	limit = normalizeLimit(limit)
	rows, err := transaction.Query(ctx, messageSelect+` WHERE m.parent_message_id=$1 AND ($2=0 OR m.created_sequence<$2) ORDER BY m.created_sequence DESC LIMIT $3`, parentMessageID, beforeSequence, limit+1)
	if err != nil {
		return MessagePage{}, err
	}
	defer rows.Close()
	messages := make([]Message, 0, limit+1)
	for rows.Next() {
		message, err := scanMessage(rows)
		if err != nil {
			return MessagePage{}, err
		}
		messages = append(messages, message)
	}
	if err := rows.Err(); err != nil {
		return MessagePage{}, err
	}
	hasMore := len(messages) > limit
	if hasMore {
		messages = messages[:limit]
	}
	for left, right := 0, len(messages)-1; left < right; left, right = left+1, right-1 {
		messages[left], messages[right] = messages[right], messages[left]
	}
	nextCursor := ""
	if hasMore && len(messages) > 0 {
		nextCursor = cursorString(messages[0].Sequence)
	}
	if err := transaction.Commit(ctx); err != nil {
		return MessagePage{}, err
	}
	return MessagePage{Messages: messages, NextCursor: nextCursor, HasMore: hasMore, Cursor: cursor}, nil
}

const messageSelect = `SELECT m.id, m.channel_id, u.id, u.name, u.initials, u.color, to_char(m.created_at AT TIME ZONE 'Asia/Tokyo', 'HH24:MI'), m.body, m.edited, m.reactions, m.thread_count, COALESCE(m.parent_message_id,''), (m.deleted_at IS NOT NULL), m.created_sequence FROM messages m JOIN users u ON u.id=m.author_id`

type rowScanner interface {
	Scan(...any) error
}

func scanMessage(row rowScanner) (Message, error) {
	var message Message
	var reactions []byte
	if err := row.Scan(&message.ID, &message.ChannelID, &message.AuthorID, &message.Author, &message.Initials, &message.Color, &message.Time, &message.Body, &message.Edited, &reactions, &message.ThreadCount, &message.ParentMessageID, &message.Deleted, &message.Sequence); err != nil {
		return Message{}, err
	}
	if message.Deleted {
		message.Body = deletedMessageBody
		message.Edited = false
		message.Reactions = nil
	}
	if len(reactions) > 0 && string(reactions) != "null" {
		_ = json.Unmarshal(reactions, &message.Reactions)
	}
	return message, nil
}

func (r *postgresRepository) CreateMessage(ctx context.Context, channelID, userID string, request messageRequest) (Message, EventRecord, error) {
	body := strings.TrimSpace(request.Body)
	if err := validateMessageBody(body); err != nil {
		return Message{}, EventRecord{}, err
	}
	parentMessageID := strings.TrimSpace(request.ParentMessageID)
	transaction, err := r.pool.Begin(ctx)
	if err != nil {
		return Message{}, EventRecord{}, err
	}
	defer transaction.Rollback(ctx)
	if parentMessageID != "" {
		var parentChannelID, parentParentMessageID string
		if err := transaction.QueryRow(ctx, `SELECT channel_id,COALESCE(parent_message_id,'') FROM messages WHERE id=$1`, parentMessageID).Scan(&parentChannelID, &parentParentMessageID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return Message{}, EventRecord{}, ErrNotFound
			}
			return Message{}, EventRecord{}, err
		}
		if parentChannelID != channelID {
			return Message{}, EventRecord{}, ErrNotFound
		}
		if parentParentMessageID != "" {
			return Message{}, EventRecord{}, ErrConflict
		}
	}
	id := "m-" + randomID()
	if _, err := transaction.Exec(ctx, `INSERT INTO messages (id, channel_id, author_id, body, parent_message_id) VALUES ($1,$2,$3,$4,$5)`, id, channelID, userID, body, nullableString(parentMessageID)); err != nil {
		if isForeignKeyViolation(err) {
			return Message{}, EventRecord{}, ErrNotFound
		}
		return Message{}, EventRecord{}, err
	}
	if parentMessageID != "" {
		if _, err := transaction.Exec(ctx, `UPDATE messages SET thread_count=thread_count+1 WHERE id=$1`, parentMessageID); err != nil {
			return Message{}, EventRecord{}, err
		}
	}
	message, err := r.getMessageFrom(ctx, transaction, id)
	if err != nil {
		return Message{}, EventRecord{}, err
	}
	record, err := appendEventTx(ctx, transaction, realtimeEvent{Type: "message.created", ChannelID: channelID, Message: pointerToMessage(message)})
	if err != nil {
		return Message{}, EventRecord{}, err
	}
	message.Sequence = record.Sequence
	if _, err := transaction.Exec(ctx, `UPDATE messages SET created_sequence=$1 WHERE id=$2`, record.Sequence, id); err != nil {
		return Message{}, EventRecord{}, err
	}
	if err := transaction.Commit(ctx); err != nil {
		return Message{}, EventRecord{}, err
	}
	record.Event.Message = pointerToMessage(message)
	return message, record, nil
}

func (r *postgresRepository) getMessage(ctx context.Context, id string) (Message, error) {
	return r.getMessageFrom(ctx, r.pool, id)
}

func (r *postgresRepository) getMessageFrom(ctx context.Context, query rowScannerSource, id string) (Message, error) {
	return scanMessage(query.QueryRow(ctx, messageSelect+` WHERE m.id=$1`, id))
}

func (r *postgresRepository) UpdateMessage(ctx context.Context, messageID, userID, body string) (Message, EventRecord, error) {
	body = strings.TrimSpace(body)
	if err := validateMessageBody(body); err != nil {
		return Message{}, EventRecord{}, err
	}
	transaction, err := r.pool.Begin(ctx)
	if err != nil {
		return Message{}, EventRecord{}, err
	}
	defer transaction.Rollback(ctx)
	var ownerID, channelID string
	var deleted bool
	if err := transaction.QueryRow(ctx, `SELECT author_id,channel_id,(deleted_at IS NOT NULL) FROM messages WHERE id=$1`, messageID).Scan(&ownerID, &channelID, &deleted); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Message{}, EventRecord{}, ErrNotFound
		}
		return Message{}, EventRecord{}, err
	}
	if ownerID != userID {
		return Message{}, EventRecord{}, ErrForbidden
	}
	if deleted {
		return Message{}, EventRecord{}, ErrConflict
	}
	if _, err := transaction.Exec(ctx, `UPDATE messages SET body=$1,edited=true WHERE id=$2`, body, messageID); err != nil {
		return Message{}, EventRecord{}, err
	}
	message, err := r.getMessageFrom(ctx, transaction, messageID)
	if err != nil {
		return Message{}, EventRecord{}, err
	}
	record, err := appendEventTx(ctx, transaction, realtimeEvent{Type: "message.updated", ChannelID: channelID, Message: pointerToMessage(message)})
	if err != nil {
		return Message{}, EventRecord{}, err
	}
	if err := transaction.Commit(ctx); err != nil {
		return Message{}, EventRecord{}, err
	}
	record.Event.Message = pointerToMessage(message)
	return message, record, nil
}

func (r *postgresRepository) DeleteMessage(ctx context.Context, messageID, userID string) (string, EventRecord, error) {
	transaction, err := r.pool.Begin(ctx)
	if err != nil {
		return "", EventRecord{}, err
	}
	defer transaction.Rollback(ctx)
	var ownerID, channelID, parentMessageID string
	var threadCount int
	var deleted bool
	if err := transaction.QueryRow(ctx, `SELECT author_id,channel_id,COALESCE(parent_message_id,''),thread_count,(deleted_at IS NOT NULL) FROM messages WHERE id=$1`, messageID).Scan(&ownerID, &channelID, &parentMessageID, &threadCount, &deleted); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", EventRecord{}, ErrNotFound
		}
		return "", EventRecord{}, err
	}
	if ownerID != userID {
		return "", EventRecord{}, ErrForbidden
	}
	if deleted {
		return "", EventRecord{}, ErrConflict
	}
	if parentMessageID == "" {
		if _, err := transaction.Exec(ctx, `UPDATE messages SET deleted_at=now(), reactions='[]'::jsonb WHERE id=$1`, messageID); err != nil {
			return "", EventRecord{}, err
		}
		if _, err := transaction.Exec(ctx, `DELETE FROM message_reactions WHERE message_id=$1`, messageID); err != nil {
			return "", EventRecord{}, err
		}
		deletedMessage, err := r.getMessageFrom(ctx, transaction, messageID)
		if err != nil {
			return "", EventRecord{}, err
		}
		record, err := appendEventTx(ctx, transaction, realtimeEvent{Type: "message.deleted", ChannelID: channelID, MessageID: messageID, Message: pointerToMessage(deletedMessage)})
		if err != nil {
			return "", EventRecord{}, err
		}
		if err := transaction.Commit(ctx); err != nil {
			return "", EventRecord{}, err
		}
		return channelID, record, nil
	}
	if _, err := transaction.Exec(ctx, `DELETE FROM messages WHERE id=$1`, messageID); err != nil {
		return "", EventRecord{}, err
	}
	if parentMessageID != "" {
		if _, err := transaction.Exec(ctx, `UPDATE messages SET thread_count=GREATEST(thread_count-1,0) WHERE id=$1`, parentMessageID); err != nil {
			return "", EventRecord{}, err
		}
	}
	record, err := appendEventTx(ctx, transaction, realtimeEvent{Type: "message.deleted", ChannelID: channelID, MessageID: messageID, ParentMessageID: parentMessageID})
	if err != nil {
		return "", EventRecord{}, err
	}
	if err := transaction.Commit(ctx); err != nil {
		return "", EventRecord{}, err
	}
	return channelID, record, nil
}
