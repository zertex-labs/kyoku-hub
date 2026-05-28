package database

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

func Connect(ctx context.Context, dbURL string, logger *zap.Logger) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	logger.Info("connected to database")
	return pool, nil
}

func Migrate(ctx context.Context, pool *pgxpool.Pool, logger *zap.Logger) error {
	schema := `
	CREATE TABLE IF NOT EXISTS artists (
		mb_artist_id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		sort_name TEXT DEFAULT '',
		disambiguation TEXT DEFAULT '',
		type TEXT DEFAULT '',
		country TEXT DEFAULT '',
		image_url TEXT DEFAULT '',
		description TEXT DEFAULT '',
		fallback_release_group_id TEXT DEFAULT '',
		last_fetched_at TIMESTAMPTZ,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);

	CREATE TABLE IF NOT EXISTS release_groups (
		mb_release_group_id TEXT PRIMARY KEY,
		mb_artist_id TEXT NOT NULL REFERENCES artists(mb_artist_id) ON DELETE CASCADE,
		title TEXT NOT NULL,
		primary_type TEXT DEFAULT '',
		secondary_types TEXT DEFAULT '',
		disambiguation TEXT DEFAULT '',
		first_release_date TEXT DEFAULT '',
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);

	CREATE INDEX IF NOT EXISTS idx_release_groups_artist ON release_groups(mb_artist_id);

	CREATE TABLE IF NOT EXISTS releases (
		mb_release_id TEXT PRIMARY KEY,
		mb_release_group_id TEXT NOT NULL REFERENCES release_groups(mb_release_group_id) ON DELETE CASCADE,
		title TEXT NOT NULL,
		status TEXT DEFAULT '',
		country TEXT DEFAULT '',
		date TEXT DEFAULT '',
		barcode TEXT DEFAULT '',
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);

	CREATE INDEX IF NOT EXISTS idx_releases_release_group ON releases(mb_release_group_id);

	CREATE TABLE IF NOT EXISTS hub_instances (
		id TEXT PRIMARY KEY,
		public_key TEXT NOT NULL,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);

	CREATE TABLE IF NOT EXISTS hub_subscriptions (
		hub_instance_id TEXT NOT NULL REFERENCES hub_instances(id) ON DELETE CASCADE,
		mb_artist_id TEXT NOT NULL REFERENCES artists(mb_artist_id) ON DELETE CASCADE,
		subscribed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		PRIMARY KEY (hub_instance_id, mb_artist_id)
	);

	CREATE TABLE IF NOT EXISTS artist_requests (
		mb_artist_id TEXT NOT NULL REFERENCES artists(mb_artist_id) ON DELETE CASCADE,
		hub_instance_id TEXT NOT NULL REFERENCES hub_instances(id) ON DELETE CASCADE,
		request_count INT NOT NULL DEFAULT 0,
		last_requested_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		PRIMARY KEY (mb_artist_id, hub_instance_id)
	);
	`

	if _, err := pool.Exec(ctx, schema); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	logger.Info("database migrations complete")
	return nil
}
