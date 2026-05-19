package worker

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/zertex-labs/kyoku-hub/apps/brainz/internal/cache"
	"github.com/zertex-labs/kyoku-hub/apps/brainz/internal/config"
)

type Worker struct {
	db     *pgxpool.Pool
	cache  *cache.Cache
	cfg    *config.Config
	logger *zap.Logger
}

func New(db *pgxpool.Pool, cache *cache.Cache, cfg *config.Config, logger *zap.Logger) *Worker {
	return &Worker{db: db, cache: cache, cfg: cfg, logger: logger}
}

// Run starts the worker loop. Blocks until ctx is cancelled.
func (w *Worker) Run(ctx context.Context) {
	w.logger.Info("worker started",
		zap.Duration("poll_interval", w.cfg.WorkerPollInterval),
		zap.Int("force_threshold", w.cfg.ForceUpdateThreshold),
	)

	ticker := time.NewTicker(w.cfg.WorkerPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("worker shutting down")
			return
		case <-ticker.C:
			w.poll(ctx)
		}
	}
}

func (w *Worker) poll(ctx context.Context) {
	// TODO: pick next artist from priority queue, fetch from MB, store, invalidate cache, push to hubs
	w.logger.Debug("polling for work")
}
