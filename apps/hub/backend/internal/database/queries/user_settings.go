package queries

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
)

// UserSettings is one user's stored preferences, as JSON so adding an option is
// a struct change and not a migration.
func (q *Queries) UserSettings(ctx context.Context, userID string) (json.RawMessage, error) {
	var raw []byte
	err := q.Pool.QueryRow(ctx,
		`SELECT data FROM user_settings WHERE user_id = $1`, userID).Scan(&raw)
	if err != nil {
		// No row is not an error: a user who has never opened settings has the
		// defaults.
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return raw, nil
}

func (q *Queries) SaveUserSettings(ctx context.Context, userID string, data []byte) error {
	_, err := q.Pool.Exec(ctx, `
		INSERT INTO user_settings (user_id, data, updated_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (user_id) DO UPDATE SET data = EXCLUDED.data, updated_at = NOW()`,
		userID, data)
	return err
}
