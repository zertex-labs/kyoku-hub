package handler

import (
	"context"
	"net/http"
	"path/filepath"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/database/queries"
	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/fsagent"
	mw "github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/middleware"
	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/model"
	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/sse"
	"github.com/zertex-labs/kyoku-hub/packages/downloader"
)

type DownloadHandler struct {
	sse          *sse.Manager
	providers    map[string]downloader.Provider
	downloadDir  string
	queries      *queries.Queries
	agentManager *fsagent.Manager
	logger       *zap.Logger
}

func NewDownloadHandler(
	sseManager *sse.Manager,
	providers map[string]downloader.Provider,
	downloadDir string,
	q *queries.Queries,
	am *fsagent.Manager,
	log *zap.Logger,
) *DownloadHandler {
	return &DownloadHandler{
		sse:          sseManager,
		providers:    providers,
		downloadDir:  downloadDir,
		queries:      q,
		agentManager: am,
		logger:       log,
	}
}

func (h *DownloadHandler) Register(g *echo.Group) {
	g.GET("/rg/:rgId/search", h.SearchProviders, mw.AuthRequired(h.queries))
	g.POST("/rg/:rgId", h.DownloadReleaseGroup, mw.AuthRequired(h.queries))
}

func (h *DownloadHandler) SearchProviders(c echo.Context) error {
	if len(h.providers) == 0 {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "no download providers configured"})
	}
	q := c.QueryParam("q")
	if q == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "missing query parameter 'q'"})
	}

	type providerResult struct {
		results []downloader.SearchResult
		err     error
	}
	ch := make(chan providerResult, len(h.providers))
	for _, p := range h.providers {
		go func(p downloader.Provider) {
			res, err := p.SearchAlbum(q)
			ch <- providerResult{res, err}
		}(p)
	}

	var all []downloader.SearchResult
	for range h.providers {
		r := <-ch
		if r.err != nil {
			h.logger.Warn("provider search error", zap.String("q", q), zap.Error(r.err))
			continue
		}
		all = append(all, r.results...)
	}
	h.logger.Info("search complete", zap.String("q", q), zap.Int("results", len(all)))

	if all == nil {
		all = []downloader.SearchResult{}
	}
	return c.JSON(http.StatusOK, all)
}

type downloadRGRequest struct {
	Provider string `json:"provider"`
	AlbumID  string `json:"album_id"`
	AgentID  string `json:"agent_id,omitempty"`
}

func (h *DownloadHandler) DownloadReleaseGroup(c echo.Context) error {
	rgID := c.Param("rgId")
	user := c.Get("user").(*model.User)

	var req downloadRGRequest
	if err := c.Bind(&req); err != nil || req.Provider == "" || req.AlbumID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "missing provider or album_id"})
	}

	p, ok := h.providers[req.Provider]
	if !ok {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "provider not configured: " + req.Provider})
	}

	go h.runDownload(rgID, p, req.AlbumID, user.ID, req.AgentID)
	return c.JSON(http.StatusAccepted, map[string]string{"status": "queued"})
}

func (h *DownloadHandler) runDownload(rgID string, p downloader.Provider, albumID, userID, agentID string) {
	destDir := filepath.Join(h.downloadDir, rgID, albumID)

	broadcast := func(phase string, track, total int) {
		h.sse.Broadcast("download:progress", map[string]interface{}{
			"rg_id": rgID,
			"phase": phase,
			"track": track,
			"total": total,
		})
	}

	err := p.DownloadAlbum(context.Background(), albumID, destDir, func(phase string, n, total int) {
		broadcast(phase, n, total)
	})
	if err != nil {
		h.logger.Error("download failed", zap.String("rg_id", rgID), zap.String("album_id", albumID), zap.Error(err))
		h.sse.Broadcast("download:progress", map[string]interface{}{
			"rg_id": rgID,
			"phase": "error",
			"error": err.Error(),
		})
		return
	}

	h.logger.Info("download complete", zap.String("rg_id", rgID), zap.String("agent_id", agentID))
	if agentID != "" {
		h.agentManager.RequestSyncAgent(userID, agentID, false)
	} else {
		h.agentManager.RequestSync(userID, false)
	}
}
