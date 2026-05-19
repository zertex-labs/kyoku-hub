package queries

import (
	"context"
	"time"

	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/model"
)

func (q *Queries) CreateSession(ctx context.Context, userID string, duration time.Duration) (*model.Session, error) {
	session := &model.Session{
		ID:        generateID(),
		UserID:    userID,
		ExpiresAt: time.Now().Add(duration),
		CreatedAt: time.Now(),
	}

	_, err := q.Pool.Exec(ctx,
		`INSERT INTO sessions (id, user_id, expires_at, created_at) VALUES ($1, $2, $3, $4)`,
		session.ID, session.UserID, session.ExpiresAt, session.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return session, nil
}

func (q *Queries) GetSessionWithUser(ctx context.Context, sessionID string) (*model.User, error) {
	user := &model.User{}
	err := q.Pool.QueryRow(ctx,
		`SELECT u.id, u.username, u.mb_user_id, u.created_at
		 FROM sessions s
		 JOIN users u ON u.id = s.user_id
		 WHERE s.id = $1 AND s.expires_at > NOW()`,
		sessionID,
	).Scan(&user.ID, &user.Username, &user.MBUserID, &user.CreatedAt)
	if err != nil {
		return nil, err
	}
	return user, nil
}

func (q *Queries) DeleteSession(ctx context.Context, sessionID string) error {
	_, err := q.Pool.Exec(ctx, `DELETE FROM sessions WHERE id = $1`, sessionID)
	return err
}
