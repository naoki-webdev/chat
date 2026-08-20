package main

import (
	"context"
	"embed"

	"github.com/jackc/pgx/v5/pgxpool"
)

type postgresRepository struct {
	pool *pgxpool.Pool
}

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
	return repository, nil
}

func (r *postgresRepository) Close() { r.pool.Close() }
