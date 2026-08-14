package store

import (
	"context"
	"database/sql"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/tonymontoya/ceph-atlas/internal/providers"
)

const currentSnapshotCTE = `WITH current_cluster AS (
	SELECT id AS snapshot_id
	FROM inventory_snapshots
	ORDER BY observed_at DESC, id DESC
	LIMIT 1
)
`

type PostgresStore struct {
	db *sql.DB
}

func OpenPostgres(ctx context.Context, databaseURL string) (*sql.DB, error) {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, err
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func NewPostgres(db *sql.DB) *PostgresStore {
	return &PostgresStore{db: db}
}

func notFound(message string) providers.ProviderError {
	return providers.ProviderError{Class: providers.ErrorNotFound, Message: message}
}

type rowScanner interface {
	Scan(dest ...any) error
}
