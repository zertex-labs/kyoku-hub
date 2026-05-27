package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"github.com/zertex-labs/kyoku-hub/apps/brainz/internal/cache"
	"github.com/zertex-labs/kyoku-hub/apps/brainz/internal/model"
	musicbrainz "github.com/zertex-labs/kyoku-hub/packages/musicbrainz-client"
)

type ArtistHandler struct {
	db     *pgxpool.Pool
	cache  *cache.Cache
	mb     *musicbrainz.Client
	logger *zap.Logger
}

func NewArtistHandler(db *pgxpool.Pool, cache *cache.Cache, mb *musicbrainz.Client, logger *zap.Logger) *ArtistHandler {
	return &ArtistHandler{db: db, cache: cache, mb: mb, logger: logger}
}

func (h *ArtistHandler) RegisterRoutes(g *echo.Group) {
	g.GET("/artist/search", h.SearchArtist)
	g.GET("/artist/:id", h.GetArtist)
	g.GET("/artist/:id/release-groups", h.GetReleaseGroups)
	g.GET("/artist/:id/release-groups/count", h.GetReleaseGroupCount)
	g.GET("/artist/:id/release-groups/:rgid/releases", h.GetReleases)
}

func (h *ArtistHandler) SearchArtist(c echo.Context) error {
	query := c.QueryParam("q")
	if query == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "missing query parameter 'q'"})
	}

	limit := 10
	offset := 0
	if v := c.QueryParam("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}
	if v := c.QueryParam("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}

	result, err := h.mb.SearchArtist(c.Request().Context(), query, limit, offset)
	if err != nil {
		h.logger.Error("failed to search artists", zap.String("query", query), zap.Error(err))
		return c.JSON(http.StatusBadGateway, map[string]string{"error": "musicbrainz search failed"})
	}

	return c.JSON(http.StatusOK, result)
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
		`SELECT mb_artist_id, name, sort_name, disambiguation, type, country, image_url, description, last_fetched_at, created_at
		 FROM artists WHERE mb_artist_id = $1`, id,
	).Scan(&artist.MBArtistID, &artist.Name, &artist.SortName, &artist.Disambiguation, &artist.Type, &artist.Country, &artist.ImageURL, &artist.Description, &artist.LastFetchedAt, &artist.CreatedAt)
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

	cacheKey := cache.GetKeyFor(cache.ReleaseGroupsLookup{MBID: id})
	if cached, found := h.cache.Get(cacheKey); found {
		return c.JSONBlob(http.StatusOK, cached)
	}

	rows, err := h.db.Query(context.Background(),
		`SELECT mb_release_group_id, title, primary_type, secondary_types, disambiguation, first_release_date
		 FROM release_groups WHERE mb_artist_id = $1
		 ORDER BY first_release_date DESC`, id,
	)
	if err != nil {
		h.logger.Error("failed to query release groups", zap.Error(err))
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal error"})
	}
	defer rows.Close()

	var releaseGroups []model.ReleaseGroup
	for rows.Next() {
		var rg model.ReleaseGroup
		if err := rows.Scan(&rg.MBReleaseGroupID, &rg.Title, &rg.PrimaryType, &rg.SecondaryTypes, &rg.Disambiguation, &rg.FirstReleaseDate); err != nil {
			h.logger.Error("failed to scan release group", zap.Error(err))
			continue
		}
		rg.MBArtistID = id
		releaseGroups = append(releaseGroups, rg)
	}

	data, _ := json.Marshal(releaseGroups)
	h.cache.Set(cacheKey, data)

	return c.JSONBlob(http.StatusOK, data)
}

func (h *ArtistHandler) GetReleaseGroupCount(c echo.Context) error {
	id := c.Param("id")
	var count int
	err := h.db.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM release_groups WHERE mb_artist_id = $1`, id,
	).Scan(&count)
	if err != nil {
		h.logger.Error("failed to count release groups", zap.Error(err))
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal error"})
	}
	return c.JSON(http.StatusOK, map[string]int{"count": count})
}

func (h *ArtistHandler) GetReleases(c echo.Context) error {
	rgid := c.Param("rgid")
	h.logger.Debug("get releases", zap.String("release_group_id", rgid))
	return c.JSON(http.StatusNotImplemented, map[string]string{"error": "not implemented"})
}
