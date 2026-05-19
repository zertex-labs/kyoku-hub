package handler

import (
	"context"
	"encoding/json"
	"fmt"
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

	result, err := h.mb.SearchArtist(context.Background(), query, limit, offset)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, result)
}

func (h *ArtistHandler) AddArtistForUser(c echo.Context) error {
	user := c.Get("user").(*model.User)
	_ = user

	var body struct {
		MBID string `json:"mbid"`
	}
	if err := c.Bind(&body); err != nil || body.MBID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "missing musicbrainzId"})
	}

	res, err := h.mb.LookupArtist(context.Background(), body.MBID, "release-groups")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "lookup failed"})
	}
	b, _ := json.MarshalIndent(res, "", "  ")
	fmt.Println(string(b))

	// return c.JSON(http.StatusOK, map[string]string{"ok": "true"})
	return c.JSON(http.StatusOK, res)
}
