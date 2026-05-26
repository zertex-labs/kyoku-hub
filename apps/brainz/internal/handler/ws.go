package handler

import (
	"context"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	"github.com/zertex-labs/kyoku-hub/apps/brainz/internal/hub"
	brainzpb "github.com/zertex-labs/kyoku-hub/proto/gen/brainz/v1"
)

var upgrader = websocket.Upgrader{
	// Only allow non-browser clients (hub instances).
	// Browsers always send an Origin header; Go WS clients do not.
	CheckOrigin: func(r *http.Request) bool {
		return r.Header.Get("Origin") == ""
	},
}

type WSHandler struct {
	hub    *hub.Hub
	db     *pgxpool.Pool
	logger *zap.Logger
}

func NewWSHandler(h *hub.Hub, db *pgxpool.Pool, logger *zap.Logger) *WSHandler {
	return &WSHandler{hub: h, db: db, logger: logger}
}

func (h *WSHandler) RegisterRoutes(e *echo.Echo) {
	e.GET("/ws", h.HandleWS)
}

func (h *WSHandler) HandleWS(c echo.Context) error {
	ws, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		return err
	}
	defer ws.Close()

	// TODO: proper auth (challenge-response with keypair)
	// For now, use a query param as instance ID
	instanceID := c.QueryParam("instance_id")
	if instanceID == "" {
		h.logger.Warn("ws connection without instance_id")
		return nil
	}

	conn := h.hub.Register(instanceID, ws)
	defer h.hub.Unregister(instanceID)

	_ = conn
	// Read loop
	for {
		_, msg, err := ws.ReadMessage()
		if err != nil {
			h.logger.Debug("ws read error", zap.Error(err))
			return nil
		}

		var envelope brainzpb.Envelope
		if err := proto.Unmarshal(msg, &envelope); err != nil {
			h.logger.Warn("failed to unmarshal ws message", zap.Error(err))
			continue
		}

		h.handleMessage(context.Background(), instanceID, &envelope)
	}
}

func (h *WSHandler) handleMessage(ctx context.Context, instanceID string, env *brainzpb.Envelope) {
	switch payload := env.Payload.(type) {
	case *brainzpb.Envelope_Subscribe:
		h.hub.Subscribe(instanceID, payload.Subscribe.MbArtistIds)
		// Ensure artists exist in brainz DB so worker can pick them up
		for _, mbid := range payload.Subscribe.MbArtistIds {
			_, err := h.db.Exec(ctx,
				`INSERT INTO artists (mb_artist_id, name) VALUES ($1, '') ON CONFLICT DO NOTHING`,
				mbid,
			)
			if err != nil {
				h.logger.Error("failed to insert artist for subscription", zap.String("mb_artist_id", mbid), zap.Error(err))
			}
		}
		h.logger.Info("subscribe",
			zap.String("instance_id", instanceID),
			zap.Strings("artists", payload.Subscribe.MbArtistIds),
		)
	case *brainzpb.Envelope_Unsubscribe:
		h.hub.Unsubscribe(instanceID, payload.Unsubscribe.MbArtistIds)
		h.logger.Debug("unsubscribe",
			zap.String("instance_id", instanceID),
			zap.Int("artists", len(payload.Unsubscribe.MbArtistIds)),
		)
	case *brainzpb.Envelope_RequestUpdate:
		// Increment request count for priority queue
		_, err := h.db.Exec(ctx, `
			INSERT INTO artist_requests (mb_artist_id, hub_instance_id, request_count, last_requested_at)
			VALUES ($1, $2, 1, NOW())
			ON CONFLICT (mb_artist_id, hub_instance_id) DO UPDATE SET
				request_count = artist_requests.request_count + 1,
				last_requested_at = NOW()
		`, payload.RequestUpdate.MbArtistId, instanceID)
		if err != nil {
			h.logger.Error("failed to increment request count", zap.Error(err))
		}
		h.logger.Debug("request update",
			zap.String("instance_id", instanceID),
			zap.String("mb_artist_id", payload.RequestUpdate.MbArtistId),
		)
	default:
		h.logger.Warn("unknown message type from hub instance", zap.String("instance_id", instanceID))
	}
}
