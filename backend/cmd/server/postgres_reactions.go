package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
)

func (r *postgresRepository) AddReaction(ctx context.Context, messageID, userID, emoji string) (Message, EventRecord, error) {
	emoji = strings.TrimSpace(emoji)
	if err := validateReactionEmoji(emoji); err != nil {
		return Message{}, EventRecord{}, err
	}
	transaction, err := r.pool.Begin(ctx)
	if err != nil {
		return Message{}, EventRecord{}, err
	}
	defer transaction.Rollback(ctx)
	var channelID string
	var rawReactions []byte
	var deleted bool
	if err := transaction.QueryRow(ctx, `SELECT channel_id,reactions,(deleted_at IS NOT NULL) FROM messages WHERE id=$1 FOR UPDATE`, messageID).Scan(&channelID, &rawReactions, &deleted); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Message{}, EventRecord{}, ErrNotFound
		}
		return Message{}, EventRecord{}, err
	}
	if deleted {
		return Message{}, EventRecord{}, ErrConflict
	}
	result, err := transaction.Exec(ctx, `INSERT INTO message_reactions (message_id,user_id,emoji) VALUES ($1,$2,$3) ON CONFLICT (message_id,user_id,emoji) DO NOTHING`, messageID, userID, emoji)
	if err != nil {
		if isForeignKeyViolation(err) {
			return Message{}, EventRecord{}, ErrUnauthorized
		}
		return Message{}, EventRecord{}, err
	}
	message, err := r.getMessageFrom(ctx, transaction, messageID)
	if err != nil {
		return Message{}, EventRecord{}, err
	}
	if result.RowsAffected() == 0 {
		setReactionState(&message, emoji, true)
		if err := transaction.Commit(ctx); err != nil {
			return Message{}, EventRecord{}, err
		}
		return message, EventRecord{}, nil
	}
	message.Reactions = reactionsFromJSON(rawReactions)
	incrementReaction(&message, emoji)
	encoded, err := json.Marshal(message.Reactions)
	if err != nil {
		return Message{}, EventRecord{}, err
	}
	if _, err := transaction.Exec(ctx, `UPDATE messages SET reactions=$1::jsonb WHERE id=$2`, encoded, messageID); err != nil {
		return Message{}, EventRecord{}, err
	}
	message, err = r.getMessageFrom(ctx, transaction, messageID)
	if err != nil {
		return Message{}, EventRecord{}, err
	}
	setReactionState(&message, emoji, true)
	eventMessage := cloneMessage(message)
	clearReactionState(&eventMessage)
	record, err := appendEventTx(ctx, transaction, realtimeEvent{Type: "reaction.added", ChannelID: channelID, Message: pointerToMessage(eventMessage)})
	if err != nil {
		return Message{}, EventRecord{}, err
	}
	if err := transaction.Commit(ctx); err != nil {
		return Message{}, EventRecord{}, err
	}
	record.Event.Message = pointerToMessage(eventMessage)
	return message, record, nil
}

func (r *postgresRepository) RemoveReaction(ctx context.Context, messageID, userID, emoji string) (Message, EventRecord, error) {
	emoji = strings.TrimSpace(emoji)
	if err := validateReactionEmoji(emoji); err != nil {
		return Message{}, EventRecord{}, err
	}
	transaction, err := r.pool.Begin(ctx)
	if err != nil {
		return Message{}, EventRecord{}, err
	}
	defer transaction.Rollback(ctx)
	var channelID string
	var rawReactions []byte
	var deleted bool
	if err := transaction.QueryRow(ctx, `SELECT channel_id,reactions,(deleted_at IS NOT NULL) FROM messages WHERE id=$1 FOR UPDATE`, messageID).Scan(&channelID, &rawReactions, &deleted); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Message{}, EventRecord{}, ErrNotFound
		}
		return Message{}, EventRecord{}, err
	}
	if deleted {
		return Message{}, EventRecord{}, ErrConflict
	}
	result, err := transaction.Exec(ctx, `DELETE FROM message_reactions WHERE message_id=$1 AND user_id=$2 AND emoji=$3`, messageID, userID, emoji)
	if err != nil {
		return Message{}, EventRecord{}, err
	}
	message, err := r.getMessageFrom(ctx, transaction, messageID)
	if err != nil {
		return Message{}, EventRecord{}, err
	}
	if result.RowsAffected() == 0 {
		if err := transaction.Commit(ctx); err != nil {
			return Message{}, EventRecord{}, err
		}
		return message, EventRecord{}, nil
	}
	message.Reactions = reactionsFromJSON(rawReactions)
	decrementReaction(&message, emoji)
	encoded, err := json.Marshal(message.Reactions)
	if err != nil {
		return Message{}, EventRecord{}, err
	}
	if _, err := transaction.Exec(ctx, `UPDATE messages SET reactions=$1::jsonb WHERE id=$2`, encoded, messageID); err != nil {
		return Message{}, EventRecord{}, err
	}
	message, err = r.getMessageFrom(ctx, transaction, messageID)
	if err != nil {
		return Message{}, EventRecord{}, err
	}
	eventMessage := cloneMessage(message)
	clearReactionState(&eventMessage)
	record, err := appendEventTx(ctx, transaction, realtimeEvent{Type: "reaction.removed", ChannelID: channelID, Message: pointerToMessage(eventMessage)})
	if err != nil {
		return Message{}, EventRecord{}, err
	}
	if err := transaction.Commit(ctx); err != nil {
		return Message{}, EventRecord{}, err
	}
	record.Event.Message = pointerToMessage(eventMessage)
	return message, record, nil
}

func reactionsFromJSON(raw []byte) []Reaction {
	var reactions []Reaction
	if len(raw) > 0 && string(raw) != "null" {
		_ = json.Unmarshal(raw, &reactions)
	}
	return reactions
}
