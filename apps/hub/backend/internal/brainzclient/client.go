package brainzclient

import (
	"context"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/database/queries"
	brainzpb "github.com/zertex-labs/kyoku-hub/proto/gen/brainz/v1"
)

type Client struct {
	url        string
	instanceID string
	queries    *queries.Queries
	logger     *zap.Logger

	conn *websocket.Conn
	mu   sync.Mutex

	// Called when brainz pushes an artist update
	OnArtistUpdated func(ctx context.Context, msg *brainzpb.ArtistUpdated)
	OnStatusChanged func(ctx context.Context, msg *brainzpb.ArtistStatusChanged)
}

func New(brainzURL, instanceID string, q *queries.Queries, logger *zap.Logger) *Client {
	return &Client{
		url:        brainzURL,
		instanceID: instanceID,
		queries:    q,
		logger:     logger,
	}
}

// Run connects to brainz and reads messages. Reconnects on failure. Blocks until ctx is cancelled.
func (c *Client) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if err := c.connect(ctx); err != nil {
			c.logger.Warn("brainz ws connect failed, retrying in 5s", zap.Error(err))
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
			continue
		}

		c.readLoop(ctx)
		c.logger.Info("brainz ws disconnected, reconnecting in 2s")
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
}

func (c *Client) connect(ctx context.Context) error {
	url := c.url + "?instance_id=" + c.instanceID
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, url, nil)
	if err != nil {
		return err
	}

	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()

	c.logger.Info("connected to brainz", zap.String("url", c.url))
	return nil
}

func (c *Client) readLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			c.conn.Close()
			return
		default:
		}

		_, msg, err := c.conn.ReadMessage()
		if err != nil {
			c.logger.Debug("brainz ws read error", zap.Error(err))
			return
		}

		var envelope brainzpb.Envelope
		if err := proto.Unmarshal(msg, &envelope); err != nil {
			c.logger.Warn("failed to unmarshal brainz message", zap.Error(err))
			continue
		}

		c.handleMessage(ctx, &envelope)
	}
}

func (c *Client) handleMessage(ctx context.Context, env *brainzpb.Envelope) {
	switch payload := env.Payload.(type) {
	case *brainzpb.Envelope_ArtistUpdated:
		if c.OnArtistUpdated != nil {
			c.OnArtistUpdated(ctx, payload.ArtistUpdated)
		}
	case *brainzpb.Envelope_ArtistStatusChanged:
		if c.OnStatusChanged != nil {
			c.OnStatusChanged(ctx, payload.ArtistStatusChanged)
		}
	}
}

// Subscribe sends a subscribe message to brainz.
func (c *Client) Subscribe(artistIDs []string) error {
	return c.send(&brainzpb.Envelope{
		Payload: &brainzpb.Envelope_Subscribe{
			Subscribe: &brainzpb.Subscribe{
				MbArtistIds: artistIDs,
			},
		},
	})
}

// Unsubscribe sends an unsubscribe message to brainz.
func (c *Client) Unsubscribe(artistIDs []string) error {
	return c.send(&brainzpb.Envelope{
		Payload: &brainzpb.Envelope_Unsubscribe{
			Unsubscribe: &brainzpb.Unsubscribe{
				MbArtistIds: artistIDs,
			},
		},
	})
}

// RequestUpdate sends a priority request for an artist.
func (c *Client) RequestUpdate(artistID string) error {
	return c.send(&brainzpb.Envelope{
		Payload: &brainzpb.Envelope_RequestUpdate{
			RequestUpdate: &brainzpb.RequestUpdate{
				MbArtistId: artistID,
			},
		},
	})
}

func (c *Client) send(msg *brainzpb.Envelope) error {
	data, err := proto.Marshal(msg)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return nil // not connected yet, will re-subscribe on reconnect
	}
	return c.conn.WriteMessage(websocket.BinaryMessage, data)
}
