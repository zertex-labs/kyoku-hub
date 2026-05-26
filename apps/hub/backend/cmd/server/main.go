package main

import (
	"context"
	"net/http"

	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
	echomw "github.com/labstack/echo/v4/middleware"
	"go.uber.org/zap"

	musicbrainz "github.com/zertex-labs/kyoku-hub/packages/musicbrainz-client"
	brainzpb "github.com/zertex-labs/kyoku-hub/proto/gen/brainz/v1"

	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/brainzclient"
	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/config"
	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/database"
	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/database/queries"
	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/handler"
	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/logger"
	mw "github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/middleware"
)

func main() {
	_ = godotenv.Load()

	log := logger.New()
	defer log.Sync()

	cfg, err := config.Load()
	if err != nil {
		log.Fatal("failed to load config", zap.Error(err))
	}

	ctx := context.Background()

	pool, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal("failed to connect to database", zap.Error(err))
	}
	defer pool.Close()

	if err := database.Migrate(ctx, pool); err != nil {
		log.Fatal("migration failed", zap.Error(err))
	}

	q := queries.New(pool)
	mb := musicbrainz.NewClient("Kyoku/0.1.0 (https://github.com/zertex-labs/kyoku-hub)")

	e := echo.New()
	e.HideBanner = true
	e.HidePort = true

	e.Use(mw.ZapLogger(log))
	e.Use(echomw.Recover())
	e.Use(echomw.CORSWithConfig(echomw.CORSConfig{
		AllowOrigins:     []string{cfg.FrontendURL},
		AllowMethods:     []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodOptions},
		AllowHeaders:     []string{echo.HeaderContentType, echo.HeaderAuthorization},
		AllowCredentials: true,
	}))

	e.GET("/health", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})

	authHandler := handler.NewAuthHandler(cfg, q)
	authHandler.Register(e.Group("/api/auth"))

	// Brainz WS client
	bc := brainzclient.New(cfg.BrainzWSURL, cfg.InstanceID, q, log)
	bc.OnArtistUpdated = func(ctx context.Context, msg *brainzpb.ArtistUpdated) {
		if err := q.UpdateArtistFromBrainz(ctx, msg.MbArtistId, msg.Name, msg.Disambiguation, msg.ImageUrl, msg.Description, len(msg.ReleaseGroups)); err != nil {
			log.Error("failed to update artist from brainz", zap.String("mb_artist_id", msg.MbArtistId), zap.Error(err))
		}
	}
	bc.OnStatusChanged = func(ctx context.Context, msg *brainzpb.ArtistStatusChanged) {
		if err := q.UpdateArtistStatus(ctx, msg.MbArtistId, msg.Status); err != nil {
			log.Error("failed to update artist status", zap.String("mb_artist_id", msg.MbArtistId), zap.Error(err))
		}
	}
	go bc.Run(ctx)

	artistHandler := handler.NewArtistHandler(cfg, q, mb, bc)
	artistHandler.Register(e.Group("/api/artist"))

	log.Info("starting hub backend", zap.String("port", cfg.Port))
	if err := e.Start(":" + cfg.Port); err != nil && err != http.ErrServerClosed {
		log.Fatal("server error", zap.Error(err))
	}
}
