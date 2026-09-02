package main

import (
	"context"
	"sort"
	"time"
)

func (r *postgresRepository) allowAuthAttempts(ctx context.Context, keys []string, now time.Time, maxAttempts int, window time.Duration) (bool, error) {
	keys = uniqueNonEmpty(keys)
	sort.Strings(keys)
	if len(keys) == 0 {
		return true, nil
	}
	transaction, err := r.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer transaction.Rollback(ctx)

	expired := make([]string, 0, len(keys))
	blocked := false
	for _, key := range keys {
		if _, err := transaction.Exec(ctx, `INSERT INTO auth_rate_limit_buckets (bucket_key, window_started_at, attempts) VALUES ($1,$2,0) ON CONFLICT (bucket_key) DO NOTHING`, key, now); err != nil {
			return false, err
		}
		var startedAt time.Time
		var attempts int
		if err := transaction.QueryRow(ctx, `SELECT window_started_at, attempts FROM auth_rate_limit_buckets WHERE bucket_key=$1 FOR UPDATE`, key).Scan(&startedAt, &attempts); err != nil {
			return false, err
		}
		if now.Sub(startedAt) >= window {
			expired = append(expired, key)
			attempts = 0
		}
		if attempts >= maxAttempts {
			blocked = true
		}
	}
	if blocked {
		return false, nil
	}
	for _, key := range expired {
		if _, err := transaction.Exec(ctx, `UPDATE auth_rate_limit_buckets SET window_started_at=$2, attempts=0 WHERE bucket_key=$1`, key, now); err != nil {
			return false, err
		}
	}
	for _, key := range keys {
		if _, err := transaction.Exec(ctx, `UPDATE auth_rate_limit_buckets SET attempts=attempts+1 WHERE bucket_key=$1`, key); err != nil {
			return false, err
		}
	}
	if err := transaction.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}

func (r *postgresRepository) resetAuthAttempts(ctx context.Context, keys []string) error {
	keys = uniqueNonEmpty(keys)
	if len(keys) == 0 {
		return nil
	}
	_, err := r.pool.Exec(ctx, `DELETE FROM auth_rate_limit_buckets WHERE bucket_key = ANY($1)`, keys)
	return err
}
