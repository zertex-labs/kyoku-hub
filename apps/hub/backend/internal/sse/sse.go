package sse

import (
	"encoding/json"
	"fmt"
	"sync"
)

// Client represents a connected SSE client.
type Client struct {
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

// Register adds a new client and returns it.
func (m *Manager) Register() *Client {
	c := &Client{
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

// Broadcast sends an event to all connected clients.
func (m *Manager) Broadcast(event string, data interface{}) {
	payload, err := json.Marshal(data)
	if err != nil {
		return
	}

	msg := fmt.Appendf(nil, "event: %s\ndata: %s\n\n", event, payload)

	m.mu.RLock()
	defer m.mu.RUnlock()
	for c := range m.clients {
		select {
		case c.Events <- msg:
		default:
			// Client buffer full, skip
		}
	}
}

// ClientCount returns the number of connected clients.
func (m *Manager) ClientCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.clients)
}
