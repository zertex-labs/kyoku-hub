package queries

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/model"
)

const (
	agentTokenExpiry = 10 * time.Minute
	maxAgentsPerUser = 3
)

// CreateAgentToken generates a random token, stores its SHA-256 hash, and returns the raw token.
func (q *Queries) CreateAgentToken(ctx context.Context, userID string) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	token := hex.EncodeToString(raw)
	hash := hashToken(token)

	_, err := q.Pool.Exec(ctx,
		`INSERT INTO agent_tokens (token_hash, user_id, expires_at)
		 VALUES ($1, $2, NOW() + $3::interval)`,
		hash, userID, fmt.Sprintf("%d seconds", int(agentTokenExpiry.Seconds())),
	)
	if err != nil {
		return "", fmt.Errorf("insert agent token: %w", err)
	}

	return token, nil
}

// ConsumeAgentToken validates and deletes a token, returning the owning user ID.
func (q *Queries) ConsumeAgentToken(ctx context.Context, token string) (string, error) {
	hash := hashToken(token)

	var userID string
	err := q.Pool.QueryRow(ctx,
		`DELETE FROM agent_tokens
		 WHERE token_hash = $1 AND expires_at > NOW()
		 RETURNING user_id`,
		hash,
	).Scan(&userID)
	if err != nil {
		return "", fmt.Errorf("invalid or expired token: %w", err)
	}

	return userID, nil
}

// CreateFsAgent registers a new fs agent for a user. Enforces max agent limit.
func (q *Queries) CreateFsAgent(ctx context.Context, userID, label, publicKey string) (*model.FsAgent, error) {
	// Check agent limit
	var count int
	err := q.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM fs_agents WHERE user_id = $1`, userID,
	).Scan(&count)
	if err != nil {
		return nil, fmt.Errorf("count agents: %w", err)
	}
	if count >= maxAgentsPerUser {
		return nil, fmt.Errorf("agent limit reached (max %d)", maxAgentsPerUser)
	}

	id := generateID()
	agent := &model.FsAgent{
		ID:        id,
		UserID:    userID,
		Label:     label,
		PublicKey: publicKey,
		CreatedAt: time.Now(),
	}

	_, err = q.Pool.Exec(ctx,
		`INSERT INTO fs_agents (id, user_id, label, public_key)
		 VALUES ($1, $2, $3, $4)`,
		agent.ID, agent.UserID, agent.Label, agent.PublicKey,
	)
	if err != nil {
		return nil, fmt.Errorf("insert fs agent: %w", err)
	}

	return agent, nil
}

// GetFsAgent looks up an agent by ID.
func (q *Queries) GetFsAgent(ctx context.Context, agentID string) (*model.FsAgent, error) {
	agent := &model.FsAgent{}
	err := q.Pool.QueryRow(ctx,
		`SELECT id, user_id, label, public_key, created_at
		 FROM fs_agents WHERE id = $1`,
		agentID,
	).Scan(&agent.ID, &agent.UserID, &agent.Label, &agent.PublicKey, &agent.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("agent not found: %w", err)
	}
	return agent, nil
}

// ListFsAgents returns all agents for a user.
func (q *Queries) ListFsAgents(ctx context.Context, userID string) ([]model.FsAgent, error) {
	rows, err := q.Pool.Query(ctx,
		`SELECT id, user_id, label, public_key, created_at
		 FROM fs_agents WHERE user_id = $1 ORDER BY created_at`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list agents: %w", err)
	}
	defer rows.Close()

	var agents []model.FsAgent
	for rows.Next() {
		var a model.FsAgent
		if err := rows.Scan(&a.ID, &a.UserID, &a.Label, &a.PublicKey, &a.CreatedAt); err != nil {
			return nil, err
		}
		agents = append(agents, a)
	}
	return agents, nil
}

// DeleteFsAgent removes an agent. Only the owning user can delete.
func (q *Queries) DeleteFsAgent(ctx context.Context, agentID, userID string) error {
	tag, err := q.Pool.Exec(ctx,
		`DELETE FROM fs_agents WHERE id = $1 AND user_id = $2`,
		agentID, userID,
	)
	if err != nil {
		return fmt.Errorf("delete agent: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("agent not found or not owned by user")
	}
	return nil
}

func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}
