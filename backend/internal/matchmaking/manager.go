package matchmaking

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

type Status string

const (
	StatusQueued   Status = "queued"
	StatusCanceled Status = "canceled"
	StatusTimeout  Status = "timeout"
)

const (
	DefaultTicketTimeout = 30 * time.Second
	ticketRetention      = 10 * time.Minute
	cleanupBatchSize     = 100
	timeoutIndexKey      = "matchmaking:timeouts"
)

var (
	ErrMissionIDRequired = errors.New("mission_id required")
	ErrRoleRequired      = errors.New("matchmaking role required")
	ErrInvalidMissionID  = errors.New("invalid matchmaking mission_id")
	ErrInvalidRole       = errors.New("invalid matchmaking role")
	ErrAlreadyQueued     = errors.New("player already queued")
	ErrTicketNotFound    = errors.New("matchmaking ticket not found")
	ErrTicketNotQueued   = errors.New("matchmaking ticket not queued")
	ErrTicketExpired     = errors.New("matchmaking ticket expired")
	ErrInvalidTicketData = errors.New("invalid matchmaking ticket data")
)

type Ticket struct {
	ID            string    `json:"id"`
	MissionID     string    `json:"mission_id"`
	PlayerID      int64     `json:"player_id"`
	SquadID       string    `json:"squad_id,omitempty"`
	Role          string    `json:"role"`
	Status        Status    `json:"status"`
	QueuePosition int64     `json:"queue_position,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	TimeoutAt     time.Time `json:"timeout_at"`
}

type Manager struct {
	redisClient *redis.Client
	mu          sync.Mutex
}

func NewManager(redisClient *redis.Client) *Manager {
	return &Manager{redisClient: redisClient}
}

func (m *Manager) Enqueue(ctx context.Context, missionID string, playerID int64, squadID string, role string) (*Ticket, error) {
	missionID = strings.TrimSpace(missionID)
	role = strings.TrimSpace(role)
	if missionID == "" {
		return nil, ErrMissionIDRequired
	}
	if role == "" {
		return nil, ErrRoleRequired
	}
	if !validIdentifier(missionID, 64) {
		return nil, ErrInvalidMissionID
	}
	if !validIdentifier(role, 32) {
		return nil, ErrInvalidRole
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	currentID, err := m.redisClient.Get(ctx, playerKey(playerID)).Result()
	if err != nil && err != redis.Nil {
		return nil, err
	}
	if err == nil {
		current, ticketErr := m.getByIDLocked(ctx, currentID)
		if ticketErr != nil && !errors.Is(ticketErr, ErrTicketNotFound) {
			return nil, ticketErr
		}
		if ticketErr == nil && current.Status == StatusQueued {
			if now.Before(current.TimeoutAt) {
				return nil, ErrAlreadyQueued
			}
			if _, markErr := m.markTerminalLocked(ctx, current, StatusTimeout, now); markErr != nil {
				return nil, markErr
			}
		}
	}

	ticketID, err := newTicketID(playerID, now)
	if err != nil {
		return nil, err
	}
	ticket := &Ticket{
		ID:        ticketID,
		MissionID: missionID,
		PlayerID:  playerID,
		SquadID:   squadID,
		Role:      role,
		Status:    StatusQueued,
		CreatedAt: now,
		UpdatedAt: now,
		TimeoutAt: now.Add(DefaultTicketTimeout),
	}

	_, err = m.redisClient.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.HSet(ctx, ticketKey(ticket.ID), ticketFields(ticket))
		pipe.Expire(ctx, ticketKey(ticket.ID), ticketRetention)
		pipe.Set(ctx, playerKey(playerID), ticket.ID, ticketRetention)
		pipe.ZAdd(ctx, queueKey(ticket.MissionID), redis.Z{
			Score:  float64(ticket.CreatedAt.UnixMilli()),
			Member: ticket.ID,
		})
		pipe.ZAdd(ctx, timeoutIndexKey, redis.Z{
			Score:  float64(ticket.TimeoutAt.UnixMilli()),
			Member: timeoutMember(ticket),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	ticket.QueuePosition = m.queuePositionLocked(ctx, ticket)
	return cloneTicket(ticket), nil
}

func (m *Manager) Cancel(ctx context.Context, playerID int64) (*Ticket, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	ticket, err := m.getByPlayerLocked(ctx, playerID)
	if err != nil {
		return nil, err
	}
	if ticket.Status != StatusQueued {
		return nil, ErrTicketNotQueued
	}

	now := time.Now()
	if !now.Before(ticket.TimeoutAt) {
		if _, err := m.markTerminalLocked(ctx, ticket, StatusTimeout, now); err != nil {
			return nil, err
		}
		return nil, ErrTicketExpired
	}

	return m.markTerminalLocked(ctx, ticket, StatusCanceled, now)
}

func (m *Manager) GetByPlayer(ctx context.Context, playerID int64) (*Ticket, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	ticket, err := m.getByPlayerLocked(ctx, playerID)
	if err != nil {
		return nil, err
	}
	if ticket.Status == StatusQueued && !time.Now().Before(ticket.TimeoutAt) {
		return m.markTerminalLocked(ctx, ticket, StatusTimeout, time.Now())
	}

	ticket.QueuePosition = m.queuePositionLocked(ctx, ticket)
	return cloneTicket(ticket), nil
}

func (m *Manager) CountQueued(ctx context.Context) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	minimumTimeout := "(" + strconv.FormatInt(time.Now().UnixMilli(), 10)
	return m.redisClient.ZCount(
		ctx,
		timeoutIndexKey,
		minimumTimeout,
		"+inf",
	).Result()
}

func (m *Manager) CleanupExpired(ctx context.Context) ([]*Ticket, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	members, err := m.redisClient.ZRangeByScore(ctx, timeoutIndexKey, &redis.ZRangeBy{
		Min:    "-inf",
		Max:    strconv.FormatInt(now.UnixMilli(), 10),
		Offset: 0,
		Count:  cleanupBatchSize,
	}).Result()
	if err != nil {
		return nil, err
	}

	expired := make([]*Ticket, 0, len(members))
	for _, member := range members {
		missionID, ticketID, ok := parseTimeoutMember(member)
		if !ok {
			m.redisClient.ZRem(ctx, timeoutIndexKey, member)
			continue
		}

		ticket, ticketErr := m.getByIDLocked(ctx, ticketID)
		if errors.Is(ticketErr, ErrTicketNotFound) {
			m.redisClient.ZRem(ctx, timeoutIndexKey, member)
			m.redisClient.ZRem(ctx, queueKey(missionID), ticketID)
			continue
		}
		if ticketErr != nil {
			return nil, ticketErr
		}
		if ticket.Status != StatusQueued {
			m.redisClient.ZRem(ctx, timeoutIndexKey, member)
			m.redisClient.ZRem(ctx, queueKey(ticket.MissionID), ticket.ID)
			continue
		}

		updated, markErr := m.markTerminalLocked(ctx, ticket, StatusTimeout, now)
		if markErr != nil {
			return nil, markErr
		}
		expired = append(expired, updated)
	}

	return expired, nil
}

func (m *Manager) RunTimeoutLoop(ctx context.Context, interval time.Duration, onTimeout func(*Ticket), onError func(error)) {
	if interval <= 0 {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			tickets, err := m.CleanupExpired(ctx)
			if err != nil {
				if onError != nil {
					onError(err)
				}
				continue
			}
			if onTimeout != nil {
				for _, ticket := range tickets {
					onTimeout(ticket)
				}
			}
		}
	}
}

func (m *Manager) getByPlayerLocked(ctx context.Context, playerID int64) (*Ticket, error) {
	ticketID, err := m.redisClient.Get(ctx, playerKey(playerID)).Result()
	if err == redis.Nil {
		return nil, ErrTicketNotFound
	}
	if err != nil {
		return nil, err
	}

	ticket, err := m.getByIDLocked(ctx, ticketID)
	if errors.Is(err, ErrTicketNotFound) {
		m.redisClient.Del(ctx, playerKey(playerID))
	}
	return ticket, err
}

func (m *Manager) getByIDLocked(ctx context.Context, ticketID string) (*Ticket, error) {
	values, err := m.redisClient.HGetAll(ctx, ticketKey(ticketID)).Result()
	if err != nil {
		return nil, err
	}
	if len(values) == 0 {
		return nil, ErrTicketNotFound
	}

	ticket, err := parseTicket(values)
	if err != nil {
		return nil, err
	}
	ticket.QueuePosition = m.queuePositionLocked(ctx, ticket)
	return ticket, nil
}

func (m *Manager) markTerminalLocked(ctx context.Context, ticket *Ticket, target Status, now time.Time) (*Ticket, error) {
	if !canTransition(ticket.Status, target) {
		return nil, ErrTicketNotQueued
	}

	ticket.Status = target
	ticket.UpdatedAt = now
	ticket.QueuePosition = 0
	_, err := m.redisClient.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.HSet(ctx, ticketKey(ticket.ID), map[string]any{
			"status":     string(ticket.Status),
			"updated_at": ticket.UpdatedAt.Format(time.RFC3339Nano),
		})
		pipe.Expire(ctx, ticketKey(ticket.ID), ticketRetention)
		pipe.Expire(ctx, playerKey(ticket.PlayerID), ticketRetention)
		pipe.ZRem(ctx, queueKey(ticket.MissionID), ticket.ID)
		pipe.ZRem(ctx, timeoutIndexKey, timeoutMember(ticket))
		return nil
	})
	if err != nil {
		return nil, err
	}

	return cloneTicket(ticket), nil
}

func (m *Manager) queuePositionLocked(ctx context.Context, ticket *Ticket) int64 {
	if ticket == nil || ticket.Status != StatusQueued {
		return 0
	}
	position, err := m.redisClient.ZRank(ctx, queueKey(ticket.MissionID), ticket.ID).Result()
	if err != nil {
		return 0
	}
	return position + 1
}

func ticketFields(ticket *Ticket) map[string]any {
	return map[string]any{
		"id":         ticket.ID,
		"mission_id": ticket.MissionID,
		"player_id":  ticket.PlayerID,
		"squad_id":   ticket.SquadID,
		"role":       ticket.Role,
		"status":     string(ticket.Status),
		"created_at": ticket.CreatedAt.Format(time.RFC3339Nano),
		"updated_at": ticket.UpdatedAt.Format(time.RFC3339Nano),
		"timeout_at": ticket.TimeoutAt.Format(time.RFC3339Nano),
	}
}

func parseTicket(values map[string]string) (*Ticket, error) {
	playerID, err := strconv.ParseInt(values["player_id"], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("%w: player_id", ErrInvalidTicketData)
	}
	createdAt, err := time.Parse(time.RFC3339Nano, values["created_at"])
	if err != nil {
		return nil, fmt.Errorf("%w: created_at", ErrInvalidTicketData)
	}
	updatedAt, err := time.Parse(time.RFC3339Nano, values["updated_at"])
	if err != nil {
		return nil, fmt.Errorf("%w: updated_at", ErrInvalidTicketData)
	}
	timeoutAt, err := time.Parse(time.RFC3339Nano, values["timeout_at"])
	if err != nil {
		return nil, fmt.Errorf("%w: timeout_at", ErrInvalidTicketData)
	}
	if values["id"] == "" || values["mission_id"] == "" || values["role"] == "" {
		return nil, ErrInvalidTicketData
	}

	return &Ticket{
		ID:        values["id"],
		MissionID: values["mission_id"],
		PlayerID:  playerID,
		SquadID:   values["squad_id"],
		Role:      values["role"],
		Status:    Status(values["status"]),
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
		TimeoutAt: timeoutAt,
	}, nil
}

func canTransition(current Status, target Status) bool {
	return current == StatusQueued && (target == StatusCanceled || target == StatusTimeout)
}

func validIdentifier(value string, maxLength int) bool {
	if value == "" || len(value) > maxLength {
		return false
	}
	for _, character := range value {
		if character >= 'a' && character <= 'z' ||
			character >= 'A' && character <= 'Z' ||
			character >= '0' && character <= '9' ||
			character == '_' || character == '-' || character == '.' {
			continue
		}
		return false
	}
	return true
}

func newTicketID(playerID int64, now time.Time) (string, error) {
	randomBytes := make([]byte, 8)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", err
	}
	return fmt.Sprintf("ticket_%d_%d_%s", playerID, now.UnixNano(), hex.EncodeToString(randomBytes)), nil
}

func queueKey(missionID string) string {
	return "matchmaking:queue:" + missionID
}

func ticketKey(ticketID string) string {
	return "matchmaking:ticket:" + ticketID
}

func playerKey(playerID int64) string {
	return "matchmaking:player:" + strconv.FormatInt(playerID, 10)
}

func timeoutMember(ticket *Ticket) string {
	return ticket.MissionID + "|" + ticket.ID
}

func parseTimeoutMember(value string) (string, string, bool) {
	parts := strings.SplitN(value, "|", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func cloneTicket(ticket *Ticket) *Ticket {
	if ticket == nil {
		return nil
	}
	copyTicket := *ticket
	return &copyTicket
}
