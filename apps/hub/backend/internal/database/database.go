package database

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

func Connect(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return pool, nil
}

func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	schema := `
	CREATE TABLE IF NOT EXISTS users (
		id TEXT PRIMARY KEY,
		username TEXT NOT NULL,
		mb_user_id TEXT UNIQUE NOT NULL,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);

	CREATE TABLE IF NOT EXISTS sessions (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		expires_at TIMESTAMPTZ NOT NULL,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);

	CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);
	CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);

	CREATE TABLE IF NOT EXISTS artists (
		mb_artist_id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		disambiguation TEXT DEFAULT '',
		image_url TEXT DEFAULT '',
		description TEXT DEFAULT '',
		release_group_count INTEGER NOT NULL DEFAULT 0,
		status TEXT NOT NULL DEFAULT 'queued',
		active BOOLEAN NOT NULL DEFAULT true,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		synced_at TIMESTAMPTZ
	);

	CREATE TABLE IF NOT EXISTS user_artists (
		user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		mb_artist_id TEXT NOT NULL REFERENCES artists(mb_artist_id) ON DELETE CASCADE,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		PRIMARY KEY (user_id, mb_artist_id)
	);

	CREATE INDEX IF NOT EXISTS idx_user_artists_artist ON user_artists(mb_artist_id);

	CREATE TABLE IF NOT EXISTS agent_tokens (
		token_hash TEXT PRIMARY KEY,
		user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		expires_at TIMESTAMPTZ NOT NULL,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);

	CREATE TABLE IF NOT EXISTS fs_agents (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		label TEXT NOT NULL DEFAULT '',
		public_key TEXT NOT NULL,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		last_seen_at TIMESTAMPTZ
	);

	CREATE INDEX IF NOT EXISTS idx_fs_agents_user_id ON fs_agents(user_id);

	CREATE TABLE IF NOT EXISTS fs_release_group_states (
		user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		agent_id TEXT NOT NULL REFERENCES fs_agents(id) ON DELETE CASCADE,
		mb_release_group_id TEXT NOT NULL,
		state TEXT NOT NULL,
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		PRIMARY KEY (user_id, agent_id, mb_release_group_id)
	);

	CREATE INDEX IF NOT EXISTS idx_frgs_user_id ON fs_release_group_states(user_id);
	`

	_, err := pool.Exec(ctx, schema)
	return err
}
