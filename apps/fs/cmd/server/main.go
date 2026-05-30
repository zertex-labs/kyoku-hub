package main

import (
	"net/http"

	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
	echomw "github.com/labstack/echo/v4/middleware"
	"go.uber.org/zap"

	"github.com/zertex-labs/kyoku-hub/apps/fs/internal/config"
	logger "github.com/zertex-labs/kyoku-hub/apps/fs/internal/logger"
	mw "github.com/zertex-labs/kyoku-hub/apps/fs/internal/middleware"
	"github.com/zertex-labs/kyoku-hub/apps/fs/internal/handler"
)

func main() {
	_ = godotenv.Load()

	log := logger.New()
	defer log.Sync()

	cfg, err := config.Load()
	if err != nil {
		log.Fatal("failed to load config", zap.Error(err))
	}

	// TODO: sqlite db to store "modified at" states, so we don't have to search everything on every boot
	// and this will most likely be running on user pcs, so there will be alot of "fresh starts"

	e := echo.New()
	e.HideBanner = true
	e.HidePort = true

	e.Use(mw.ZapLogger(log))
	e.Use(echomw.Recover())

	e.GET("/health", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})

	fsHandler := handler.NewFsHandler(log, cfg)
	fsHandler.RegisterRoutes(e.Group("/api/fs"))

	log.Info("starting hub backend", zap.String("port", cfg.Port))
	if err := e.Start(":" + cfg.Port); err != nil && err != http.ErrServerClosed {
		log.Fatal("server error", zap.Error(err))
	}
}
