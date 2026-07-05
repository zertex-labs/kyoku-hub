package handler

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	mw "github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/middleware"
	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/database/queries"
	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/fsagent"
	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/model"
	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/sse"
	"github.com/zertex-labs/kyoku-hub/packages/downloader"
	"github.com/zertex-labs/kyoku-hub/packages/downloader/tidal"
)

type DownloadHandler struct {
	sse          *sse.Manager
	tidal        *tidal.Tidal
	downloadDir  string
	queries      *queries.Queries
	agentManager *fsagent.Manager
	logger       *zap.Logger
}

func NewDownloadHandler(sseManager *sse.Manager, t *tidal.Tidal, downloadDir string, q *queries.Queries, am *fsagent.Manager, logger *zap.Logger) *DownloadHandler {
	return &DownloadHandler{
		sse:          sseManager,
		tidal:        t,
		downloadDir:  downloadDir,
		queries:      q,
		agentManager: am,
		logger:       logger,
	}
}

func (h *DownloadHandler) Register(g *echo.Group) {
	g.POST("/rg/:rgId", h.DownloadReleaseGroup, mw.AuthRequired(h.queries))
}

type downloadRGRequest struct {
	Title      string `json:"title"`
	ArtistName string `json:"artist_name"`
}

type downloadProgressEvent struct {
	RgID        string  `json:"rg_id"`
	TrackIndex  int     `json:"track_index"`
	TotalTracks int     `json:"total_tracks"`
	Phase       string  `json:"phase"`
	Progress    float64 `json:"progress"`
	Error       string  `json:"error,omitempty"`
}

func (h *DownloadHandler) DownloadReleaseGroup(c echo.Context) error {
	if h.tidal == nil || !h.tidal.IsAuthenticated() {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "no download provider configured"})
	}

	rgID := c.Param("rgId")
	user := c.Get("user").(*model.User)

	var req downloadRGRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if req.Title == "" || req.ArtistName == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "title and artist_name required"})
	}

	go h.runDownload(user.ID, rgID, req)
	return c.JSON(http.StatusAccepted, map[string]string{"status": "queued"})
}

func (h *DownloadHandler) sendProgress(userID, rgID, phase string, trackIdx, total int, progress float64, errMsg string) {
	h.sse.SendToUser(userID, "download:progress", downloadProgressEvent{
		RgID:        rgID,
		TrackIndex:  trackIdx,
		TotalTracks: total,
		Phase:       phase,
		Progress:    progress,
		Error:       errMsg,
	})
}

func (h *DownloadHandler) runDownload(userID, rgID string, req downloadRGRequest) {
	h.sendProgress(userID, rgID, "searching", 0, 0, 0, "")

	results, err := h.tidal.SearchAlbum(req.ArtistName + " " + req.Title)
	if err != nil || len(results) == 0 {
		h.logger.Warn("tidal search failed", zap.String("query", req.ArtistName+" "+req.Title), zap.Error(err))
		h.sendProgress(userID, rgID, "error", 0, 0, 0, "album not found on Tidal")
		return
	}

	album, err := h.tidal.GetAlbum(results[0].ID)
	if err != nil {
		h.logger.Error("tidal GetAlbum failed", zap.String("id", results[0].ID), zap.Error(err))
		h.sendProgress(userID, rgID, "error", 0, 0, 0, "failed to fetch album metadata")
		return
	}

	total := len(album.Tracks)
	h.sendProgress(userID, rgID, "downloading", 0, total, 0, "")

	albumDir := filepath.Join(
		h.downloadDir,
		downloader.SanitizeFilename(req.ArtistName),
		downloader.SanitizeFilename(req.Title),
	)
	if err := os.MkdirAll(albumDir, 0o755); err != nil {
		h.logger.Error("failed to create download dir", zap.String("path", albumDir), zap.Error(err))
		h.sendProgress(userID, rgID, "error", 0, total, 0, "failed to create output directory")
		return
	}

	pad := 2
	if total >= 100 {
		pad = 3
	}

	for i, track := range album.Tracks {
		h.sendProgress(userID, rgID, "downloading", i+1, total, 0, "")

		streamInfo, err := h.tidal.GetStreamURL(track.ID)
		if err != nil {
			h.logger.Warn("failed to get stream URL", zap.String("track_id", track.ID), zap.String("title", track.Title), zap.Error(err))
			h.sendProgress(userID, rgID, "downloading", i+1, total, 0, fmt.Sprintf("track %d: %s", i+1, err.Error()))
			continue
		}

		ext := downloader.MimeToExtension(streamInfo.MimeType)
		filename := fmt.Sprintf("%0*d - %s.%s", pad, track.TrackNumber, downloader.SanitizeFilename(track.Title), ext)
		filePath := filepath.Join(albumDir, filename)

		if err := downloadSegments(filePath, streamInfo.URLs); err != nil {
			h.logger.Warn("failed to download track", zap.String("title", track.Title), zap.Error(err))
			continue
		}

		h.sendProgress(userID, rgID, "downloading", i+1, total, 100, "")
	}

	h.sendProgress(userID, rgID, "complete", total, total, 100, "")
	h.logger.Info("download complete", zap.String("rg_id", rgID), zap.String("dir", albumDir))

	if sent := h.agentManager.RequestSync(userID, false); sent == 0 {
		h.logger.Warn("download complete but no fs agents connected to trigger rescan", zap.String("user_id", userID))
	}
}

func downloadSegments(path string, urls []string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	client := &http.Client{}
	for _, u := range urls {
		resp, err := client.Get(u)
		if err != nil {
			return fmt.Errorf("GET %s: %w", u, err)
		}
		_, cpErr := io.Copy(f, resp.Body)
		resp.Body.Close()
		if cpErr != nil {
			return fmt.Errorf("write segment: %w", cpErr)
		}
	}
	return nil
}
