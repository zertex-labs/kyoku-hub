package queries

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// LatestJobPayload returns the stored snapshot of the newest job for a release
// group, or nil when the user never downloaded it. Used to find the directory an
// album was published to, for re-tagging it later.
func (q *Queries) LatestJobPayload(ctx context.Context, userID, rgID string) ([]byte, error) {
	var payload []byte
	// Only jobs that actually published somewhere: an inspect or a re-tag for the
	// same album is newer but has no directory, and picking it would answer
	// "never downloaded" for an album sitting right there.
	err := q.Pool.QueryRow(ctx,
		`SELECT payload FROM download_jobs
		  WHERE user_id = $1 AND rg_id = $2
		    AND COALESCE(payload->>'dir', '') <> ''
		  ORDER BY created_at DESC LIMIT 1`,
		userID, rgID).Scan(&payload)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return payload, nil
}
