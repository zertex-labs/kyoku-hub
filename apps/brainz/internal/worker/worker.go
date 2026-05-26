package worker

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	musicbrainz "github.com/zertex-labs/kyoku-hub/packages/musicbrainz-client"

	"github.com/zertex-labs/kyoku-hub/apps/brainz/internal/cache"
	"github.com/zertex-labs/kyoku-hub/apps/brainz/internal/config"
	"github.com/zertex-labs/kyoku-hub/apps/brainz/internal/hub"
	brainzpb "github.com/zertex-labs/kyoku-hub/proto/gen/brainz/v1"
)

type Worker struct {
	db     *pgxpool.Pool
	cache  *cache.Cache
	cfg    *config.Config
	mb     *musicbrainz.Client
	hub    *hub.Hub
	logger *zap.Logger
}

func New(db *pgxpool.Pool, cache *cache.Cache, cfg *config.Config, mb *musicbrainz.Client, hub *hub.Hub, logger *zap.Logger) *Worker {
	return &Worker{db: db, cache: cache, cfg: cfg, mb: mb, hub: hub, logger: logger}
}

// Run starts the worker loop. Blocks until ctx is cancelled.
func (w *Worker) Run(ctx context.Context) {
	w.logger.Info("worker started",
		zap.Duration("poll_interval", w.cfg.WorkerPollInterval),
	)

	ticker := time.NewTicker(w.cfg.WorkerPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("worker shutting down")
			return
		case <-ticker.C:
			w.Poll(ctx)
		}
	}
}

func (w *Worker) Poll(ctx context.Context) {
	// Pick next artist from priority queue:
	// 1. Artists with pending requests (most requests first)
	// 2. Artists never fetched (oldest created_at first)
	// 3. Stale artists past minimum refetch interval (oldest last_fetched_at first)
	var mbArtistID string
	var totalRequests int

	refetchSeconds := int(w.cfg.WorkerMinimumRefetchInterval.Seconds())

	err := w.db.QueryRow(ctx, `
		SELECT a.mb_artist_id, COALESCE(SUM(r.request_count), 0)::int AS total_requests
		FROM artists a
		LEFT JOIN artist_requests r ON r.mb_artist_id = a.mb_artist_id
		GROUP BY a.mb_artist_id, a.last_fetched_at, a.created_at
		HAVING
			COALESCE(SUM(r.request_count), 0) > 0
			OR a.last_fetched_at IS NULL
			OR a.last_fetched_at < NOW() - $1 * INTERVAL '1 second'
		ORDER BY
			CASE
				WHEN COALESCE(SUM(r.request_count), 0) > 0 THEN 0
				WHEN a.last_fetched_at IS NULL THEN 1
				ELSE 2
			END,
			COALESCE(SUM(r.request_count), 0) DESC,
			a.created_at ASC,
			a.last_fetched_at ASC
		LIMIT 1
	`, refetchSeconds).Scan(&mbArtistID, &totalRequests)

	if err != nil {
		w.logger.Debug("worker poll: no artists in queue")
		return
	}

	w.logger.Info("syncing artist", zap.String("mb_artist_id", mbArtistID), zap.Int("requests", totalRequests))
	w.SyncArtist(ctx, mbArtistID)
}

