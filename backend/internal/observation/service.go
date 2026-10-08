package observation

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"game-realtime-gm/backend/internal/matchmaking"
	"game-realtime-gm/backend/internal/mission"
	"game-realtime-gm/backend/internal/model"
	"game-realtime-gm/backend/internal/squad"
	realtimews "game-realtime-gm/backend/internal/ws"
)

var (
	ErrPlayerNotFound      = errors.New("observation player not found")
	ErrPlayerAssetNotFound = errors.New("observation player asset not found")
	ErrInvalidPage         = errors.New("invalid observation page")
)

const settlementItemSelect = `SELECT
    m.id,
    m.mission_instance_id,
    m.mission_id,
    m.squad_id,
    m.submitted_by_player_id,
    COALESCE(p.username, ''),
    m.status,
    m.completion_seconds,
    m.score,
    (
        SELECT COUNT(*)
        FROM reward_records reward_count
        WHERE reward_count.mission_record_id = m.id
          AND reward_count.status = 'granted'
    ),
    m.created_at
FROM mission_records m
LEFT JOIN players p ON p.id = m.submitted_by_player_id`

type Summary struct {
	OnlineConnections int           `json:"online_connections"`
	Squads            squad.Stats   `json:"squads"`
	MatchmakingQueued int64         `json:"matchmaking_queued"`
	Missions          mission.Stats `json:"missions"`
	Settlements       int64         `json:"settlements"`
	ObservedAt        time.Time     `json:"observed_at"`
}

type SettlementItem struct {
	ID                  int64     `json:"id"`
	MissionInstanceID   string    `json:"mission_instance_id"`
	MissionID           string    `json:"mission_id"`
	SquadID             string    `json:"squad_id"`
	SubmittedByPlayerID int64     `json:"submitted_by_player_id"`
	SubmittedByUsername string    `json:"submitted_by_username"`
	Status              string    `json:"status"`
	CompletionSeconds   int64     `json:"completion_seconds"`
	Score               int64     `json:"score"`
	RewardCount         int64     `json:"reward_count"`
	CreatedAt           time.Time `json:"created_at"`
}

type SettlementPage struct {
	Items    []SettlementItem `json:"items"`
	Page     int              `json:"page"`
	PageSize int              `json:"page_size"`
	Total    int64            `json:"total"`
}

type PlayerState struct {
	PlayerID         int64               `json:"player_id"`
	Username         string              `json:"username"`
	Nickname         string              `json:"nickname"`
	Status           string              `json:"status"`
	SoftCurrency     int64               `json:"soft_currency"`
	Online           bool                `json:"online"`
	Squad            *squad.Squad        `json:"squad,omitempty"`
	Mission          *mission.Instance   `json:"mission,omitempty"`
	Matching         bool                `json:"matching"`
	Matchmaking      *matchmaking.Ticket `json:"matchmaking,omitempty"`
	LatestSettlement *SettlementItem     `json:"latest_settlement,omitempty"`
	ObservedAt       time.Time           `json:"observed_at"`
}

type Service struct {
	db                 *sql.DB
	wsManager          *realtimews.Manager
	squadManager       *squad.Manager
	missionManager     *mission.Manager
	matchmakingManager *matchmaking.Manager
}

type scanner interface {
	Scan(dest ...any) error
}

func NewService(
	db *sql.DB,
	wsManager *realtimews.Manager,
	squadManager *squad.Manager,
	missionManager *mission.Manager,
	matchmakingManager *matchmaking.Manager,
) *Service {
	return &Service{
		db:                 db,
		wsManager:          wsManager,
		squadManager:       squadManager,
		missionManager:     missionManager,
		matchmakingManager: matchmakingManager,
	}
}

