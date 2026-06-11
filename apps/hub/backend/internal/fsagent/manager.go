package fsagent

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/database/queries"
	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/model"
	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/sse"
	"github.com/zertex-labs/kyoku-hub/packages/validation"
	fspb "github.com/zertex-labs/kyoku-hub/proto/gen/fs/v1"
)

const (
	challengeSize = 32
	authTimeout   = 10 * time.Second
	writeWait     = 10 * time.Second
	pongWait      = 60 * time.Second
	pingPeriod    = (pongWait * 9) / 10
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Conn represents an authenticated fs agent connection.
type Conn struct {
	AgentID string
	UserID  string
	ws      *websocket.Conn
	mu      sync.Mutex
}

func (c *Conn) sendProto(msg *fspb.Envelope) error {
	data, err := proto.Marshal(msg)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ws.SetWriteDeadline(time.Now().Add(writeWait))
	return c.ws.WriteMessage(websocket.BinaryMessage, data)
}

// Manager tracks connected fs agent connections per user.
type Manager struct {
	mu sync.RWMutex
	// userID -> agentID -> *Conn
	conns        map[string]map[string]*Conn
	queries      *queries.Queries
	sseManager   *sse.Manager
	state        *stateStore
	requiredTags []string
	logger       *zap.Logger
}

func NewManager(q *queries.Queries, sseManager *sse.Manager, requiredTags []string, logger *zap.Logger) *Manager {
	return &Manager{
		conns:        make(map[string]map[string]*Conn),
		queries:      q,
		sseManager:   sseManager,
		state:        newStateStore(),
		requiredTags: requiredTags,
		logger:       logger,
	}
}

// HandleWS is the Echo handler for GET /ws/agent. Upgrades to WS and runs challenge-response auth.
func (m *Manager) HandleWS(c echo.Context) error {
	agentID := c.QueryParam("agent_id")
	if agentID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "missing agent_id"})
	}

	// Look up agent in DB
	agent, err := m.queries.GetFsAgent(c.Request().Context(), agentID)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unknown agent"})
	}

	// Decode stored public key
	pubKeyBytes, err := base64.StdEncoding.DecodeString(agent.PublicKey)
	if err != nil || len(pubKeyBytes) != ed25519.PublicKeySize {
		m.logger.Error("invalid stored public key", zap.String("agent_id", agentID))
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "invalid agent key"})
	}
	pubKey := ed25519.PublicKey(pubKeyBytes)

	// Upgrade to WebSocket
	ws, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		return err
	}
	defer ws.Close()

	// Run challenge-response auth
	conn := &Conn{AgentID: agentID, UserID: agent.UserID, ws: ws}

	if err := m.authenticate(conn, pubKey); err != nil {
		m.logger.Warn("agent auth failed", zap.String("agent_id", agentID), zap.Error(err))
		return nil // WS already closed by authenticate on failure
	}

	m.logger.Info("fs agent connected",
		zap.String("agent_id", agentID),
		zap.String("user_id", agent.UserID),
		zap.String("label", agent.Label),
	)

	m.register(conn)
	defer m.unregister(conn)

	m.touchLastSeen(agentID)
	// Record the last moment the agent was online when it disconnects.
	defer m.touchLastSeen(agentID)

	// Request the agent's current library state immediately so hub data is
	// fresh after every (re)connect — no manual sync needed.
	if err := conn.sendProto(&fspb.Envelope{
		Payload: &fspb.Envelope_SyncRequest{
			SyncRequest: &fspb.SyncRequest{Full: false},
		},
	}); err != nil {
		m.logger.Warn("failed to request initial sync", zap.String("agent_id", agentID), zap.Error(err))
	}

	m.readLoop(conn)
	return nil
}

func (m *Manager) touchLastSeen(agentID string) {
	if err := m.queries.TouchFsAgentLastSeen(context.Background(), agentID); err != nil {
		m.logger.Warn("failed to update agent last_seen", zap.String("agent_id", agentID), zap.Error(err))
	}
}

