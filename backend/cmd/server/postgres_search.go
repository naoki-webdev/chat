package main

import (
	"context"
	"strings"
)

func (r *postgresRepository) SearchMessages(ctx context.Context, channelID, query string, limit int) ([]Message, error) {
	pattern := escapeILikePattern(strings.TrimSpace(query))
	rows, err := r.pool.Query(ctx, messageSelect+` WHERE m.channel_id=$1 AND m.parent_message_id IS NULL AND m.deleted_at IS NULL AND (m.body ILIKE '%' || $2 || '%' ESCAPE '\' OR u.name ILIKE '%' || $2 || '%' ESCAPE '\' OR u.handle ILIKE '%' || $2 || '%' ESCAPE '\') ORDER BY m.created_sequence DESC LIMIT $3`, channelID, pattern, normalizeLimit(limit))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	results := make([]Message, 0, limit)
	for rows.Next() {
		message, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, message)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for left, right := 0, len(results)-1; left < right; left, right = left+1, right-1 {
		results[left], results[right] = results[right], results[left]
	}
	return results, nil
}

func escapeILikePattern(value string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(value)
}
