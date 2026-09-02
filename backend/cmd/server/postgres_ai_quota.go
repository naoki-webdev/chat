package main

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

func (r *postgresRepository) ConsumeAIDailyQuota(ctx context.Context, userID string, day time.Time, limit int) (bool, error) {
	var count int
	err := r.pool.QueryRow(ctx, `
INSERT INTO ai_daily_usage (user_id, usage_date, request_count)
VALUES ($1, $2::date, 1)
ON CONFLICT (user_id, usage_date) DO UPDATE
SET request_count = ai_daily_usage.request_count + 1
WHERE ai_daily_usage.request_count < $3
RETURNING request_count`, userID, day.UTC().Format("2006-01-02"), limit).Scan(&count)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}
