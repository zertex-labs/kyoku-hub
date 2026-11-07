package queries

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// GetPluginConfig returns the encrypted configuration blob for a plugin, or nil
// when nothing has been saved yet.
func (q *Queries) GetPluginConfig(ctx context.Context, pluginID string) ([]byte, error) {
	var blob []byte
	err := q.Pool.QueryRow(ctx,
		`SELECT values FROM plugin_config WHERE plugin_id = $1`, pluginID,
	).Scan(&blob)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return blob, nil
}

func (q *Queries) SetPluginConfig(ctx context.Context, pluginID string, blob []byte) error {
	_, err := q.Pool.Exec(ctx, `
		INSERT INTO plugin_config (plugin_id, values, updated_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (plugin_id) DO UPDATE SET values = EXCLUDED.values, updated_at = NOW()`,
		pluginID, blob)
	return err
}
