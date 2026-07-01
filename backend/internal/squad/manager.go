package squad

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

const MaxMembers = 4

var (
	ErrPlayerAlreadyInSquad = errors.New("player already in squad")
	ErrPlayerNotInSquad     = errors.New("player not in squad")
	ErrSquadNotFound        = errors.New("squad not found")
	ErrSquadFull            = errors.New("squad is full")
)

type Member struct {
	PlayerID int64     `json:"player_id"`
	Username string    `json:"username"`
	Ready    bool      `json:"ready"`
	JoinedAt time.Time `json:"joined_at"`
}

type Squad struct {
	ID         string    `json:"id"`
	LeaderID   int64     `json:"leader_id"`
	MaxMembers int       `json:"max_members"`
	Members    []Member  `json:"members"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type Manager struct {
	mu          sync.RWMutex
	nextID      int64
	squads      map[string]*Squad
	playerSquad map[int64]string
}

func NewManager() *Manager {
	return &Manager{
		nextID:      1,
		squads:      make(map[string]*Squad),
		playerSquad: make(map[int64]string),
	}
}

func (m *Manager) Create(playerID int64, username string) (*Squad, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.playerSquad[playerID]; exists {
		return nil, ErrPlayerAlreadyInSquad
	}

	now := time.Now()
	squadID := fmt.Sprintf("squad_%d", m.nextID)
	m.nextID++

	s := &Squad{
		ID:         squadID,
		LeaderID:   playerID,
		MaxMembers: MaxMembers,
		Members: []Member{
			{
				PlayerID: playerID,
				Username: username,
				Ready:    true,
				JoinedAt: now,
			},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	m.squads[squadID] = s
	m.playerSquad[playerID] = squadID

	return cloneSquad(s), nil
}

func (m *Manager) Join(squadID string, playerID int64, username string) (*Squad, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.playerSquad[playerID]; exists {
		return nil, ErrPlayerAlreadyInSquad
	}

	s, exists := m.squads[squadID]
	if !exists {
		return nil, ErrSquadNotFound
	}

	if len(s.Members) >= s.MaxMembers {
		return nil, ErrSquadFull
	}

	now := time.Now()
	s.Members = append(s.Members, Member{
		PlayerID: playerID,
		Username: username,
		Ready:    false,
		JoinedAt: now,
	})
	s.UpdatedAt = now
	m.playerSquad[playerID] = squadID

	return cloneSquad(s), nil
}

func (m *Manager) Leave(playerID int64) (*Squad, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	squadID, exists := m.playerSquad[playerID]
	if !exists {
		return nil, false, ErrPlayerNotInSquad
	}

	s, exists := m.squads[squadID]
	if !exists {
		delete(m.playerSquad, playerID)
		return nil, false, ErrSquadNotFound
	}

	nextMembers := make([]Member, 0, len(s.Members)-1)
	for _, member := range s.Members {
		if member.PlayerID != playerID {
			nextMembers = append(nextMembers, member)
		}
	}

	delete(m.playerSquad, playerID)

	if len(nextMembers) == 0 {
		delete(m.squads, squadID)
		return nil, true, nil
	}

	s.Members = nextMembers
	if s.LeaderID == playerID {
		s.LeaderID = nextMembers[0].PlayerID
	}
	s.UpdatedAt = time.Now()

	return cloneSquad(s), false, nil
}

func (m *Manager) SetReady(playerID int64, ready bool) (*Squad, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	squadID, exists := m.playerSquad[playerID]
	if !exists {
		return nil, ErrPlayerNotInSquad
	}

	s, exists := m.squads[squadID]
	if !exists {
		delete(m.playerSquad, playerID)
		return nil, ErrSquadNotFound
	}

	for i := range s.Members {
		if s.Members[i].PlayerID == playerID {
			s.Members[i].Ready = ready
			s.UpdatedAt = time.Now()
			return cloneSquad(s), nil
		}
	}

	delete(m.playerSquad, playerID)
	return nil, ErrPlayerNotInSquad
}

func (m *Manager) GetByPlayer(playerID int64) (*Squad, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	squadID, exists := m.playerSquad[playerID]
	if !exists {
		return nil, ErrPlayerNotInSquad
	}

	s, exists := m.squads[squadID]
	if !exists {
		return nil, ErrSquadNotFound
	}

	return cloneSquad(s), nil
}

func cloneSquad(s *Squad) *Squad {
	if s == nil {
		return nil
	}

	members := make([]Member, len(s.Members))
	copy(members, s.Members)

	return &Squad{
		ID:         s.ID,
		LeaderID:   s.LeaderID,
		MaxMembers: s.MaxMembers,
		Members:    members,
		CreatedAt:  s.CreatedAt,
		UpdatedAt:  s.UpdatedAt,
	}
}
