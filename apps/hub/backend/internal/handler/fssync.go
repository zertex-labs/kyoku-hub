package handler

import (
	"net/http"

	"github.com/labstack/echo/v4"

	mw "github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/middleware"
	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/model"

	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/database/queries"
	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/fsagent"
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
