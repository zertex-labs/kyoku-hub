package queries

import (
	"crypto/rand"
	"encoding/hex"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Queries struct {
	Pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Queries {
	return &Queries{Pool: pool}
}

func generateID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}
