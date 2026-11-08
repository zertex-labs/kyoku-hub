package handler

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/database/queries"
	mw "github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/middleware"
	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/model"
	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/settings"
)

// SettingsHandler owns the options a user chooses: how files are tagged and what
// counts as a complete file.
type SettingsHandler struct {
	q      *queries.Queries
	store  *settings.Store
	logger *zap.Logger
}

func NewSettingsHandler(q *queries.Queries, store *settings.Store, logger *zap.Logger) *SettingsHandler {
	return &SettingsHandler{q: q, store: store, logger: logger}
}

func (h *SettingsHandler) Register(g *echo.Group) {
	g.GET("", h.Get, mw.AuthRequired(h.q))
	g.PUT("", h.Put, mw.AuthRequired(h.q))
}

func (h *SettingsHandler) Get(c echo.Context) error {
	user := c.Get("user").(*model.User)
	return c.JSON(http.StatusOK, h.store.Get(c.Request().Context(), user.ID))
}

// Put takes a partial body: a single checkbox moving sends one field, and the
// store merges it over what is already there. A whole-form submit sends all of
// them.
func (h *SettingsHandler) Put(c echo.Context) error {
	user := c.Get("user").(*model.User)
	ctx := c.Request().Context()

	current := h.store.Get(ctx, user.ID)

	var body struct {
		RequireMusicBrainzTags *bool   `json:"requireMusicBrainzTags"`
		EmbedCover             *bool   `json:"embedCover"`
		WriteCoverFile         *bool   `json:"writeCoverFile"`
		PreferProviderGenre    *bool   `json:"preferProviderGenre"`
		AlbumPath              *string `json:"albumPath"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid body")
	}

	if body.RequireMusicBrainzTags != nil {
		current.RequireMusicBrainzTags = *body.RequireMusicBrainzTags
	}
	if body.EmbedCover != nil {
		current.EmbedCover = *body.EmbedCover
	}
	if body.WriteCoverFile != nil {
		current.WriteCoverFile = *body.WriteCoverFile
	}
	if body.PreferProviderGenre != nil {
		current.PreferProviderGenre = *body.PreferProviderGenre
	}
	if body.AlbumPath != nil {
		current.AlbumPath = strings.TrimSpace(*body.AlbumPath)
		if current.AlbumPath == "" {
			current.AlbumPath = settings.DefaultAlbumPath
		}
		if !strings.Contains(current.AlbumPath, "{album}") {
			return echo.NewHTTPError(http.StatusBadRequest, "the album path must contain {album}")
		}
	}
	if err := current.Validate(); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	if err := h.store.Put(ctx, user.ID, current); err != nil {
		h.logger.Error("failed to save settings", zap.Error(err))
		return echo.NewHTTPError(http.StatusInternalServerError, "could not save settings")
	}

	// Re-deriving the states is cheap and it is the point of the option: turning
	// it on should tell you which albums fall short, without waiting for the
	// files to be touched again.
	return c.JSON(http.StatusOK, current)
}
