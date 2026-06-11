package handler

import (
	"encoding/json"
	"net/http"
	"net/url"

	mw "github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/middleware"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/brainzclient"
	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/config"
	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/database/queries"
	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/fsagent"
	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/model"
	"github.com/zertex-labs/kyoku-hub/packages/validation"
)

const maxAddsPerHour = 20

type ArtistHandler struct {
	cfg          *config.Config
	queries      *queries.Queries
	bc           *brainzclient.Client
	brainzAPI    *brainzclient.API
	agentManager *fsagent.Manager
	logger       *zap.Logger
}

func NewArtistHandler(cfg *config.Config, q *queries.Queries, bc *brainzclient.Client, brainzAPI *brainzclient.API, am *fsagent.Manager, logger *zap.Logger) *ArtistHandler {
	return &ArtistHandler{cfg: cfg, queries: q, bc: bc, brainzAPI: brainzAPI, agentManager: am, logger: logger}
}

func (h *ArtistHandler) Register(g *echo.Group) {
	g.GET("/search", h.SearchForArtist)
	g.POST("/add", h.AddArtistForUser, mw.AuthRequired(h.queries))
	g.POST("/remove", h.RemoveArtistForUser, mw.AuthRequired(h.queries))
	g.GET("/list", h.ListUserArtists, mw.AuthRequired(h.queries))
	g.GET("/:id", h.GetArtistDetail, mw.AuthRequired(h.queries))
	g.GET("/:id/release-groups", h.GetArtistReleaseGroups, mw.AuthRequired(h.queries))
}

func (h *ArtistHandler) SearchForArtist(c echo.Context) error {
	query := c.QueryParam("q")
	if query == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "missing query parameter 'q'"})
	}

	limit := c.QueryParam("limit")
	offset := c.QueryParam("offset")

	path := "/artist/search?q=" + url.QueryEscape(query)
	if limit != "" {
		path += "&limit=" + limit
	}
	if offset != "" {
		path += "&offset=" + offset
	}

	status, body, err := h.brainzAPI.ProxyGet(path)
	if err != nil {
		h.logger.Error("brainz search failed", zap.String("query", query), zap.Error(err))
		return c.JSON(http.StatusBadGateway, map[string]string{"error": "brainz unavailable"})
	}

	return c.JSONBlob(status, body)
}

func (h *ArtistHandler) AddArtistForUser(c echo.Context) error {
	user := c.Get("user").(*model.User)

	var body struct {
		MBID           string `json:"mbid"`
		Name           string `json:"name"`
		Disambiguation string `json:"disambiguation"`
	}
	if err := c.Bind(&body); err != nil || body.MBID == "" || body.Name == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "missing mbid or name"})
	}

	if !validation.IsMBID(body.MBID) {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid mbid format"})
	}

	ctx := c.Request().Context()

	// DB-backed rate limit: max adds per hour per user
	recentAdds, err := h.queries.CountRecentUserAdds(ctx, user.ID)
	if err != nil {
		h.logger.Error("failed to check rate limit", zap.Error(err))
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal error"})
	}
	if recentAdds >= maxAddsPerHour {
		return c.JSON(http.StatusTooManyRequests, map[string]string{"error": "too many artist adds, try again later"})
	}

	artist, err := h.queries.UpsertArtist(ctx, body.MBID, body.Name, body.Disambiguation)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to upsert artist"})
	}

	if err := h.queries.LinkUserArtist(ctx, user.ID, body.MBID); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to link artist"})
	}

	// Send subscribe to brainz via WS
	h.bc.Subscribe([]string{body.MBID})

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

	if !validation.IsMBID(body.MBID) {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid mbid format"})
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
		// Send unsubscribe to brainz via WS
		h.bc.Unsubscribe([]string{body.MBID})
	}

	return c.JSON(http.StatusOK, map[string]string{"ok": "true"})
}

func (h *ArtistHandler) ListUserArtists(c echo.Context) error {
	user := c.Get("user").(*model.User)

	artists, err := h.queries.GetUserArtists(c.Request().Context(), user.ID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to list artists"})
	}

	return c.JSON(http.StatusOK, artists)
}

func (h *ArtistHandler) GetArtistDetail(c echo.Context) error {
	id := c.Param("id")
	if !validation.IsMBID(id) {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid mbid format"})
	}

	status, body, err := h.brainzAPI.ProxyGet("/artist/" + id)
	if err != nil {
		h.logger.Error("brainz proxy failed", zap.String("path", "/artist/"+id), zap.Error(err))
		return c.JSON(http.StatusBadGateway, map[string]string{"error": "brainz unavailable"})
	}

	return c.JSONBlob(status, body)
}

func (h *ArtistHandler) GetArtistReleaseGroups(c echo.Context) error {
	id := c.Param("id")
	if !validation.IsMBID(id) {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid mbid format"})
	}

	status, body, err := h.brainzAPI.ProxyGet("/artist/" + id + "/release-groups")
	if err != nil {
		h.logger.Error("brainz proxy failed", zap.String("path", "/artist/"+id+"/release-groups"), zap.Error(err))
		return c.JSON(http.StatusBadGateway, map[string]string{"error": "brainz unavailable"})
	}

	if status != http.StatusOK {
		return c.JSONBlob(status, body)
	}

	user := c.Get("user").(*model.User)
	states, err := h.queries.GetUserReleaseGroupStates(c.Request().Context(), user.ID)
	if err != nil {
		h.logger.Error("failed to fetch rg states", zap.Error(err))
		// Non-fatal: return without states
		return c.JSONBlob(status, body)
	}

	var rgs []map[string]interface{}
	if err := json.Unmarshal(body, &rgs); err != nil {
		return c.JSONBlob(status, body)
	}

	// Artist name is needed to match untagged files (no MB IDs) against
	// release groups by name. Failure is non-fatal: matching is skipped.
	artistName, err := h.queries.GetArtistName(c.Request().Context(), id)
	if err != nil {
		artistName = ""
	}

	for i, rg := range rgs {
		rgID, _ := rg["mb_release_group_id"].(string)
		if s, ok := states[rgID]; ok {
			rgs[i]["fs_state"] = s
			continue
		}

		// Unknown to fs by MBID. If an untagged file matches by artist +
		// title, the user likely has it but the files lack MusicBrainz tags.
		title, _ := rg["title"].(string)
		if artistName != "" && h.agentManager.HasUntaggedMatch(user.ID, artistName, title) {
			rgs[i]["fs_state"] = string(model.RGStateMBMiss)
		} else {
			rgs[i]["fs_state"] = string(model.RGStateMissing)
		}
	}

	return c.JSON(http.StatusOK, rgs)
}
