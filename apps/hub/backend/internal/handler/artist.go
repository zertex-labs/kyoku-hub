package handler

import (
	"net/http"
	"strconv"

	mw "github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/middleware"
	musicbrainz "github.com/zertex-labs/kyoku-hub/packages/musicbrainz-client"

	"github.com/labstack/echo/v4"
	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/config"
	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/database/queries"
	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/model"
)

type ArtistHandler struct {
	cfg     *config.Config
	queries *queries.Queries
	mb      *musicbrainz.Client
}

func NewArtistHandler(cfg *config.Config, q *queries.Queries, mb *musicbrainz.Client) *ArtistHandler {
	return &ArtistHandler{cfg: cfg, queries: q, mb: mb}
}

func (h *ArtistHandler) Register(g *echo.Group) {
	g.GET("/search", h.SearchForArtist)
	g.POST("/add", h.AddArtistForUser, mw.AuthRequired(h.queries))
	g.POST("/remove", h.RemoveArtistForUser, mw.AuthRequired(h.queries))
	g.GET("/list", h.ListUserArtists, mw.AuthRequired(h.queries))
}

func (h *ArtistHandler) SearchForArtist(c echo.Context) error {
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
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, result)
}

func (h *ArtistHandler) AddArtistForUser(c echo.Context) error {
	user := c.Get("user").(*model.User)

	var body struct {
		MBID string `json:"mbid"`
	}
	if err := c.Bind(&body); err != nil || body.MBID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "missing mbid"})
	}

	// Lookup from MusicBrainz to validate MBID and get canonical data
	res, err := h.mb.LookupArtist(c.Request().Context(), body.MBID, "")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid mbid or lookup failed"})
	}

	artist, err := h.queries.UpsertArtist(c.Request().Context(), res.ID, res.Name, res.Disambiguation)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to upsert artist"})
	}

	if err := h.queries.LinkUserArtist(c.Request().Context(), user.ID, res.ID); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to link artist"})
	}

	// TODO: send subscribe to brainz via WS

	return c.JSON(http.StatusOK, artist)
}

func (h *ArtistHandler) RemoveArtistForUser(c echo.Context) error {
	user := c.Get("user").(*model.User)

	var body struct {
		MBID string `json:"mbid"`
	}
	if err := c.Bind(&body); err != nil || body.MBID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "missing mbid"})
	}

	ctx := c.Request().Context()

	if err := h.queries.UnlinkUserArtist(ctx, user.ID, body.MBID); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to unlink artist"})
	}

	// If no other users track this artist, deactivate it
	count, err := h.queries.CountArtistSubscribers(ctx, body.MBID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to check subscribers"})
	}

	if count == 0 {
		if err := h.queries.DeactivateArtist(ctx, body.MBID); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to deactivate artist"})
		}
		// TODO: send unsubscribe to brainz via WS
	}

	return c.JSON(http.StatusOK, map[string]string{"ok": "true"})
}

func (h *ArtistHandler) ListUserArtists(c echo.Context) error {
	user := c.Get("user").(*model.User)

	artists, err := h.queries.GetUserArtists(c.Request().Context(), user.ID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to list artists"})
	}

	if artists == nil {
		artists = []model.Artist{}
	}

	return c.JSON(http.StatusOK, artists)
}
