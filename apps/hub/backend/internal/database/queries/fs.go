package queries

import (
	"context"
)

// UpsertReleaseGroupStates persists computed FS states for a user's release groups.
func (q *Queries) UpsertReleaseGroupStates(ctx context.Context, userID string, states map[string]string) error {
	for rgID, state := range states {
		_, err := q.Pool.Exec(ctx, `
			INSERT INTO user_release_group_states (user_id, mb_release_group_id, state, updated_at)
			VALUES ($1, $2, $3, NOW())
			ON CONFLICT (user_id, mb_release_group_id) DO UPDATE SET state = $3, updated_at = NOW()`,
			userID, rgID, state,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

// GetUserReleaseGroupStates returns a map of mb_release_group_id -> state for a user.
func (q *Queries) GetUserReleaseGroupStates(ctx context.Context, userID string) (map[string]string, error) {
	rows, err := q.Pool.Query(ctx, `
		SELECT mb_release_group_id, state
		FROM user_release_group_states
		WHERE user_id = $1`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	states := make(map[string]string)
	for rows.Next() {
		var rgID, state string
		if err := rows.Scan(&rgID, &state); err != nil {
			return nil, err
		}
		states[rgID] = state
	}
	return states, rows.Err()
}