func (w *Worker) SyncArtist(ctx context.Context, mbArtistID string) {
	// 1. Fetch artist from MusicBrainz
	artist, err := w.mb.LookupArtist(ctx, mbArtistID)
	if err != nil {
		w.logger.Error("failed to lookup artist", zap.String("mb_artist_id", mbArtistID), zap.Error(err))
		return
	}

	// 2. Fetch all release groups (paginated)
	releaseGroups, err := w.mb.BrowseReleaseGroups(ctx, mbArtistID)
	if err != nil {
		w.logger.Error("failed to browse release groups", zap.String("mb_artist_id", mbArtistID), zap.Error(err))
		return
	}

	// 3. Fetch Last.fm image(TODO: last.fm seems to have deprecated their image values, and now depend on user provided images which aren't in the api) + description
	imageURL := ""
	description := ""
	info, err := musicbrainz.FetchLastFM(ctx, w.cfg.LastFMAPIKey, mbArtistID)
	if err != nil {
		w.logger.Warn("lastfm fetch failed", zap.String("mb_artist_id", mbArtistID), zap.Error(err))
	} else {
		imageURL = info.ImageURL
		description = info.Description
		w.logger.Info("lastfm response",
			zap.String("mb_artist_id", mbArtistID),
			zap.String("image_url", info.ImageURL),
			zap.Int("description_len", len(info.Description)),
		)
	}

	// 3. Determine fallback release group (first album, or first anything)
	fallbackRGID := ""
	for _, rg := range releaseGroups {
		if rg.PrimaryType == "Album" {
			fallbackRGID = rg.ID
			break
		}
	}
	if fallbackRGID == "" && len(releaseGroups) > 0 {
		fallbackRGID = releaseGroups[0].ID
	}

	// If no Last.fm image, use CAA release group cover as fallback
	if imageURL == "" && fallbackRGID != "" {
		imageURL = fmt.Sprintf("https://coverartarchive.org/release-group/%s/front-500", fallbackRGID)
	}

	// 4. Upsert artist in DB
	_, err = w.db.Exec(ctx, `
		INSERT INTO artists (mb_artist_id, name, sort_name, disambiguation, type, country, image_url, description, fallback_release_group_id, last_fetched_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW())
		ON CONFLICT (mb_artist_id) DO UPDATE SET
			name = EXCLUDED.name,
			sort_name = EXCLUDED.sort_name,
			disambiguation = EXCLUDED.disambiguation,
			type = EXCLUDED.type,
			country = EXCLUDED.country,
			image_url = EXCLUDED.image_url,
			description = EXCLUDED.description,
			fallback_release_group_id = EXCLUDED.fallback_release_group_id,
			last_fetched_at = NOW()
	`, mbArtistID, artist.Name, artist.SortName, artist.Disambiguation, artist.Type, artist.Country, imageURL, description, fallbackRGID)
	if err != nil {
		w.logger.Error("failed to upsert artist", zap.Error(err))
		return
	}

	// 5. Upsert release groups
	for _, rg := range releaseGroups {
		secondaryTypes := strings.Join(rg.SecondaryTypes, ", ")
		_, err = w.db.Exec(ctx, `
			INSERT INTO release_groups (mb_release_group_id, mb_artist_id, title, primary_type, secondary_types, disambiguation, first_release_date)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (mb_release_group_id) DO UPDATE SET
				title = EXCLUDED.title,
				primary_type = EXCLUDED.primary_type,
				secondary_types = EXCLUDED.secondary_types,
				disambiguation = EXCLUDED.disambiguation,
				first_release_date = EXCLUDED.first_release_date
		`, rg.ID, mbArtistID, rg.Title, rg.PrimaryType, secondaryTypes, rg.Disambiguation, rg.FirstReleaseDate)
		if err != nil {
			w.logger.Error("failed to upsert release group", zap.String("rg_id", rg.ID), zap.Error(err))
		}
	}

	// 6. Reset request counts
	_, err = w.db.Exec(ctx, `DELETE FROM artist_requests WHERE mb_artist_id = $1`, mbArtistID)
	if err != nil {
		w.logger.Error("failed to reset request counts", zap.Error(err))
	}

	// 7. Invalidate cache
	w.cache.Delete(fmt.Sprintf("artist:%s", mbArtistID))
	w.cache.Delete(fmt.Sprintf("release-groups:%s", mbArtistID))

	// 8. Push update to subscribed hubs via WS
	releaseGroupsPb := make([]*brainzpb.ReleaseGroup, 0, len(releaseGroups))
	for _, rg := range releaseGroups {
		releaseGroupsPb = append(releaseGroupsPb, &brainzpb.ReleaseGroup{
			MbReleaseGroupId: rg.ID,
			Title:            rg.Title,
			PrimaryType:      rg.PrimaryType,
			FirstReleaseDate: rg.FirstReleaseDate,
		})
	}

	w.hub.Broadcast(mbArtistID, &brainzpb.Envelope{
		Payload: &brainzpb.Envelope_ArtistUpdated{
			ArtistUpdated: &brainzpb.ArtistUpdated{
				MbArtistId:     mbArtistID,
				Name:           artist.Name,
				Disambiguation: artist.Disambiguation,
				ImageUrl:       imageURL,
				Description:    description,
				Type:           artist.Type,
				Country:        artist.Country,
				ReleaseGroups:  releaseGroupsPb,
			},
		},
	})

	w.logger.Info("artist synced",
		zap.String("mb_artist_id", mbArtistID),
		zap.String("name", artist.Name),
		zap.String("image_url", imageURL),
		zap.Int("release_groups", len(releaseGroups)),
	)
}
