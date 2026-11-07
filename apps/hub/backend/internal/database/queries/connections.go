package queries

import (
	"context"

	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/model"
)

func (q *Queries) ListProviderCredentials(ctx context.Context, userID string) ([]model.ProviderCredential, error) {
	rows, err := q.Pool.Query(ctx,
		`SELECT user_id, provider, credentials_json, created_at, updated_at
		 FROM provider_credentials WHERE user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []model.ProviderCredential
	for rows.Next() {
		var c model.ProviderCredential
		if err := rows.Scan(&c.UserID, &c.Provider, &c.CredentialsJSON, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, c)
	}
	return result, rows.Err()
}

func (q *Queries) GetProviderCredential(ctx context.Context, userID, provider string) (*model.ProviderCredential, error) {
	var c model.ProviderCredential
	err := q.Pool.QueryRow(ctx,
		`SELECT user_id, provider, credentials_json, created_at, updated_at
		 FROM provider_credentials WHERE user_id = $1 AND provider = $2`,
		userID, provider,
	).Scan(&c.UserID, &c.Provider, &c.CredentialsJSON, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (q *Queries) UpsertProviderCredential(ctx context.Context, userID, provider, credentialsJSON string) error {
	_, err := q.Pool.Exec(ctx,
		`INSERT INTO provider_credentials (user_id, provider, credentials_json)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (user_id, provider) DO UPDATE
		   SET credentials_json = EXCLUDED.credentials_json, updated_at = NOW()`,
		userID, provider, credentialsJSON,
	)
	return err
}

func (q *Queries) DeleteProviderCredential(ctx context.Context, userID, provider string) error {
	_, err := q.Pool.Exec(ctx,
		`DELETE FROM provider_credentials WHERE user_id = $1 AND provider = $2`,
		userID, provider,
	)
	return err
}
