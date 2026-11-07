package main

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
	echomw "github.com/labstack/echo/v4/middleware"
	"go.uber.org/zap"

	brainzpb "github.com/zertex-labs/kyoku-hub/proto/gen/brainz/v1"

	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/brainzclient"
	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/config"
	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/database"
	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/database/queries"
	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/download"
	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/fsagent"
	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/handler"
	mw "github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/middleware"
	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/plugins"
	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/sse"
	logger "github.com/zertex-labs/kyoku-hub/packages/logger"
	api "github.com/zertex-labs/kyoku-hub/packages/plugin-api"
)

func main() {
	// The hub re-execs itself as the plugin launcher: it confines the child and
	// then becomes the plugin, so the ruleset is inherited and cannot be undone.
	if len(os.Args) > 3 && os.Args[1] == plugins.SandboxFlag {
		argv := os.Args[3:]
		if argv[0] == "--" { // tolerate an explicit separator on the command line
			argv = argv[1:]
		}
		if err := plugins.RunSandboxed(os.Args[2], argv); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	_ = godotenv.Load()

	log := logger.New("hub")
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
	sseManager := sse.NewManager()
	bc := brainzclient.New(cfg.BrainzWSURL, cfg.InstanceID, q, log)
	bc.OnArtistUpdated = func(ctx context.Context, msg *brainzpb.ArtistUpdated) {
		if err := q.UpdateArtistFromBrainz(ctx, msg.MbArtistId, msg.Name, msg.Disambiguation, msg.ImageUrl, msg.Description, len(msg.ReleaseGroups)); err != nil {
			log.Error("failed to update artist from brainz", zap.String("mb_artist_id", msg.MbArtistId), zap.Error(err))
		}
		sseManager.Broadcast("artist:updated", map[string]interface{}{
			"mb_artist_id":        msg.MbArtistId,
			"name":                msg.Name,
			"disambiguation":      msg.Disambiguation,
			"image_url":           msg.ImageUrl,
			"description":         msg.Description,
			"release_group_count": len(msg.ReleaseGroups),
			"status":              "synced",
		})
	}
	bc.OnStatusChanged = func(ctx context.Context, msg *brainzpb.ArtistStatusChanged) {
		if msg.Status == "failed" {
			// Invalid artist — clean up from hub DB
			if err := q.DeleteArtistAndLinks(ctx, msg.MbArtistId); err != nil {
				log.Error("failed to delete invalid artist", zap.String("mb_artist_id", msg.MbArtistId), zap.Error(err))
			}
			sseManager.Broadcast("artist:failed", map[string]interface{}{
				"mb_artist_id": msg.MbArtistId,
			})
			return
		}

		if err := q.UpdateArtistStatus(ctx, msg.MbArtistId, msg.Status); err != nil {
			log.Error("failed to update artist status", zap.String("mb_artist_id", msg.MbArtistId), zap.Error(err))
		}
		sseManager.Broadcast("artist:status", map[string]interface{}{
			"mb_artist_id": msg.MbArtistId,
			"status":       msg.Status,
		})
	}
	go bc.Run(ctx)

	eventsHandler := handler.NewEventsHandler(sseManager)
	e.GET("/api/events", eventsHandler.Stream, mw.AuthRequired(q))

	// Brainz HTTP API client (for proxying REST requests)
	brainzAPI := brainzclient.NewAPI(cfg.BrainzAPIURL)

	// Fs agent WS endpoint (challenge-response auth, no session needed)
	agentManager := fsagent.NewManager(q, sseManager, cfg.RequiredTags, log)
	e.GET("/ws/agent", agentManager.HandleWS)

	artistHandler := handler.NewArtistHandler(cfg, q, bc, brainzAPI, agentManager, log)
	artistHandler.Register(e.Group("/api/artist"))

	// Fs agent management (REST endpoints)
	agentHandler := handler.NewAgentHandler(q, agentManager)
	agentHandler.Register(e.Group("/api/agent"))

	// Fs sync + on-demand scan details
	fsSyncHandler := handler.NewFsSyncHandler(q, agentManager)
	fsSyncHandler.Register(e.Group("/api/fs"))

	// Downloader — optional; the handler returns 503 when the user has no sources
	// connected. Sources come from provider_credentials, so the walk is per user.
	sources := handler.SourceResolver(q, log)
	downloadManager := download.NewManager(cfg.DownloadDir, sources, sseManager, handler.NewJobStore(q), log)
	downloadManager.OnPublish = func(userID, agentID string) {
		if agentID != "" {
			agentManager.RequestSyncAgent(userID, agentID, false)
			return
		}
		agentManager.RequestSync(userID, false)
	}
	appInfo := api.App{Name: "kyoku-hub", Version: download.AppVersion}

	// Plugin config is encrypted with a key derived from the session secret: a
	// hub that cannot protect it should not offer the feature at all.
	var pluginStoreConcrete *plugins.Store
	var pluginStore plugins.ConfigStore
	if box, err := plugins.NewSecretBox(cfg.SessionSecret); err != nil {
		log.Warn("plugin config disabled", zap.Error(err))
	} else {
		pluginStoreConcrete = plugins.NewStore(q, box)
		pluginStore = pluginStoreConcrete
	}

	var pluginRegistry *plugins.Registry
	if registry, err := plugins.NewRegistry(cfg.PluginsDir, pluginStore, appInfo, log); err != nil {
		log.Warn("plugins not loaded", zap.String("dir", cfg.PluginsDir), zap.Error(err))
	} else {
		pluginRegistry = registry
		downloadManager.SetDispatcher(registry)
		if n := registry.Count(); n > 0 {
			log.Info("plugins loaded", zap.Int("count", n), zap.Strings("names", registry.Names()))
		}
	}

	pluginHandler := handler.NewPluginHandler(pluginRegistry, pluginStoreConcrete, downloadManager, q)
	pluginHandler.Register(e.Group("/api/plugins"))

	downloadHandler := handler.NewDownloadHandler(downloadManager, sources, q, log)
	downloadHandler.Register(e.Group("/api/download"))

	connectionsHandler := handler.NewConnectionsHandler(q)
	connectionsHandler.Register(e.Group("/api/connections"))

	log.Info("starting hub backend", zap.String("port", cfg.Port))
	if err := e.Start(":" + cfg.Port); err != nil && err != http.ErrServerClosed {
		log.Fatal("server error", zap.Error(err))
	}
}
