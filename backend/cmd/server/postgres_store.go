package main

import (
	"context"
	"embed"
	"log"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type postgresRepository struct {
	pool              *pgxpool.Pool
	cleanupCancel     context.CancelFunc
	cleanupDone       chan struct{}
	listenerCancel    context.CancelFunc
	listenerDone      chan struct{}
	listenerReady     chan struct{}
	listenerReadyOnce sync.Once
	listenerMu        sync.Mutex
	closeOnce         sync.Once
}

const (
	sessionCleanupInterval     = time.Hour
	sessionCleanupQueryTimeout = 5 * time.Second
)

// migrationFS keeps the schema versioned and deployable with the server binary.
//
//go:embed migrations/*.sql
var migrationFS embed.FS

func newPostgresRepository(ctx context.Context, databaseURL string) (*postgresRepository, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	repository := &postgresRepository{pool: pool}
	if err := repository.migrate(ctx); err != nil {
		repository.Close()
		return nil, err
	}
	if shouldSeedDemoData() {
		if err := repository.seed(ctx); err != nil {
			repository.Close()
			return nil, err
		}
	}
	repository.startSessionCleanup()
	return repository, nil
}

func (r *postgresRepository) startSessionCleanup() {
	cleanupContext, cancel := context.WithCancel(context.Background())
	r.cleanupCancel = cancel
	r.cleanupDone = make(chan struct{})
	go r.sessionCleanupLoop(cleanupContext)
}

func (r *postgresRepository) sessionCleanupLoop(ctx context.Context) {
	defer close(r.cleanupDone)
	r.deleteExpiredSessions(ctx)
	r.deleteExpiredRateLimitData(ctx)

	ticker := time.NewTicker(sessionCleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.deleteExpiredSessions(ctx)
			r.deleteExpiredRateLimitData(ctx)
		}
	}
}

func (r *postgresRepository) deleteExpiredRateLimitData(ctx context.Context) {
	queryContext, cancel := context.WithTimeout(ctx, sessionCleanupQueryTimeout)
	defer cancel()
	queries := []string{
		`DELETE FROM auth_rate_limit_buckets WHERE window_started_at < now() - interval '1 day'`,
		`DELETE FROM ai_daily_usage WHERE usage_date < current_date - 90`,
		`DELETE FROM ai_request_limits WHERE updated_at < now() - interval '1 day' AND in_flight = false`,
	}
	for _, query := range queries {
		if _, err := r.pool.Exec(queryContext, query); err != nil && ctx.Err() == nil {
			log.Printf("expired rate limit data cleanup failed: %v", err)
			return
		}
	}
}

func (r *postgresRepository) deleteExpiredSessions(ctx context.Context) {
	queryContext, cancel := context.WithTimeout(ctx, sessionCleanupQueryTimeout)
	defer cancel()
	if _, err := r.pool.Exec(queryContext, `DELETE FROM sessions WHERE expires_at <= now()`); err != nil && ctx.Err() == nil {
		log.Printf("expired session cleanup failed: %v", err)
	}
}

func (r *postgresRepository) Close() {
	r.closeOnce.Do(func() {
		r.listenerMu.Lock()
		listenerCancel := r.listenerCancel
		listenerDone := r.listenerDone
		r.listenerMu.Unlock()
		if listenerCancel != nil {
			listenerCancel()
			<-listenerDone
		}
		if r.cleanupCancel != nil {
			r.cleanupCancel()
			<-r.cleanupDone
		}
		r.pool.Close()
	})
}
