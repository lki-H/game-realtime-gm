package ws

import (
	"sort"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Client struct {
	PlayerID    int64
	Username    string
	Conn        *websocket.Conn
	ConnectedAt time.Time
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

	if exists && oldClient.Conn != client.Conn {
		return oldClient.Conn
	}

	return nil
}

func (m *Manager) Unregister(playerID int64, conn *websocket.Conn) {
	m.mu.Lock()
	defer m.mu.Unlock()

	currentClient, exists := m.clients[playerID]
	if !exists {
		return
	}

	if currentClient.Conn == conn {
		delete(m.clients, playerID)
	}
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
