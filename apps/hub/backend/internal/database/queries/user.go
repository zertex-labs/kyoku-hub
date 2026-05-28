package queries

import (
	"context"

	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/model"
)

func (q *Queries) FindOrCreateUser(ctx context.Context, mbUserID, username string) (*model.User, error) {
	user := &model.User{}
	err := q.Pool.QueryRow(ctx,
		`INSERT INTO users (id, username, mb_user_id)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (mb_user_id) DO UPDATE SET username = EXCLUDED.username
		 RETURNING id, username, mb_user_id, created_at`,
		generateID(), username, mbUserID,
	).Scan(&user.ID, &user.Username, &user.MBUserID, &user.CreatedAt)
	if err != nil {
		return nil, err
	}
	return user, nil
}
