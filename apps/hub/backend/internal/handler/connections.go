package handler

import (
	"encoding/json"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/database/queries"
	mw "github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/middleware"
	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/model"
	"github.com/zertex-labs/kyoku-hub/packages/downloader/deezer"
)

type ConnectionsHandler struct {
	queries *queries.Queries
}

func NewConnectionsHandler(q *queries.Queries) *ConnectionsHandler {
	return &ConnectionsHandler{queries: q}
}

func (h *ConnectionsHandler) Register(g *echo.Group) {
	g.GET("", h.List, mw.AuthRequired(h.queries))
	g.PUT("/:provider", h.Upsert, mw.AuthRequired(h.queries))
	g.DELETE("/:provider", h.Delete, mw.AuthRequired(h.queries))
}

type connectionStatus struct {
	Provider  string `json:"provider"`
	Connected bool   `json:"connected"`
}

func (h *ConnectionsHandler) List(c echo.Context) error {
	user := c.Get("user").(*model.User)

	creds, err := h.queries.ListProviderCredentials(c.Request().Context(), user.ID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to list connections"})
	}

	connected := make(map[string]bool)
	for _, cr := range creds {
		connected[cr.Provider] = true
	}

	// Deezer only: Tidal is parked (no tagging, no token rotation), and listing a
	// source that cannot be set up is worse than not listing it.
	statuses := []connectionStatus{
		{Provider: "deezer", Connected: connected["deezer"]},
	}

	return c.JSON(http.StatusOK, statuses)
}

type upsertDeezerRequest struct {
	ARL string `json:"arl"`
}

func (h *ConnectionsHandler) Upsert(c echo.Context) error {
	user := c.Get("user").(*model.User)
	provider := c.Param("provider")

	switch provider {
	case "deezer":
		var req upsertDeezerRequest
		if err := c.Bind(&req); err != nil || req.ARL == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "missing arl"})
		}

		auth, err := deezer.InitSession(req.ARL)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid ARL: " + err.Error()})
		}

		credsJSON, err := json.Marshal(auth)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to encode credentials"})
		}

		if err := h.queries.UpsertProviderCredential(c.Request().Context(), user.ID, "deezer", string(credsJSON)); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to save credentials"})
		}

		return c.JSON(http.StatusOK, connectionStatus{Provider: "deezer", Connected: true})

	default:
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "unsupported provider: " + provider})
	}
}

func (h *ConnectionsHandler) Delete(c echo.Context) error {
	user := c.Get("user").(*model.User)
	provider := c.Param("provider")

	if err := h.queries.DeleteProviderCredential(c.Request().Context(), user.ID, provider); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to delete credentials"})
	}

	return c.JSON(http.StatusOK, connectionStatus{Provider: provider, Connected: false})
}
