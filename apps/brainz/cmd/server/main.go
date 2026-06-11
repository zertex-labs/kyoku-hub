package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"github.com/zertex-labs/kyoku-hub/packages/logger"

	"github.com/zertex-labs/kyoku-hub/apps/brainz/internal/cache"
	"github.com/zertex-labs/kyoku-hub/apps/brainz/internal/config"
	"github.com/zertex-labs/kyoku-hub/apps/brainz/internal/database"
	"github.com/zertex-labs/kyoku-hub/apps/brainz/internal/handler"
	"github.com/zertex-labs/kyoku-hub/apps/brainz/internal/hub"
	"github.com/zertex-labs/kyoku-hub/apps/brainz/internal/worker"
	musicbrainz "github.com/zertex-labs/kyoku-hub/packages/musicbrainz-client"
)

func main() {
	_ = godotenv.Load()

	log := logger.New("brainz")
	defer log.Sync()

	cfg, err := config.Load()
	if err != nil {
		log.Fatal("configuration error", zap.Error(err))
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Database
	db, err := database.Connect(ctx, cfg.DBUrl, log)
	if err != nil {
		log.Fatal("failed to connect to database", zap.Error(err))
	}
	defer db.Close()

	if err := database.Migrate(ctx, db, log); err != nil {
		log.Fatal("failed to migrate database", zap.Error(err))
	}

	// Cache
	c, err := cache.New(cfg, log)
	if err != nil {
		log.Fatal("failed to init cache", zap.Error(err))
	}
	defer c.Close()

	// MusicBrainz client(TODO: dynamically update version)
	mb := musicbrainz.NewClient("Kyoku/0.1 (https://github.com/zertex-labs/kyoku-hub)")

	// Hub (manages connected hub instances)
	h := hub.New(log)

	// Worker
	w := worker.New(db, c, cfg, mb, h, log)
	go w.Run(ctx)

	// Echo server
	e := echo.New()
	e.HideBanner = true

	api := e.Group("/api")
	artistHandler := handler.NewArtistHandler(db, c, mb, log)
	artistHandler.RegisterRoutes(api)

	wsHandler := handler.NewWSHandler(h, db, log)
	wsHandler.RegisterRoutes(e)

	go func() {
		<-ctx.Done()
		log.Info("shutting down server")
		e.Close()
	}()

	log.Info("starting brainz server", zap.String("port", cfg.Port))
	if err := e.Start(fmt.Sprintf(":%s", cfg.Port)); err != nil {
		log.Info("server stopped", zap.Error(err))
	}
}
