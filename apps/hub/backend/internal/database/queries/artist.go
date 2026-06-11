package queries

import (
	"context"

	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/model"
)

func (q *Queries) UpsertArtist(ctx context.Context, mbArtistID, name, disambiguation string) (*model.Artist, error) {
	query := `
		INSERT INTO artists (mb_artist_id, name, disambiguation, status, active)
		VALUES ($1, $2, $3, 'queued', true)
		ON CONFLICT (mb_artist_id) DO UPDATE SET active = true
		RETURNING mb_artist_id, name, disambiguation, image_url, description, release_group_count, status, created_at, synced_at`

	var a model.Artist
	err := q.Pool.QueryRow(ctx, query, mbArtistID, name, disambiguation).Scan(
		&a.MBArtistID, &a.Name, &a.Disambiguation, &a.ImageURL, &a.Description, &a.ReleaseGroupCount, &a.Status, &a.CreatedAt, &a.SyncedAt,
	)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (q *Queries) LinkUserArtist(ctx context.Context, userID, mbArtistID string) error {
	query := `INSERT INTO user_artists (user_id, mb_artist_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`
	_, err := q.Pool.Exec(ctx, query, userID, mbArtistID)
	return err
}

func (q *Queries) UnlinkUserArtist(ctx context.Context, userID, mbArtistID string) error {
	query := `DELETE FROM user_artists WHERE user_id = $1 AND mb_artist_id = $2`
	_, err := q.Pool.Exec(ctx, query, userID, mbArtistID)
	return err
}

func (q *Queries) CountArtistSubscribers(ctx context.Context, mbArtistID string) (int, error) {
	var count int
	err := q.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM user_artists WHERE mb_artist_id = $1`, mbArtistID).Scan(&count)
	return count, err
}

func (q *Queries) GetArtistName(ctx context.Context, mbArtistID string) (string, error) {
	var name string
	err := q.Pool.QueryRow(ctx, `SELECT name FROM artists WHERE mb_artist_id = $1`, mbArtistID).Scan(&name)
	return name, err
}

func (q *Queries) DeactivateArtist(ctx context.Context, mbArtistID string) error {
	_, err := q.Pool.Exec(ctx, `UPDATE artists SET active = false WHERE mb_artist_id = $1`, mbArtistID)
	return err
}

func (q *Queries) GetUserArtists(ctx context.Context, userID string) ([]model.Artist, error) {
	query := `
		SELECT a.mb_artist_id, a.name, a.disambiguation, a.image_url, a.description, a.release_group_count, a.status, a.created_at, a.synced_at
		FROM artists a
		JOIN user_artists ua ON ua.mb_artist_id = a.mb_artist_id
		WHERE ua.user_id = $1
		ORDER BY ua.created_at DESC`

	rows, err := q.Pool.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	artists := []model.Artist{}
	for rows.Next() {
		var a model.Artist
		if err := rows.Scan(&a.MBArtistID, &a.Name, &a.Disambiguation, &a.ImageURL, &a.Description, &a.ReleaseGroupCount, &a.Status, &a.CreatedAt, &a.SyncedAt); err != nil {
			return nil, err
		}
		artists = append(artists, a)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return artists, nil
}

func (q *Queries) UpdateArtistFromBrainz(ctx context.Context, mbArtistID, name, disambiguation, imageURL, description string, releaseGroupCount int) error {
	_, err := q.Pool.Exec(ctx, `
		UPDATE artists SET
			name = $2,
			disambiguation = $3,
			image_url = $4,
			description = $5,
			release_group_count = $6,
			status = 'synced',
			synced_at = NOW()
		WHERE mb_artist_id = $1`,
		mbArtistID, name, disambiguation, imageURL, description, releaseGroupCount,
	)
	return err
}

func (q *Queries) UpdateArtistStatus(ctx context.Context, mbArtistID, status string) error {
	_, err := q.Pool.Exec(ctx, `UPDATE artists SET status = $2 WHERE mb_artist_id = $1`, mbArtistID, status)
	return err
}

func (q *Queries) CountRecentUserAdds(ctx context.Context, userID string) (int, error) {
	var count int
	err := q.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM user_artists WHERE user_id = $1 AND created_at > NOW() - INTERVAL '1 hour'`,
		userID,
	).Scan(&count)
	return count, err
}

func (q *Queries) DeleteArtistAndLinks(ctx context.Context, mbArtistID string) error {
	// Delete user_artists links first (FK), then the artist row
	if _, err := q.Pool.Exec(ctx, `DELETE FROM user_artists WHERE mb_artist_id = $1`, mbArtistID); err != nil {
		return err
	}
	_, err := q.Pool.Exec(ctx, `DELETE FROM artists WHERE mb_artist_id = $1`, mbArtistID)
	return err
}
