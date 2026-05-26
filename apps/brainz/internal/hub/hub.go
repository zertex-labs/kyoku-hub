package hub

import (
	"sync"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	brainzpb "github.com/zertex-labs/kyoku-hub/proto/gen/brainz/v1"
)

// Conn represents a connected hub instance.
type Conn struct {
	ID   string
	conn *websocket.Conn
	mu   sync.Mutex
}

func (c *Conn) Send(msg *brainzpb.Envelope) error {
	data, err := proto.Marshal(msg)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn.WriteMessage(websocket.BinaryMessage, data)
}

// Hub manages connected hub instances and their artist subscriptions.
type Hub struct {
	mu     sync.RWMutex
	conns  map[string]*Conn                    // instanceID -> conn
	subs   map[string]map[string]struct{}       // artistID -> set of instanceIDs
	logger *zap.Logger
}

func New(logger *zap.Logger) *Hub {
	return &Hub{
		conns:  make(map[string]*Conn),
		subs:   make(map[string]map[string]struct{}),
		logger: logger,
	}
}

func (h *Hub) Register(instanceID string, ws *websocket.Conn) *Conn {
	c := &Conn{ID: instanceID, conn: ws}
	h.mu.Lock()
	h.conns[instanceID] = c
	h.mu.Unlock()
	h.logger.Info("hub instance connected", zap.String("instance_id", instanceID))
	return c
}

func (h *Hub) Unregister(instanceID string) {
	h.mu.Lock()
	delete(h.conns, instanceID)
	// Remove from all subscriptions
	for artistID, instances := range h.subs {
		delete(instances, instanceID)
		if len(instances) == 0 {
			delete(h.subs, artistID)
		}
	}
	h.mu.Unlock()
	h.logger.Info("hub instance disconnected", zap.String("instance_id", instanceID))
}

func (h *Hub) Subscribe(instanceID string, artistIDs []string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, aid := range artistIDs {
		if h.subs[aid] == nil {
			h.subs[aid] = make(map[string]struct{})
		}
		h.subs[aid][instanceID] = struct{}{}
	}
}

func (h *Hub) Unsubscribe(instanceID string, artistIDs []string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, aid := range artistIDs {
		if instances, ok := h.subs[aid]; ok {
			delete(instances, instanceID)
			if len(instances) == 0 {
				delete(h.subs, aid)
			}
		}
	}
}

// Broadcast sends a message to all hub instances subscribed to the given artist.
func (h *Hub) Broadcast(artistID string, msg *brainzpb.Envelope) {
	h.mu.RLock()
	instances, ok := h.subs[artistID]
	if !ok {
		h.mu.RUnlock()
		return
	}
	// Copy instance IDs to avoid holding lock during send
	ids := make([]string, 0, len(instances))
	for id := range instances {
		ids = append(ids, id)
	}
	h.mu.RUnlock()

	for _, id := range ids {
		h.mu.RLock()
		conn, ok := h.conns[id]
		h.mu.RUnlock()
		if ok {
			if err := conn.Send(msg); err != nil {
				h.logger.Warn("failed to send to hub instance", zap.String("instance_id", id), zap.Error(err))
			}
		}
	}
}
