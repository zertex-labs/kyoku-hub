package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"github.com/zertex-labs/kyoku-hub/apps/brainz/internal/cache"
	"github.com/zertex-labs/kyoku-hub/apps/brainz/internal/model"
)

type ArtistHandler struct {
	db     *pgxpool.Pool
	cache  *cache.Cache
	logger *zap.Logger
}

func NewArtistHandler(db *pgxpool.Pool, cache *cache.Cache, logger *zap.Logger) *ArtistHandler {
	return &ArtistHandler{db: db, cache: cache, logger: logger}
}

func (h *ArtistHandler) RegisterRoutes(g *echo.Group) {
	g.GET("/artist/:id", h.GetArtist)
	g.GET("/artist/:id/release-groups", h.GetReleaseGroups)
	g.GET("/artist/:id/release-groups/:rgid/releases", h.GetReleases)
}

func (h *ArtistHandler) GetArtist(c echo.Context) error {
	id := c.Param("id")
	h.logger.Debug("get artist", zap.String("id", id))
	cacheKey := cache.GetKeyFor(cache.ArtistLookup{MBID: id})

	if cached, found := h.cache.Get(cacheKey); found {
		h.logger.Debug("found cached artist", zap.String("cacheKey", cacheKey))
		return c.JSONBlob(http.StatusOK, cached)
	}

	artist := &model.Artist{}
	err := h.db.QueryRow(context.Background(),
		`SELECT mb_artist_id, name, sort_name, disambiguation, type, country, last_fetched_at, created_at
		 FROM artists WHERE mb_artist_id = $1`, id,
	).Scan(&artist.MBArtistID, &artist.Name, &artist.SortName, &artist.Disambiguation, &artist.Type, &artist.Country, &artist.LastFetchedAt, &artist.CreatedAt)
	if err != nil {
		h.logger.Error("failed to get artist", zap.Error(err))
		return c.JSON(http.StatusNotFound, map[string]string{"error": "artist not found"})
	}

	data, _ := json.Marshal(artist)
	h.cache.Set(cacheKey, data)

	return c.JSONBlob(http.StatusOK, data)
}

func (h *ArtistHandler) GetReleaseGroups(c echo.Context) error {
	id := c.Param("id")
	h.logger.Debug("get release groups", zap.String("artist_id", id))
	return c.JSON(http.StatusNotImplemented, map[string]string{"error": "not implemented"})
}

func (h *ArtistHandler) GetReleases(c echo.Context) error {
	rgid := c.Param("rgid")
	h.logger.Debug("get releases", zap.String("release_group_id", rgid))
	return c.JSON(http.StatusNotImplemented, map[string]string{"error": "not implemented"})
}