// authenticate runs the challenge-response handshake.
func (m *Manager) authenticate(conn *Conn, pubKey ed25519.PublicKey) error {
	// Generate nonce
	nonce := make([]byte, challengeSize)
	if _, err := rand.Read(nonce); err != nil {
		return fmt.Errorf("generate nonce: %w", err)
	}

	// Send challenge
	if err := conn.sendProto(&fspb.Envelope{
		Payload: &fspb.Envelope_Challenge{
			Challenge: &fspb.Challenge{Nonce: nonce},
		},
	}); err != nil {
		return fmt.Errorf("send challenge: %w", err)
	}

	// Read response with timeout
	conn.ws.SetReadDeadline(time.Now().Add(authTimeout))
	_, msg, err := conn.ws.ReadMessage()
	if err != nil {
		return fmt.Errorf("read challenge response: %w", err)
	}

	var envelope fspb.Envelope
	if err := proto.Unmarshal(msg, &envelope); err != nil {
		return fmt.Errorf("unmarshal response: %w", err)
	}

	resp, ok := envelope.Payload.(*fspb.Envelope_ChallengeResponse)
	if !ok {
		conn.sendAuthResult(false, "expected challenge_response")
		return fmt.Errorf("unexpected message type")
	}

	// Verify signature
	if !ed25519.Verify(pubKey, nonce, resp.ChallengeResponse.Signature) {
		conn.sendAuthResult(false, "invalid signature")
		return fmt.Errorf("signature verification failed")
	}

	// Auth success
	conn.sendAuthResult(true, "")
	// Reset read deadline for normal operation
	conn.ws.SetReadDeadline(time.Time{})
	return nil
}

func (c *Conn) sendAuthResult(success bool, errMsg string) {
	c.sendProto(&fspb.Envelope{
		Payload: &fspb.Envelope_AuthResult{
			AuthResult: &fspb.AuthResult{
				Success: success,
				Error:   errMsg,
			},
		},
	})
}

func (m *Manager) register(conn *Conn) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.conns[conn.UserID] == nil {
		m.conns[conn.UserID] = make(map[string]*Conn)
	}
	m.conns[conn.UserID][conn.AgentID] = conn
}

func (m *Manager) unregister(conn *Conn) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if agents, ok := m.conns[conn.UserID]; ok {
		delete(agents, conn.AgentID)
		if len(agents) == 0 {
			delete(m.conns, conn.UserID)
		}
	}
	conn.ws.Close()
	m.logger.Info("fs agent disconnected",
		zap.String("agent_id", conn.AgentID),
		zap.String("user_id", conn.UserID),
	)
}

func (m *Manager) readLoop(conn *Conn) {
	conn.ws.SetReadDeadline(time.Now().Add(pongWait))
	conn.ws.SetPongHandler(func(string) error {
		conn.ws.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	// Start ping ticker
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(pingPeriod)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				conn.mu.Lock()
				conn.ws.SetWriteDeadline(time.Now().Add(writeWait))
				err := conn.ws.WriteMessage(websocket.PingMessage, nil)
				conn.mu.Unlock()
				if err != nil {
					return
				}
			case <-done:
				return
			}
		}
	}()
	defer close(done)

	for {
		_, msg, err := conn.ws.ReadMessage()
		if err != nil {
			m.logger.Debug("fs agent read error", zap.String("agent_id", conn.AgentID), zap.Error(err))
			return
		}

		var envelope fspb.Envelope
		if err := proto.Unmarshal(msg, &envelope); err != nil {
			m.logger.Warn("failed to unmarshal fs agent message", zap.String("agent_id", conn.AgentID), zap.Error(err))
			continue
		}

		m.handleMessage(conn, &envelope)
	}
}

func (m *Manager) handleMessage(conn *Conn, env *fspb.Envelope) {
	switch p := env.Payload.(type) {
	case *fspb.Envelope_ScanResult:
		m.processScanResult(conn, p.ScanResult)
	default:
		m.logger.Debug("received unhandled message from fs agent",
			zap.String("agent_id", conn.AgentID),
		)
	}
}

