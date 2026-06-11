package queries

import (
	"context"
	"fmt"
)

// statePriority ranks fs states when merging across agents: if any agent has
// a perfect copy of a release group, the user effectively has it.
var statePriority = map[string]int{
	"PERFECT": 2,
	"BAD":     1,
}

// ReplaceAgentReleaseGroupStates atomically replaces all persisted states for
// one agent with the result of its latest scan. An empty map clears the
// agent's states (e.g. all files removed).
func (q *Queries) ReplaceAgentReleaseGroupStates(ctx context.Context, userID, agentID string, states map[string]string) error {
	tx, err := q.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx,
		`DELETE FROM fs_release_group_states WHERE user_id = $1 AND agent_id = $2`,
		userID, agentID,
	); err != nil {
		return fmt.Errorf("clear agent states: %w", err)
	}

	for rgID, state := range states {
		if _, err := tx.Exec(ctx, `
			INSERT INTO fs_release_group_states (user_id, agent_id, mb_release_group_id, state, updated_at)
			VALUES ($1, $2, $3, $4, NOW())`,
			userID, agentID, rgID, state,
		); err != nil {
			return fmt.Errorf("insert state: %w", err)
		}
	}

	return tx.Commit(ctx)
}

// GetUserReleaseGroupStates returns mb_release_group_id -> state for a user,
// merged across all their agents (best state wins).
func (q *Queries) GetUserReleaseGroupStates(ctx context.Context, userID string) (map[string]string, error) {
	rows, err := q.Pool.Query(ctx, `
		SELECT mb_release_group_id, state
		FROM fs_release_group_states
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
		if existing, ok := states[rgID]; !ok || statePriority[state] > statePriority[existing] {
			states[rgID] = state
		}
	}
	return states, rows.Err()
}
