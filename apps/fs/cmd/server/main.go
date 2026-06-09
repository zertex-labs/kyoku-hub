package main

import (
	"context"
	"net/http"

	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
	echomw "github.com/labstack/echo/v4/middleware"
	"go.uber.org/zap"

	"github.com/zertex-labs/kyoku-hub/apps/fs/internal/auth"
	"github.com/zertex-labs/kyoku-hub/apps/fs/internal/config"
	"github.com/zertex-labs/kyoku-hub/apps/fs/internal/handler"
	"github.com/zertex-labs/kyoku-hub/apps/fs/internal/hubclient"
	logger "github.com/zertex-labs/kyoku-hub/packages/logger"
	mw "github.com/zertex-labs/kyoku-hub/apps/fs/internal/middleware"
)

func main() {
	_ = godotenv.Load()

	log := logger.New("fs")
	defer log.Sync()

	cfg, err := config.Load()
	if err != nil {
		log.Fatal("failed to load config", zap.Error(err))
	}

	// Load or generate keypair
	identity, needsRegistration, err := auth.LoadOrGenerate(cfg.DataDir)
	if err != nil {
		log.Fatal("failed to load/generate keypair", zap.Error(err))
	}

	if needsRegistration {
		if cfg.AgentToken == "" {
			log.Fatal("agent not registered and AGENT_TOKEN not set — generate a token in hub UI and set AGENT_TOKEN")
		}

		log.Info("registering agent with hub", zap.String("hub_api", cfg.HubAPIURL))
		agentID, err := hubclient.Register(cfg.HubAPIURL, cfg.AgentToken, identity.PublicKey, cfg.AgentLabel)
		if err != nil {
			log.Fatal("agent registration failed", zap.Error(err))
		}

		identity.AgentID = agentID
		if err := auth.Save(cfg.DataDir, identity); err != nil {
			log.Fatal("failed to save agent identity", zap.Error(err))
		}

		log.Info("agent registered successfully", zap.String("agent_id", agentID))
	} else {
		log.Info("loaded existing agent identity", zap.String("agent_id", identity.AgentID))
	}

	// Decode private key for WS client
	privateKey, err := identity.PrivateKeyBytes()
	if err != nil {
		log.Fatal("failed to decode private key", zap.Error(err))
	}

	// Start hub WS client in background
	ctx := context.Background()
	hc := hubclient.NewClient(cfg.HubWSURL, identity.AgentID, privateKey, log)
	go hc.Run(ctx)

	// HTTP server (health check + local API)
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

	log.Info("starting fs agent", zap.String("port", cfg.Port), zap.String("agent_id", identity.AgentID))
	if err := e.Start(":" + cfg.Port); err != nil && err != http.ErrServerClosed {
		log.Fatal("server error", zap.Error(err))
	}
}
