package mission

import (
	"crypto/rand"
	"encoding/hex"

	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

type Status string

const (
	StatusWaiting  Status = "waiting"
	StatusReady    Status = "ready"
	StatusRunning  Status = "running"
	StatusFinished Status = "finished"
	StatusCanceled Status = "canceled"
)

var (
	ErrMissionIDRequired      = errors.New("mission_id required")
	ErrMissionHasNoPlayers    = errors.New("mission has no players")
	ErrMissionNotFound        = errors.New("mission instance not found")
	ErrSquadAlreadyInMission  = errors.New("squad already has active mission")
	ErrInvalidStateTransition = errors.New("invalid mission state transition")
)

type Instance struct {
	ID         string     `json:"id"`
	MissionID  string     `json:"mission_id"`
	SquadID    string     `json:"squad_id"`
	PlayerIDs  []int64    `json:"player_ids"`
	Status     Status     `json:"status"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	ReadyAt    *time.Time `json:"ready_at,omitempty"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

type Stats struct {
	Total    int `json:"total"`
	Waiting  int `json:"waiting"`
	Ready    int `json:"ready"`
	Running  int `json:"running"`
	Finished int `json:"finished"`
	Canceled int `json:"canceled"`
}

type Manager struct {
	mu            sync.RWMutex
	instances     map[string]*Instance
	squadMission  map[string]string
	playerMission map[int64]string
}

func NewManager() *Manager {
	return &Manager{
		instances:     make(map[string]*Instance),
		squadMission:  make(map[string]string),
		playerMission: make(map[int64]string),
	}
}

func (m *Manager) Create(missionID string, squadID string, playerIDs []int64) (*Instance, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	missionID = strings.TrimSpace(missionID)
	if missionID == "" {
		return nil, ErrMissionIDRequired
	}
	if len(playerIDs) == 0 {
		return nil, ErrMissionHasNoPlayers
	}

	if currentID, exists := m.squadMission[squadID]; exists {
		current := m.instances[currentID]
		if current != nil && !isTerminal(current.Status) {
			return nil, ErrSquadAlreadyInMission
		}
	}

	now := time.Now()
	instanceID, err := newInstanceID(now)
	if err != nil {
		return nil, err
	}

	instance := &Instance{
		ID:        instanceID,
		MissionID: missionID,
		SquadID:   squadID,
		PlayerIDs: append([]int64(nil), playerIDs...),
		Status:    StatusWaiting,
		CreatedAt: now,
		UpdatedAt: now,
	}

	m.instances[instanceID] = instance
	m.squadMission[squadID] = instanceID
	for _, playerID := range playerIDs {
		m.playerMission[playerID] = instanceID
	}

	return cloneInstance(instance), nil
}

func (m *Manager) Transition(instanceID string, target Status) (*Instance, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	instance, exists := m.instances[instanceID]
	if !exists {
		return nil, ErrMissionNotFound
	}
	if !canTransition(instance.Status, target) {
		return nil, ErrInvalidStateTransition
	}

	now := time.Now()
	instance.Status = target
	instance.UpdatedAt = now

	switch target {
	case StatusReady:
		instance.ReadyAt = timePointer(now)
	case StatusRunning:
		instance.StartedAt = timePointer(now)
	case StatusFinished:
		instance.FinishedAt = timePointer(now)
	}

	return cloneInstance(instance), nil
}

func (m *Manager) GetByPlayer(playerID int64) (*Instance, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	instanceID, exists := m.playerMission[playerID]
	if !exists {
		return nil, ErrMissionNotFound
	}
	instance, exists := m.instances[instanceID]
	if !exists {
		return nil, ErrMissionNotFound
	}

	return cloneInstance(instance), nil
}

func (m *Manager) GetBySquad(squadID string) (*Instance, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	instanceID, exists := m.squadMission[squadID]
	if !exists {
		return nil, ErrMissionNotFound
	}
	instance, exists := m.instances[instanceID]
	if !exists {
		return nil, ErrMissionNotFound
	}

	return cloneInstance(instance), nil
}

func (m *Manager) GetByID(instanceID string) (*Instance, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	instance, exists := m.instances[instanceID]
	if !exists {
		return nil, ErrMissionNotFound
	}
	return cloneInstance(instance), nil
}

func (m *Manager) Stats() Stats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := Stats{Total: len(m.instances)}
	for _, instance := range m.instances {
		switch instance.Status {
		case StatusWaiting:
			stats.Waiting++
		case StatusReady:
			stats.Ready++
		case StatusRunning:
			stats.Running++
		case StatusFinished:
			stats.Finished++
		case StatusCanceled:
			stats.Canceled++
		}
	}
	return stats
}

func newInstanceID(now time.Time) (string, error) {
	randomBytes := make([]byte, 8)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", err
	}
	return fmt.Sprintf(
		"mission_instance_%d_%s",
		now.UnixNano(),
		hex.EncodeToString(randomBytes),
	), nil
}

func canTransition(current Status, target Status) bool {
	switch current {
	case StatusWaiting:
		return target == StatusReady || target == StatusCanceled
	case StatusReady:
		return target == StatusRunning
	case StatusRunning:
		return target == StatusFinished
	default:
		return false
	}
}

func isTerminal(status Status) bool {
	return status == StatusFinished || status == StatusCanceled
}

func cloneInstance(instance *Instance) *Instance {
	if instance == nil {
		return nil
	}

	return &Instance{
		ID:         instance.ID,
		MissionID:  instance.MissionID,
		SquadID:    instance.SquadID,
		PlayerIDs:  append([]int64(nil), instance.PlayerIDs...),
		Status:     instance.Status,
		CreatedAt:  instance.CreatedAt,
		UpdatedAt:  instance.UpdatedAt,
		ReadyAt:    cloneTimePointer(instance.ReadyAt),
		StartedAt:  cloneTimePointer(instance.StartedAt),
		FinishedAt: cloneTimePointer(instance.FinishedAt),
	}
}

func timePointer(value time.Time) *time.Time {
	copyValue := value
	return &copyValue
}

func cloneTimePointer(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}
