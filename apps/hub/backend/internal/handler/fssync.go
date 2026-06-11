package handler

import (
	"net/http"

	"github.com/labstack/echo/v4"

	mw "github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/middleware"
	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/model"

	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/database/queries"
	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/fsagent"
	"github.com/zertex-labs/kyoku-hub/packages/validation"
)

type FsSyncHandler struct {
	queries      *queries.Queries
	agentManager *fsagent.Manager
}

func NewFsSyncHandler(q *queries.Queries, am *fsagent.Manager) *FsSyncHandler {
	return &FsSyncHandler{queries: q, agentManager: am}
}

func (h *FsSyncHandler) Register(g *echo.Group) {
	g.POST("/sync", h.TriggerSync, mw.AuthRequired(h.queries))
	g.GET("/issues", h.GetIssues, mw.AuthRequired(h.queries))
	g.GET("/untagged", h.GetUntagged, mw.AuthRequired(h.queries))
}

func (h *FsSyncHandler) TriggerSync(c echo.Context) error {
	user := c.Get("user").(*model.User)

	var body struct {
		Full bool `json:"full"`
	}
	_ = c.Bind(&body)

	sent := h.agentManager.RequestSync(user.ID, body.Full)
	if sent == 0 {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"agents_synced": 0,
			"message":       "no agents connected",
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"agents_synced": sent,
	})
}

// GetIssues returns per-track tag issues for one release group, fetched on
// demand (data lives in memory and is refreshed on every agent sync).
func (h *FsSyncHandler) GetIssues(c echo.Context) error {
	user := c.Get("user").(*model.User)

	rgID := c.QueryParam("rg")
	if !validation.IsMBID(rgID) {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid or missing rg parameter"})
	}

	issues := h.agentManager.IssuesFor(user.ID, rgID)
	if issues == nil {
		issues = []fsagent.TrackIssue{}
	}
	return c.JSON(http.StatusOK, issues)
}

// GetUntagged returns scanned tracks that lack MusicBrainz IDs.
func (h *FsSyncHandler) GetUntagged(c echo.Context) error {
	user := c.Get("user").(*model.User)

	untagged := h.agentManager.UntaggedFor(user.ID)
	if untagged == nil {
		untagged = []fsagent.UntaggedTrack{}
	}
	return c.JSON(http.StatusOK, untagged)
}
