package plugins

import (
	"context"
	"fmt"

	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/database/queries"
)

// Store persists per-plugin configuration. Scope is the install, not the user: a
// plugin acts on the library, and every download on this hub shares it.
type Store struct {
	q   *queries.Queries
	box *SecretBox
}

func NewStore(q *queries.Queries, box *SecretBox) *Store {
	return &Store{q: q, box: box}
}

// Values decrypts the saved configuration for a plugin.
func (s *Store) Values(ctx context.Context, pluginID string) (map[string]any, error) {
	blob, err := s.q.GetPluginConfig(ctx, pluginID)
	if err != nil {
		return nil, err
	}
	return s.box.Open(blob)
}

// PluginStates implements StateStore.
func (s *Store) PluginStates(ctx context.Context) (map[string]State, error) {
	rows, err := s.q.PluginStates(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]State, len(rows))
	for id, r := range rows {
		out[id] = State{Enabled: r.Enabled, UpdatedAt: r.UpdatedAt}
	}
	return out, nil
}

// SetPluginEnabled implements StateStore.
func (s *Store) SetPluginEnabled(ctx context.Context, pluginID string, enabled bool) error {
	return s.q.SetPluginEnabled(ctx, pluginID, enabled)
}

// Save encrypts and stores the configuration for a plugin.
func (s *Store) Save(ctx context.Context, pluginID string, values map[string]any) error {
	blob, err := s.box.Seal(values)
	if err != nil {
		return fmt.Errorf("encrypt config: %w", err)
	}
	return s.q.SetPluginConfig(ctx, pluginID, blob)
}
