package main

import (
	"context"

	"github.com/jackc/pgx/v5"
)

func (r *postgresRepository) ListThreadRoots(ctx context.Context, userID string, limit int) (ThreadRootPage, error) {
	limit = normalizeLimit(limit)
	transaction, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return ThreadRootPage{}, err
	}
	defer transaction.Rollback(ctx)

	var total int
	if err := transaction.QueryRow(ctx, `
SELECT count(*)
FROM messages m
JOIN channel_members cm ON cm.channel_id=m.channel_id AND cm.user_id=$1
WHERE m.parent_message_id IS NULL AND m.thread_count > 0`, userID).Scan(&total); err != nil {
		return ThreadRootPage{}, err
	}
	rows, err := transaction.Query(ctx, messageSelect+` JOIN channel_members cm ON cm.channel_id=m.channel_id AND cm.user_id=$1 WHERE m.parent_message_id IS NULL AND m.thread_count > 0 ORDER BY m.created_sequence DESC LIMIT $2`, userID, limit)
	if err != nil {
		return ThreadRootPage{}, err
	}
	defer rows.Close()
	items := make([]Message, 0, limit)
	for rows.Next() {
		message, err := scanMessage(rows)
		if err != nil {
			return ThreadRootPage{}, err
		}
		items = append(items, message)
	}
	if err := rows.Err(); err != nil {
		return ThreadRootPage{}, err
	}
	for left, right := 0, len(items)-1; left < right; left, right = left+1, right-1 {
		items[left], items[right] = items[right], items[left]
	}
	if err := transaction.Commit(ctx); err != nil {
		return ThreadRootPage{}, err
	}
	return ThreadRootPage{Messages: items, Total: total}, nil
}
