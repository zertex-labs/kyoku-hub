package handler

import (
	"context"
	"encoding/json"

	"go.uber.org/zap"

	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/database/queries"
	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/download"
	"github.com/zertex-labs/kyoku-hub/packages/downloader"
	"github.com/zertex-labs/kyoku-hub/packages/downloader/deezer"
)

// SourceResolver builds the caller's configured sources from their stored
// credentials. One entry per provider they have connected — the credentials
// table is the registry of what this user can actually use.
//
// Tidal is parked (no tagging, no token rotation), so only Deezer is built here;
// adding a case is the whole cost of bringing a source back.
func SourceResolver(q *queries.Queries, logger *zap.Logger) download.SourceResolver {
	return func(ctx context.Context, userID string) map[string]downloader.Source {
		out := make(map[string]downloader.Source)
		creds, err := q.ListProviderCredentials(ctx, userID)
		if err != nil {
			logger.Warn("could not list provider credentials", zap.Error(err), zap.String("user", userID))
			return out
		}
		for _, c := range creds {
			switch c.Provider {
			case "deezer":
				var auth deezer.Auth
				if err := json.Unmarshal([]byte(c.CredentialsJSON), &auth); err != nil {
					logger.Warn("stored deezer credentials are unreadable", zap.Error(err))
					continue
				}
				out["deezer"] = deezer.New(auth, downloader.DefaultConfig())
			}
		}
		return out
	}
}

// JobStore persists job snapshots through the queries layer.
type JobStore struct {
	q *queries.Queries
}

func NewJobStore(q *queries.Queries) JobStore { return JobStore{q: q} }

func (s JobStore) Save(ctx context.Context, userID string, job download.JobView) error {
	payload, err := json.Marshal(job)
	if err != nil {
		return err
	}
	return s.q.SaveDownloadJob(ctx, userID, job.ID, job.RGID, job.Phase, payload)
}
