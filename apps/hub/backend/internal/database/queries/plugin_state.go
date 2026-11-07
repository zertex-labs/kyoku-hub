package queries

import (
	"context"
	"time"
)

// PluginState is the installed/enabled bookkeeping for one plugin.
type PluginState struct {
	Enabled   bool
	UpdatedAt time.Time
}

func (q *Queries) PluginStates(ctx context.Context) (map[string]PluginState, error) {
	rows, err := q.Pool.Query(ctx, `SELECT plugin_id, enabled, updated_at FROM plugin_state`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]PluginState{}
	for rows.Next() {
		var id string
		var s PluginState
		if err := rows.Scan(&id, &s.Enabled, &s.UpdatedAt); err != nil {
			return nil, err
		}
		out[id] = s
	}
	return out, rows.Err()
}

func (q *Queries) SetPluginEnabled(ctx context.Context, pluginID string, enabled bool) error {
	_, err := q.Pool.Exec(ctx, `
		INSERT INTO plugin_state (plugin_id, enabled, updated_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (plugin_id) DO UPDATE SET enabled = EXCLUDED.enabled, updated_at = NOW()`,
		pluginID, enabled)
	return err
}
