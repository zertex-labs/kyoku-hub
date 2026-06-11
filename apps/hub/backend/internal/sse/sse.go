package sse

import (
	"encoding/json"
	"fmt"
	"sync"
)

// Client represents a connected SSE client.
type Client struct {
	UserID string
	Events chan []byte
}

// Manager manages SSE client connections and broadcasts events.
type Manager struct {
	mu      sync.RWMutex
	clients map[*Client]struct{}
}

// NewManager creates a new SSE manager.
func NewManager() *Manager {
	return &Manager{
		clients: make(map[*Client]struct{}),
	}
}

// Register adds a new client owned by the given user and returns it.
func (m *Manager) Register(userID string) *Client {
	c := &Client{
		UserID: userID,
		Events: make(chan []byte, 64),
	}
	m.mu.Lock()
	m.clients[c] = struct{}{}
	m.mu.Unlock()
	return c
}

// Unregister removes a client and closes its channel.
func (m *Manager) Unregister(c *Client) {
	m.mu.Lock()
	delete(m.clients, c)
	m.mu.Unlock()
	close(c.Events)
}

// Broadcast sends an event to all connected clients. Use for shared state
// (e.g. artist updates); never for user-private data.
func (m *Manager) Broadcast(event string, data interface{}) {
	msg, err := formatEvent(event, data)
	if err != nil {
		return
	}

	m.mu.RLock()
	defer m.mu.RUnlock()
	for c := range m.clients {
		c.trySend(msg)
	}
}

// SendToUser sends an event only to clients belonging to the given user.
func (m *Manager) SendToUser(userID, event string, data interface{}) {
	msg, err := formatEvent(event, data)
	if err != nil {
		return
	}

	m.mu.RLock()
	defer m.mu.RUnlock()
	for c := range m.clients {
		if c.UserID == userID {
			c.trySend(msg)
		}
	}
}

// ClientCount returns the number of connected clients.
func (m *Manager) ClientCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.clients)
}

func (c *Client) trySend(msg []byte) {
	select {
	case c.Events <- msg:
	default:
		// Client buffer full, skip
	}
}

func formatEvent(event string, data interface{}) ([]byte, error) {
	payload, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	return fmt.Appendf(nil, "event: %s\ndata: %s\n\n", event, payload), nil
}
