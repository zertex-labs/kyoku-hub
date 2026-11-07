package handler

import (
	"context"
	"net/http"
	"sort"
	"strings"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/database/queries"
	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/download"
	mw "github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/middleware"
	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/model"
	"github.com/zertex-labs/kyoku-hub/packages/downloader"
)

// DownloadHandler owns the download API: provider search for the release-group
// page, and the job endpoints the modal drives.
//
// The walk itself lives in internal/download.Manager — the handler only starts
// jobs, answers decisions and reads snapshots. Keeping the loop out of the
// request path is what makes a job survive the browser tab that started it.
type DownloadHandler struct {
	jobs    *download.Manager
	sources download.SourceResolver
	queries *queries.Queries
	logger  *zap.Logger
}

func NewDownloadHandler(
	jobs *download.Manager,
	sources download.SourceResolver,
	q *queries.Queries,
	log *zap.Logger,
) *DownloadHandler {
	return &DownloadHandler{jobs: jobs, sources: sources, queries: q, logger: log}
}

func (h *DownloadHandler) Register(g *echo.Group) {
	g.GET("/rg/:rgId/search", h.Search, mw.AuthRequired(h.queries))
	g.GET("/rg/:rgId/releases", h.Releases, mw.AuthRequired(h.queries))

	g.POST("/jobs", h.StartJob, mw.AuthRequired(h.queries))
	g.GET("/jobs", h.ListJobs, mw.AuthRequired(h.queries))
	g.GET("/jobs/:id", h.GetJob, mw.AuthRequired(h.queries))
	g.POST("/jobs/:id/decide", h.DecideJob, mw.AuthRequired(h.queries))
	g.POST("/jobs/:id/cancel", h.CancelJob, mw.AuthRequired(h.queries))
}

// Search fans out over the caller's sources and merges the results.
//
// `artist` and `title` describe the release group the user came from. Provider
// search is loose — "Ado DND" also returns adonä, Noir and You'll Never Get To
// Heaven — so results by another artist are dropped and the rest are ordered by
// how close the title is. A picker listing six different artists is not a
// catalogue, it is a search engine with no context.
func (h *DownloadHandler) Search(c echo.Context) error {
	user := c.Get("user").(*model.User)
	q := c.QueryParam("q")
	artist := strings.TrimSpace(c.QueryParam("artist"))
	title := strings.TrimSpace(c.QueryParam("title"))
	// Escape hatch: two spellings of one artist ("Ye" / "Kanye West") would
	// otherwise hide every result with no way back.
	includeOthers := c.QueryParam("others") == "1"
	if q == "" && artist != "" {
		q = strings.TrimSpace(artist + " " + title)
	}
	if q == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "missing query parameter 'q'"})
	}

	sources := h.sources(c.Request().Context(), user.ID)
	if len(sources) == 0 {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "no download sources configured"})
	}

	ctx := c.Request().Context()
	type result struct {
		items []downloader.SearchResult
		err   error
	}
	ch := make(chan result, len(sources))
	for _, src := range sources {
		go func(s downloader.Source) {
			items, err := s.SearchAlbums(ctx, q)
			ch <- result{items, err}
		}(src)
	}

	all := make([]downloader.SearchResult, 0, 20)
	for range sources {
		r := <-ch
		if r.err != nil {
			h.logger.Warn("provider search error", zap.String("q", q), zap.Error(r.err))
			continue
		}
		all = append(all, r.items...)
	}

	if artist != "" && !includeOthers {
		all = rankResults(all, artist, title)
	}

	if len(all) == 0 && artist != "" && !includeOthers {
		all = artistFallback(ctx, sources, artist, title, h.logger)
	}

	if all == nil {
		all = []downloader.SearchResult{}
	}
	return c.JSON(http.StatusOK, all)
}

// artistFallback walks each source's discography for the artist itself.
//
// Needed because a catalogue keeps the licensed title, not the one on the cover:
// Deezer files 三毒史 as "triviṣa-itihāsa" by "Sheena Ringo", so a search for the
// Japanese pair returns an empty set for an album that is very much there.
//
// These results are deliberately NOT re-filtered by artist name: the source
// matched the artist on its own terms, and comparing scripts lexically would
// discard exactly what we came for.
func artistFallback(
	ctx context.Context,
	sources map[string]downloader.Source,
	artist, title string,
	log *zap.Logger,
) []downloader.SearchResult {
	out := make([]downloader.SearchResult, 0, 20)
	names := make([]string, 0, len(sources))
	for name := range sources {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		src := sources[name]
		as, ok := src.(downloader.ArtistSource)
		if !ok {
			continue
		}
		items, err := as.ArtistAlbums(ctx, artist, title)
		if err != nil {
			log.Warn("artist fallback failed", zap.String("artist", artist), zap.Error(err))
			continue
		}
		out = append(out, items...)
	}
	return out
}

