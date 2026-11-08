package hubclient

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	"github.com/zertex-labs/kyoku-hub/apps/fs/internal/handler"
	"github.com/zertex-labs/kyoku-hub/apps/fs/internal/writer"
	fspb "github.com/zertex-labs/kyoku-hub/proto/gen/fs/v1"
)

const (
	authTimeout = 10 * time.Second
	writeWait   = 10 * time.Second

	// Reconnect backoff: exponential with jitter, reset after a successful connect.
	backoffMin = 1 * time.Second
	backoffMax = 60 * time.Second

	// Hub pings every ~54s; if nothing arrives within this window the
	// connection is considered dead and we reconnect.
	serverPingWait = 90 * time.Second
)

// Client manages the outbound WebSocket connection from fs to hub.
type Client struct {
	url        string
	agentID    string
	privateKey ed25519.PrivateKey
	fsHandler  *handler.FsHandler
	logger     *zap.Logger

	conn *websocket.Conn
	mu   sync.Mutex

	scanMu sync.Mutex // serializes scans triggered by sync requests

	// libraryDir is where manifests' relative paths resolve.
	libraryDir string

	// writeMu serializes write jobs: they are large sequential writes to the
	// user's disk, and two albums at once only makes the progress meaningless.
	writeMu  sync.Mutex
	executor *writer.Executor
}

func NewClient(hubWSURL, agentID string, privateKey ed25519.PrivateKey, fsHandler *handler.FsHandler, libraryDir string, logger *zap.Logger) *Client {
	return &Client{
		url:        hubWSURL,
		agentID:    agentID,
		privateKey: privateKey,
		fsHandler:  fsHandler,
		libraryDir: libraryDir,
		logger:     logger,
	}
}

// Run connects to hub and maintains the connection. Reconnects on failure
// with exponential backoff. Blocks until ctx is cancelled.
func (c *Client) Run(ctx context.Context) {
	backoff := backoffMin

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if err := c.connect(ctx); err != nil {
			delay := withJitter(backoff)
			c.logger.Warn("hub ws connect failed, retrying", zap.Duration("retry_in", delay), zap.Error(err))
			if !sleepCtx(ctx, delay) {
				return
			}
			backoff = nextBackoff(backoff)
			continue
		}

		backoff = backoffMin // connected + authenticated: reset backoff
		c.readLoop(ctx)

		delay := withJitter(backoff)
		c.logger.Info("hub ws disconnected, reconnecting", zap.Duration("retry_in", delay))
		if !sleepCtx(ctx, delay) {
			return
		}
		backoff = nextBackoff(backoff)
	}
}

func nextBackoff(d time.Duration) time.Duration {
	d *= 2
	if d > backoffMax {
		return backoffMax
	}
	return d
}

// withJitter returns d ± up to 25% to avoid reconnect stampedes.
func withJitter(d time.Duration) time.Duration {
	jitter := time.Duration(rand.Int63n(int64(d) / 2))
	return d*3/4 + jitter
}

// sleepCtx sleeps for d; returns false if ctx was cancelled first.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
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

	// Detect half-dead connections: hub pings periodically, so something
	// must arrive within serverPingWait or the read times out.
	conn.SetReadDeadline(time.Now().Add(serverPingWait))
	conn.SetPingHandler(func(message string) error {
		conn.SetReadDeadline(time.Now().Add(serverPingWait))
		return conn.WriteControl(websocket.PongMessage, []byte(message), time.Now().Add(writeWait))
	})

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

// reply answers a request, echoing its id so the hub can match the two. Without
// that the hub has no way to know which of several answers belongs to which
// question, and would have to guess by shape.
func (c *Client) reply(requestID string, env *fspb.Envelope) error {
	if requestID != "" {
		env.RequestId = requestID
	}
	return c.send(env)
}

func (c *Client) handleMessage(ctx context.Context, env *fspb.Envelope) {
	switch p := env.Payload.(type) {
	case *fspb.Envelope_SyncRequest:
		go c.handleSyncRequest(p.SyncRequest)
	case *fspb.Envelope_WriteAlbum:
		go c.handleWriteAlbum(p.WriteAlbum)
	default:
		c.logger.Debug("received unhandled message from hub")
	}
}

// handleWriteAlbum performs a job the hub cannot: the hub has no access to this
// disk. Every step reports back as a WriteEvent, including failure, so the hub
// never has to infer an outcome from silence.
func (c *Client) handleWriteAlbum(album *fspb.WriteAlbum) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	if c.executor == nil {
		c.executor = writer.New(c.libraryDir)
	}
	if album == nil {
		return
	}
	c.logger.Info("write job received",
		zap.String("job_id", album.GetJobId()),
		zap.String("kind", album.GetKind()),
		zap.Int("tracks", len(album.GetTracks())),
	)

	err := c.executor.Run(context.Background(), album, func(ev *fspb.WriteEvent) {
		if sendErr := c.send(&fspb.Envelope{
			Payload: &fspb.Envelope_WriteEvent{WriteEvent: ev},
		}); sendErr != nil {
			c.logger.Warn("failed to send write event", zap.Error(sendErr))
		}
	})
	if err != nil {
		c.logger.Warn("write job failed", zap.String("job_id", album.GetJobId()), zap.Error(err))
		return
	}

	// The verdict changed: tell the hub to re-read this library, which is what
	// keeps the album grid honest after a download or a re-tag.
	go c.handleSyncRequest(&fspb.SyncRequest{Full: false})
}

func (c *Client) handleSyncRequest(req *fspb.SyncRequest) {
	// Serialize scans. Full rescans are explicit user intent, so they wait
	// for the running scan; incremental ones arriving mid-scan are dropped
	// (the in-flight scan's result is fresh enough).
	if !c.scanMu.TryLock() {
		if !req.Full {
			c.logger.Info("scan already in progress, skipping sync request")
			return
		}
		c.scanMu.Lock()
	}
	defer c.scanMu.Unlock()

	c.logger.Info("received sync request", zap.Bool("full", req.Full))

	dirs, err := c.fsHandler.ScanDirs(c.fsHandler.ScanDirsConfig(), req.Full)
	if err != nil {
		c.logger.Error("scan failed", zap.Error(err))
		return
	}

	var tracks []*fspb.TrackEntry
	for _, files := range dirs {
		for _, f := range files {
			tracks = append(tracks, &fspb.TrackEntry{
				Rpath:                     f.RelativePath,
				Title:                     f.MusicTags.Title,
				Album:                     f.MusicTags.Album,
				Artist:                    f.MusicTags.Artist,
				Year:                      int32(f.MusicTags.Year),
				TrackNum:                  int32(f.MusicTags.TrackNum),
				MusicbrainzReleaseGroupId: f.MusicTags.MusicBrainzReleaseGroupID,
				MusicbrainzArtistId:       f.MusicTags.MusicBrainzArtistID,
				MusicbrainzReleaseId:      f.MusicTags.MusicBrainzReleaseID,
				MusicbrainzTrackId:        f.MusicTags.MusicBrainzRecordingID,
				Isrc:                      f.MusicTags.ISRC,
			})
		}
	}

	if err := c.send(&fspb.Envelope{
		Payload: &fspb.Envelope_ScanResult{
			ScanResult: &fspb.ScanResult{Tracks: tracks},
		},
	}); err != nil {
		c.logger.Error("failed to send scan result", zap.Error(err))
	}

	c.logger.Info("scan result sent", zap.Int("tracks", len(tracks)))
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
