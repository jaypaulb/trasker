package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Store provides access to the PostgreSQL database.
type Store struct {
	pool *pgxpool.Pool
}

// New creates a Store wrapping the given connection pool.
func New(ctx context.Context, pool *pgxpool.Pool) (*Store, error) {
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("pinging database: %w", err)
	}
	return &Store{pool: pool}, nil
}

// Ping verifies the database connection is alive.
func (s *Store) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

// Pool returns the underlying connection pool (for use by sub-stores).
func (s *Store) Pool() *pgxpool.Pool {
	return s.pool
}

// ConnectPool creates a new pgxpool from a connection string.
// Use this in production; tests provide their own pool.
func ConnectPool(ctx context.Context, connString string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return nil, fmt.Errorf("parsing connection string: %w", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("creating connection pool: %w", err)
	}
	return pool, nil
}
