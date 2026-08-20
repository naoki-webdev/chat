package main

import (
	"context"
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
	rows, err := r.pool.Query(ctx, messageSelect+` WHERE m.channel_id=$1 AND m.created_sequence > COALESCE((SELECT last_read_sequence FROM channel_read_states WHERE user_id=$2 AND channel_id=$1),0) ORDER BY m.created_sequence DESC`, channelID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Message, 0)
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