func (s *Service) Summary(ctx context.Context) (*Summary, error) {
	var queued int64
	if s.matchmakingManager != nil {
		var err error
		queued, err = s.matchmakingManager.CountQueued(ctx)
		if err != nil {
			return nil, err
		}
	}

	var settlements int64
	if err := s.db.QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM mission_records WHERE status = 'settled'`,
	).Scan(&settlements); err != nil {
		return nil, err
	}

	summary := &Summary{
		MatchmakingQueued: queued,
		Settlements:       settlements,
		ObservedAt:        time.Now(),
	}
	if s.wsManager != nil {
		summary.OnlineConnections = s.wsManager.Count()
		summary.Squads = s.squadManager.Stats()
		summary.Missions = s.missionManager.Stats()
	}
	return summary, nil
}

func (s *Service) Player(ctx context.Context, playerID int64) (*PlayerState, error) {
	state := &PlayerState{PlayerID: playerID}
	var softCurrency sql.NullInt64
	err := s.db.QueryRowContext(
		ctx,
		`SELECT p.username, p.nickname, p.status, a.soft_currency
         FROM players p
         LEFT JOIN player_assets a ON a.player_id = p.id
         WHERE p.id = ?`,
		playerID,
	).Scan(
		&state.Username,
		&state.Nickname,
		&state.Status,
		&softCurrency,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrPlayerNotFound
	}
	if err != nil {
		return nil, err
	}
	if !softCurrency.Valid {
		return nil, ErrPlayerAssetNotFound
	}
	state.SoftCurrency = softCurrency.Int64
	if s.wsManager != nil {
		state.Online = s.wsManager.IsConnected(playerID)

		currentSquad, err := s.squadManager.GetByPlayer(playerID)
		if err == nil {
			state.Squad = currentSquad
		} else if !errors.Is(err, squad.ErrPlayerNotInSquad) && !errors.Is(err, squad.ErrSquadNotFound) {
			return nil, err
		}

		currentMission, err := s.missionManager.GetByPlayer(playerID)
		if err == nil {
			state.Mission = currentMission
		} else if !errors.Is(err, mission.ErrMissionNotFound) {
			return nil, err
		}

		ticket, err := s.matchmakingManager.GetByPlayer(ctx, playerID)
		if err == nil {
			state.Matchmaking = ticket
			state.Matching = ticket.Status == matchmaking.StatusQueued
		} else if !errors.Is(err, matchmaking.ErrTicketNotFound) {
			return nil, err
		}
	}

	latestSettlement, err := s.latestSettlement(ctx, playerID)
	if err != nil {
		return nil, err
	}
	state.LatestSettlement = latestSettlement
	state.ObservedAt = time.Now()

	return state, nil
}

func (s *Service) ListSettlements(
	ctx context.Context,
	page int,
	pageSize int,
	missionID string,
	playerID int64,
) (*SettlementPage, error) {
	if page <= 0 || pageSize <= 0 || pageSize > 50 || playerID < 0 {
		return nil, ErrInvalidPage
	}

	whereSQL, args := settlementWhere(missionID, playerID)
	var total int64
	if err := s.db.QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM mission_records m `+whereSQL,
		args...,
	).Scan(&total); err != nil {
		return nil, err
	}

	offset := (page - 1) * pageSize
	listArgs := append(append([]any{}, args...), pageSize, offset)
	rows, err := s.db.QueryContext(
		ctx,
		settlementItemSelect+` `+whereSQL+`
         ORDER BY m.created_at DESC, m.id DESC
         LIMIT ? OFFSET ?`,
		listArgs...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]SettlementItem, 0, pageSize)
	for rows.Next() {
		item, err := scanSettlement(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return &SettlementPage{
		Items:    items,
		Page:     page,
		PageSize: pageSize,
		Total:    total,
	}, nil
}

func (s *Service) latestSettlement(ctx context.Context, playerID int64) (*SettlementItem, error) {
	whereSQL, args := settlementWhere("", playerID)
	row := s.db.QueryRowContext(
		ctx,
		settlementItemSelect+` `+whereSQL+`
         ORDER BY m.created_at DESC, m.id DESC
         LIMIT 1`,
		args...,
	)

	item, err := scanSettlement(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return item, err
}

func settlementWhere(missionID string, playerID int64) (string, []any) {
	parts := []string{"m.status = 'settled'"}
	args := make([]any, 0, 2)

	missionID = strings.TrimSpace(missionID)
	if missionID != "" {
		parts = append(parts, "m.mission_id = ?")
		args = append(args, missionID)
	}
	if playerID > 0 {
		parts = append(parts, `EXISTS (
            SELECT 1
            FROM reward_records participant_reward
            WHERE participant_reward.mission_record_id = m.id
              AND participant_reward.player_id = ?
              AND participant_reward.status = 'granted'
        )`)
		args = append(args, playerID)
	}

	return "WHERE " + strings.Join(parts, " AND "), args
}

func scanSettlement(source scanner) (*SettlementItem, error) {
	var item SettlementItem
	err := source.Scan(
		&item.ID,
		&item.MissionInstanceID,
		&item.MissionID,
		&item.SquadID,
		&item.SubmittedByPlayerID,
		&item.SubmittedByUsername,
		&item.Status,
		&item.CompletionSeconds,
		&item.Score,
		&item.RewardCount,
		&item.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	item.CreatedAt = model.LegacyDatetime(item.CreatedAt)
	return &item, nil
}
