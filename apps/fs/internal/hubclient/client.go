package hubclient

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	fspb "github.com/zertex-labs/kyoku-hub/proto/gen/fs/v1"
)

const (
	authTimeout    = 10 * time.Second
	connectRetry   = 5 * time.Second
	reconnectDelay = 2 * time.Second
	writeWait      = 10 * time.Second
)

// Client manages the outbound WebSocket connection from fs to hub.
type Client struct {
	url        string
	agentID    string
	privateKey ed25519.PrivateKey
	logger     *zap.Logger

	conn *websocket.Conn
	mu   sync.Mutex
}

func NewClient(hubWSURL, agentID string, privateKey ed25519.PrivateKey, logger *zap.Logger) *Client {
	return &Client{
		url:        hubWSURL,
		agentID:    agentID,
		privateKey: privateKey,
		logger:     logger,
	}
}

// Run connects to hub and maintains the connection. Reconnects on failure. Blocks until ctx is cancelled.
func (c *Client) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if err := c.connect(ctx); err != nil {
			c.logger.Warn("hub ws connect failed, retrying", zap.Duration("retry_in", connectRetry), zap.Error(err))
			select {
			case <-ctx.Done():
				return
			case <-time.After(connectRetry):
			}
			continue
		}

		c.readLoop(ctx)
		c.logger.Info("hub ws disconnected, reconnecting", zap.Duration("retry_in", reconnectDelay))
		select {
		case <-ctx.Done():
			return
		case <-time.After(reconnectDelay):
		}
	}
}

func (c *Client) connect(ctx context.Context) error {
	url := c.url + "?agent_id=" + c.agentID
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, url, nil)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}

	// Run challenge-response auth
	if err := c.authenticate(conn); err != nil {
		conn.Close()
		return fmt.Errorf("auth: %w", err)
	}

	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()

	c.logger.Info("connected to hub", zap.String("url", c.url))
	return nil
}

// authenticate handles the challenge-response handshake with hub.
func (c *Client) authenticate(conn *websocket.Conn) error {
	// Read challenge from hub
	conn.SetReadDeadline(time.Now().Add(authTimeout))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		return fmt.Errorf("read challenge: %w", err)
	}

	var envelope fspb.Envelope
	if err := proto.Unmarshal(msg, &envelope); err != nil {
		return fmt.Errorf("unmarshal challenge: %w", err)
	}

	challenge, ok := envelope.Payload.(*fspb.Envelope_Challenge)
	if !ok {
		return fmt.Errorf("expected challenge, got different message type")
	}

	// Sign the nonce with our private key
	signature := ed25519.Sign(c.privateKey, challenge.Challenge.Nonce)

	// Send signature back
	resp := &fspb.Envelope{
		Payload: &fspb.Envelope_ChallengeResponse{
			ChallengeResponse: &fspb.ChallengeResponse{
				Signature: signature,
			},
		},
	}
	data, err := proto.Marshal(resp)
	if err != nil {
		return fmt.Errorf("marshal response: %w", err)
	}
	conn.SetWriteDeadline(time.Now().Add(writeWait))
	if err := conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
		return fmt.Errorf("send response: %w", err)
	}

	// Read auth result
	conn.SetReadDeadline(time.Now().Add(authTimeout))
	_, msg, err = conn.ReadMessage()
	if err != nil {
		return fmt.Errorf("read auth result: %w", err)
	}

	var resultEnv fspb.Envelope
	if err := proto.Unmarshal(msg, &resultEnv); err != nil {
		return fmt.Errorf("unmarshal auth result: %w", err)
	}

	result, ok := resultEnv.Payload.(*fspb.Envelope_AuthResult)
	if !ok {
		return fmt.Errorf("expected auth_result, got different message type")
	}

	if !result.AuthResult.Success {
		return fmt.Errorf("auth rejected: %s", result.AuthResult.Error)
	}

	// Reset deadlines for normal operation
	conn.SetReadDeadline(time.Time{})
	conn.SetWriteDeadline(time.Time{})
	return nil
}

func (c *Client) readLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			c.mu.Lock()
			if c.conn != nil {
				c.conn.Close()
			}
			c.mu.Unlock()
			return
		default:
		}

		_, msg, err := c.conn.ReadMessage()
		if err != nil {
			c.logger.Debug("hub ws read error", zap.Error(err))
			return
		}

		var envelope fspb.Envelope
		if err := proto.Unmarshal(msg, &envelope); err != nil {
			c.logger.Warn("failed to unmarshal hub message", zap.Error(err))
			continue
		}

		c.handleMessage(ctx, &envelope)
	}
}

func (c *Client) handleMessage(ctx context.Context, env *fspb.Envelope) {
	// Future: handle scan requests, etc.
	c.logger.Debug("received message from hub (no handlers yet)")
}

func (c *Client) send(msg *fspb.Envelope) error {
	data, err := proto.Marshal(msg)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return fmt.Errorf("not connected")
	}
	c.conn.SetWriteDeadline(time.Now().Add(writeWait))
	return c.conn.WriteMessage(websocket.BinaryMessage, data)
}
