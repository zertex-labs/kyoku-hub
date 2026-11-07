package queries

import (
	"context"
	"encoding/json"

	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/model"
)

// SaveDownloadJob upserts the JSON snapshot of a job. The in-memory registry is
// authoritative while a job runs; this is the record that survives a restart and
// the thing the UI can list after the fact.
func (q *Queries) SaveDownloadJob(ctx context.Context, userID, id, rgID, status string, payload json.RawMessage) error {
	_, err := q.Pool.Exec(ctx,
		`INSERT INTO download_jobs (id, user_id, rg_id, status, payload)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (id) DO UPDATE
		   SET status = EXCLUDED.status,
		       payload = EXCLUDED.payload,
		       updated_at = NOW()`,
		id, userID, rgID, status, payload)
	return err
}

// ListDownloadJobs returns the caller's recent jobs, newest first.
func (q *Queries) ListDownloadJobs(ctx context.Context, userID string, limit int) ([]model.DownloadJob, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	rows, err := q.Pool.Query(ctx,
		`SELECT id, user_id, rg_id, status, payload, created_at, updated_at
		 FROM download_jobs WHERE user_id = $1
		 ORDER BY created_at DESC LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.DownloadJob
	for rows.Next() {
		var j model.DownloadJob
		if err := rows.Scan(&j.ID, &j.UserID, &j.RGID, &j.Status, &j.Payload, &j.CreatedAt, &j.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}