func (m *Manager) processScanResult(conn *Conn, result *fspb.ScanResult) {
	ctx := context.Background()
	userID := conn.UserID

	type rgGroup struct {
		perfect bool
		issues  []TrackIssue
	}
	groups := make(map[string]*rgGroup)
	var untagged []UntaggedTrack

	for _, t := range result.Tracks {
		missing := validation.Missing(validation.Track{
			Title:    t.Title,
			Album:    t.Album,
			Artist:   t.Artist,
			Year:     int(t.Year),
			TrackNum: int(t.TrackNum),
		}, m.requiredTags)

		if t.MusicbrainzReleaseGroupId == "" {
			untagged = append(untagged, UntaggedTrack{
				AgentID: conn.AgentID,
				Rpath:   t.Rpath,
				Title:   t.Title,
				Album:   t.Album,
				Artist:  t.Artist,
			})
			continue
		}

		rg, ok := groups[t.MusicbrainzReleaseGroupId]
		if !ok {
			rg = &rgGroup{perfect: true}
			groups[t.MusicbrainzReleaseGroupId] = rg
		}
		if len(missing) > 0 {
			rg.perfect = false
			rg.issues = append(rg.issues, TrackIssue{Rpath: t.Rpath, Missing: missing})
		}
	}

	states := make(map[string]string, len(groups))
	issues := make(map[string][]TrackIssue)
	for rgID, rg := range groups {
		if rg.perfect {
			states[rgID] = string(model.RGStatePerfect)
		} else {
			states[rgID] = string(model.RGStateBad)
			issues[rgID] = rg.issues
		}
	}

	// Replace (not merge) this agent's states so removed/fixed files don't
	// leave stale rows behind. An empty scan clears them entirely.
	if err := m.queries.ReplaceAgentReleaseGroupStates(ctx, userID, conn.AgentID, states); err != nil {
		m.logger.Error("failed to replace release group states",
			zap.String("agent_id", conn.AgentID),
			zap.Error(err),
		)
		return
	}

	m.state.setScan(userID, conn.AgentID, issues, untagged)

	// Issue/untagged details are fetched on demand over REST; the event just
	// signals "fresh data available".
	m.sseManager.SendToUser(userID, "fs:synced", map[string]interface{}{
		"agent_id":       conn.AgentID,
		"tracks":         len(result.Tracks),
		"release_groups": len(states),
		"untagged":       len(untagged),
	})

	m.logger.Info("processed scan result",
		zap.String("agent_id", conn.AgentID),
		zap.Int("tracks", len(result.Tracks)),
		zap.Int("release_groups", len(states)),
		zap.Int("untagged", len(untagged)),
	)
}

// GetUserConns returns all active agent connections for a user.
func (m *Manager) GetUserConns(userID string) []*Conn {
	m.mu.RLock()
	defer m.mu.RUnlock()
	agents, ok := m.conns[userID]
	if !ok {
		return nil
	}
	conns := make([]*Conn, 0, len(agents))
	for _, c := range agents {
		conns = append(conns, c)
	}
	return conns
}

// RequestSync sends a SyncRequest to all active agents for the given user.
// Returns the number of agents that received the request.
func (m *Manager) RequestSync(userID string, full bool) int {
	conns := m.GetUserConns(userID)
	sent := 0
	for _, c := range conns {
		if err := c.sendProto(&fspb.Envelope{
			Payload: &fspb.Envelope_SyncRequest{
				SyncRequest: &fspb.SyncRequest{Full: full},
			},
		}); err != nil {
			m.logger.Warn("failed to send sync request",
				zap.String("agent_id", c.AgentID),
				zap.Error(err),
			)
			continue
		}
		sent++
	}
	return sent
}

// IsConnected reports whether the given agent currently has a live WS connection.
func (m *Manager) IsConnected(userID, agentID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.conns[userID][agentID]
	return ok
}

// IssuesFor returns tag issues for one release group, merged across the
// user's agents.
func (m *Manager) IssuesFor(userID, rgID string) []TrackIssue {
	return m.state.issuesFor(userID, rgID)
}

// UntaggedFor returns all scanned tracks lacking MusicBrainz IDs for a user.
func (m *Manager) UntaggedFor(userID string) []UntaggedTrack {
	return m.state.untaggedFor(userID)
}

// HasUntaggedMatch reports whether an untagged track plausibly matches the
// given artist name + release-group title.
func (m *Manager) HasUntaggedMatch(userID, artistName, rgTitle string) bool {
	return m.state.hasUntaggedMatch(userID, artistName, rgTitle)
}

// DropAgent disconnects an agent's live connection (if any) and discards its
// in-memory scan state. Called when an agent registration is deleted.
func (m *Manager) DropAgent(userID, agentID string) {
	m.mu.RLock()
	conn := m.conns[userID][agentID]
	m.mu.RUnlock()
	if conn != nil {
		conn.ws.Close() // readLoop exits -> unregister runs
	}
	m.state.dropAgent(userID, agentID)
}