// Releases lists the MusicBrainz editions of a release group, so the user can
// pick the one they mean instead of leaving the resolver to guess from the track
// count — which cannot tell seven 13-track editions apart.
func (h *DownloadHandler) Releases(c echo.Context) error {
	items, err := h.jobs.MBReleases(c.Request().Context(), c.Param("rgId"))
	if err != nil {
		// Empty is a fine answer here: the page just offers no choice.
		h.logger.Warn("release list unavailable", zap.Error(err))
		return c.JSON(http.StatusOK, []download.MBRelease{})
	}
	return c.JSON(http.StatusOK, items)
}

type startJobRequest struct {
	RGID    string             `json:"rg_id"`
	Album   download.AlbumView `json:"album"`
	AgentID string             `json:"agent_id"`
	Mode    string             `json:"mode"`
	Chain   []string           `json:"chain"`

	PreferredProvider string `json:"provider"`
	PreferredAlbumID  string `json:"album_id"`
}

// StartJob creates the job and returns its first snapshot. The walk runs in the
// background and streams over SSE (`download:job`).
func (h *DownloadHandler) StartJob(c echo.Context) error {
	user := c.Get("user").(*model.User)

	var req startJobRequest
	if err := c.Bind(&req); err != nil || req.RGID == "" || req.Album.Title == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "missing rg_id or album"})
	}

	job, err := h.jobs.Start(c.Request().Context(), user.ID, req.RGID, download.StartRequest{
		Album:   req.Album,
		AgentID: req.AgentID,
		Mode:    req.Mode,
		Chain:   req.Chain,

		PreferredProvider: req.PreferredProvider,
		PreferredAlbumID:  req.PreferredAlbumID,
	})
	if err != nil {
		status := http.StatusBadRequest
		if err.Error() == "no download sources configured" {
			status = http.StatusServiceUnavailable
		}
		return c.JSON(status, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusAccepted, job)
}

func (h *DownloadHandler) ListJobs(c echo.Context) error {
	user := c.Get("user").(*model.User)
	return c.JSON(http.StatusOK, h.jobs.List(user.ID))
}

func (h *DownloadHandler) GetJob(c echo.Context) error {
	user := c.Get("user").(*model.User)
	job, ok := h.jobs.Get(user.ID, c.Param("id"))
	if !ok {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "job not found"})
	}
	return c.JSON(http.StatusOK, job.Snapshot())
}

func (h *DownloadHandler) DecideJob(c echo.Context) error {
	user := c.Get("user").(*model.User)

	var req struct {
		Choice string `json:"choice"`
	}
	if err := c.Bind(&req); err != nil || req.Choice == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "missing choice"})
	}
	if err := h.jobs.Decide(user.ID, c.Param("id"), req.Choice); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	job, ok := h.jobs.Get(user.ID, c.Param("id"))
	if !ok {
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	}
	return c.JSON(http.StatusOK, job.Snapshot())
}

func (h *DownloadHandler) CancelJob(c echo.Context) error {
	user := c.Get("user").(*model.User)
	if err := h.jobs.Cancel(user.ID, c.Param("id")); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "cancelled"})
}

// rankResults drops results by another artist and orders the rest: exact title
// first, then the ones that only add a deluxe/remaster suffix, then everything
// else. Version markers (remix, instrumental, sped up) sort below the album.
func rankResults(items []downloader.SearchResult, artist, title string) []downloader.SearchResult {
	// No reference artist means no basis to drop anything: a caller that cannot
	// tell us who the album is by should still get every result.
	if artist == "" {
		return items
	}

	kept := make([]downloader.SearchResult, 0, len(items))
	for _, it := range items {
		if downloader.ArtistMatches(artist, it.Artist) {
			kept = append(kept, it)
		}
	}

	sort.SliceStable(kept, func(i, j int) bool {
		ri, rj := scoreResult(kept[i], title), scoreResult(kept[j], title)
		if ri != rj {
			return ri > rj
		}
		return kept[i].Title < kept[j].Title
	})
	return kept
}

func scoreResult(it downloader.SearchResult, title string) int {
	if title == "" {
		return 0
	}
	score := downloader.TitleRank(title, it.Title)
	if downloader.NoisyRelease(title, it.Title) {
		score -= 40
	}
	return score
}
