package ws

import (
	"sort"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Client struct {
	ConnectionID string
	PlayerID     int64
	Username     string
	Conn         *websocket.Conn
	ConnectedAt  time.Time
	LastPongAt   time.Time
}

type Manager struct {
	mu      sync.RWMutex
	clients map[int64]*Client
}

func NewManager() *Manager {
	return &Manager{
		clients: make(map[int64]*Client),
	}
}

func (m *Manager) Register(client *Client) *websocket.Conn {
	m.mu.Lock()
	defer m.mu.Unlock()

	oldClient, exists := m.clients[client.PlayerID]
	m.clients[client.PlayerID] = client

	if exists && oldClient.ConnectionID != client.ConnectionID {
		return oldClient.Conn
	}

	return nil
}

func (m *Manager) Unregister(playerID int64, connectionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	currentClient, exists := m.clients[playerID]
	if !exists {
		return
	}

	if currentClient.ConnectionID == connectionID {
		delete(m.clients, playerID)
	}
}

func (m *Manager) UpdateLastPong(playerID int64, connectionID string, lastPongAt time.Time) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	currentClient, exists := m.clients[playerID]
	if !exists {
		return false
	}

	if currentClient.ConnectionID != connectionID {
		return false
	}

	currentClient.LastPongAt = lastPongAt
	return true
}

func (m *Manager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return len(m.clients)
}

func (m *Manager) PlayerIDs() []int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	playerIDs := make([]int64, 0, len(m.clients))
	for playerID := range m.clients {
		playerIDs = append(playerIDs, playerID)
	}

	sort.Slice(playerIDs, func(i, j int) bool {
		return playerIDs[i] < playerIDs[j]
	})

	return playerIDs
}
