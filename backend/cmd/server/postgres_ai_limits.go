package main

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

const aiRequestLease = 90 * time.Second

// acquireAIRequest is an atomic lease shared by every application instance
// using the same PostgreSQL database.
func (r *postgresRepository) acquireAIRequest(ctx context.Context, key string, now time.Time, minInterval time.Duration) (bool, error) {
	var acquired bool
	err := r.pool.QueryRow(ctx, `
INSERT INTO ai_request_limits (request_key, in_flight, last_started_at, updated_at)
VALUES ($1, TRUE, $2, $2)
ON CONFLICT (request_key) DO UPDATE
SET in_flight=TRUE, last_started_at=EXCLUDED.last_started_at, updated_at=EXCLUDED.updated_at
WHERE (NOT ai_request_limits.in_flight OR ai_request_limits.updated_at <= $3)
  AND (ai_request_limits.last_started_at IS NULL OR ai_request_limits.last_started_at <= $4)
RETURNING TRUE`, key, now, now.Add(-aiRequestLease), now.Add(-minInterval)).Scan(&acquired)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return acquired, err
}

func (r *postgresRepository) releaseAIRequest(ctx context.Context, key string) error {
	_, err := r.pool.Exec(ctx, `UPDATE ai_request_limits SET in_flight=FALSE, updated_at=now() WHERE request_key=$1`, key)
	return err
}
