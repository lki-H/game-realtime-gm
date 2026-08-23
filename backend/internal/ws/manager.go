package ws

import (
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const clientWriteWait = 10 * time.Second

var ErrClientNotConnected = errors.New("websocket client not connected")

type Client struct {
	ConnectionID string
	PlayerID     int64
	Username     string
	Conn         *websocket.Conn
	WriteMu      *sync.Mutex
	ConnectedAt  time.Time
	LastPongAt   time.Time
}

func (c *Client) SendJSON(value any) error {
	if c == nil || c.Conn == nil || c.WriteMu == nil {
		return ErrClientNotConnected
	}

	c.WriteMu.Lock()
	defer c.WriteMu.Unlock()

	if err := c.Conn.SetWriteDeadline(time.Now().Add(clientWriteWait)); err != nil {
		return err
	}

	return c.Conn.WriteJSON(value)
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

func (m *Manager) Unregister(playerID int64, connectionID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	currentClient, exists := m.clients[playerID]
	if !exists {
		return false
	}
	if currentClient.ConnectionID != connectionID {
		return false
	}

	delete(m.clients, playerID)
	return true
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

func (m *Manager) IsConnected(playerID int64) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	_, exists := m.clients[playerID]
	return exists
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

func (m *Manager) SendToPlayer(playerID int64, value any) error {
	m.mu.RLock()
	client, exists := m.clients[playerID]
	m.mu.RUnlock()

	if !exists {
		return ErrClientNotConnected
	}

	return client.SendJSON(value)
}

func (m *Manager) BroadcastToPlayers(playerIDs []int64, value any) []int64 {
	failedPlayerIDs := make([]int64, 0)

	for _, playerID := range playerIDs {
		if err := m.SendToPlayer(playerID, value); err != nil {
			failedPlayerIDs = append(failedPlayerIDs, playerID)
		}
	}

	return failedPlayerIDs
}
