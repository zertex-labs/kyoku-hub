package handler

import (
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"github.com/zertex-labs/kyoku-hub/apps/brainz/internal/cache"
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
	// TODO: check cache, then DB
	id := c.Param("id")
	h.logger.Debug("get artist", zap.String("id", id))
	return c.JSON(http.StatusNotImplemented, map[string]string{"error": "not implemented"})
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
