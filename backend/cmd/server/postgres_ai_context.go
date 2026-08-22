package main

import (
	"context"

	"github.com/jackc/pgx/v5"
)

func (r *postgresRepository) ListAIContextMessages(ctx context.Context, channelID string, limit int) ([]Message, error) {
	limit = normalizeLimit(limit)
	rows, err := r.pool.Query(ctx, messageSelect+` WHERE m.channel_id=$1 ORDER BY m.created_sequence DESC LIMIT $2`, channelID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Message, 0, limit)
	for rows.Next() {
		message, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, message)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for left, right := 0, len(items)-1; left < right; left, right = left+1, right-1 {
		items[left], items[right] = items[right], items[left]
	}
	return items, nil
}

func (r *postgresRepository) ListUnreadMessages(ctx context.Context, userID, channelID string) ([]Message, error) {
	items, _, err := r.ListUnreadMessageContext(ctx, userID, channelID, 0)
	return items, err
}

func (r *postgresRepository) ListUnreadMessageContext(ctx context.Context, userID, channelID string, limit int) ([]Message, int, error) {
	transaction, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, 0, err
	}
	defer transaction.Rollback(ctx)
	var unreadCount int
	if err := transaction.QueryRow(ctx, `SELECT count(*) FROM messages m WHERE m.channel_id=$1 AND m.created_sequence > COALESCE((SELECT last_read_sequence FROM channel_read_states WHERE user_id=$2 AND channel_id=$1),0)`, channelID, userID).Scan(&unreadCount); err != nil {
		return nil, 0, err
	}
	query := messageSelect + ` WHERE m.channel_id=$1 AND m.created_sequence > COALESCE((SELECT last_read_sequence FROM channel_read_states WHERE user_id=$2 AND channel_id=$1),0) ORDER BY m.created_sequence DESC`
	args := []any{channelID, userID}
	if limit > 0 {
		query += ` LIMIT $3`
		args = append(args, limit)
	}
	rows, err := transaction.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	capacity := unreadCount
	if limit > 0 && capacity > limit {
		capacity = limit
	}
	items := make([]Message, 0, capacity)
	for rows.Next() {
		message, err := scanMessage(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, message)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	for left, right := 0, len(items)-1; left < right; left, right = left+1, right-1 {
		items[left], items[right] = items[right], items[left]
	}
	if err := transaction.Commit(ctx); err != nil {
		return nil, 0, err
	}
	return items, unreadCount, nil
}
