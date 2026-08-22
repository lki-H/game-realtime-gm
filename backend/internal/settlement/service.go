package settlement

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"strings"
	"time"

	"game-realtime-gm/backend/internal/mission"
	"game-realtime-gm/backend/internal/model"

	"github.com/go-sql-driver/mysql"
)

const (
	missionRecordStatusRecorded = "recorded"
	rewardRecordStatusPending   = "pending"
	rewardTypeSoftCurrency      = "soft_currency"
	rewardAmountSoftCurrency    = int64(100)
)

var (
	ErrMissionInstanceIDRequired = errors.New("mission_instance_id required")
	ErrNonceRequired             = errors.New("settlement nonce required")
	ErrInvalidNonce              = errors.New("invalid settlement nonce")
	ErrMissionNotFound           = errors.New("settlement mission not found")
	ErrMissionSquadChanged       = errors.New("settlement mission squad changed")
	ErrPlayerNotInMission        = errors.New("player not in mission")
	ErrMissionNotFinished        = errors.New("mission not finished")
	ErrInvalidMissionTimes       = errors.New("invalid mission times")
	ErrMissionHasNoPlayers       = errors.New("mission has no players")
	ErrNonceReplayed             = errors.New("settlement nonce replayed")
)

type Service struct {
	db             *sql.DB
	missionManager *mission.Manager
}

func NewService(db *sql.DB, missionManager *mission.Manager) *Service {
	return &Service{
		db:             db,
		missionManager: missionManager,
	}
}

func (s *Service) Create(ctx context.Context, playerID int64, squadID string, missionInstanceID string, nonce string) (*model.SettlementResult, error) {
	missionInstanceID = strings.TrimSpace(missionInstanceID)
	nonce = strings.TrimSpace(nonce)

	if missionInstanceID == "" {
		return nil, ErrMissionInstanceIDRequired
	}
	if nonce == "" {
		return nil, ErrNonceRequired
	}
	if !validNonce(nonce) {
		return nil, ErrInvalidNonce
	}

	missionState, err := s.missionManager.GetByID(missionInstanceID)
	if err != nil {
		return nil, ErrMissionNotFound
	}
	if missionState.SquadID != squadID {
		return nil, ErrMissionSquadChanged
	}
	if !containsPlayer(missionState.PlayerIDs, playerID) {
		return nil, ErrPlayerNotInMission
	}
	if missionState.Status != mission.StatusFinished || missionState.StartedAt == nil || missionState.FinishedAt == nil {
		return nil, ErrMissionNotFinished
	}
	if len(missionState.PlayerIDs) == 0 {
		return nil, ErrMissionHasNoPlayers
	}

	duration := missionState.FinishedAt.Sub(*missionState.StartedAt)
	if duration < 0 {
		return nil, ErrInvalidMissionTimes
	}
	completionSeconds := int64(math.Ceil(duration.Seconds()))
	score := calculateScore(completionSeconds)
	now := time.Now()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	insertResult, err := tx.ExecContext(
		ctx,
		`INSERT INTO mission_records (
            mission_instance_id,
            mission_id,
            squad_id,
            submitted_by_player_id,
            nonce,
            status,
            completion_seconds,
            score,
            created_at,
            updated_at
        ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		missionState.ID,
		missionState.MissionID,
		missionState.SquadID,
		playerID,
		nonce,
		missionRecordStatusRecorded,
		completionSeconds,
		score,
		now,
		now,
	)
	if err != nil {
		var mysqlErr *mysql.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
			return nil, ErrNonceReplayed
		}
		return nil, err
	}

	missionRecordID, err := insertResult.LastInsertId()
	if err != nil {
		return nil, err
	}

	rewards := make([]model.RewardRecord, 0, len(missionState.PlayerIDs))
	for _, rewardPlayerID := range missionState.PlayerIDs {
		rewardResult, err := tx.ExecContext(
			ctx,
			`INSERT INTO reward_records (
                mission_record_id,
                mission_instance_id,
                player_id,
                reward_type,
                amount,
                status,
                created_at
            ) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			missionRecordID,
			missionState.ID,
			rewardPlayerID,
			rewardTypeSoftCurrency,
			rewardAmountSoftCurrency,
			rewardRecordStatusPending,
			now,
		)
		if err != nil {
			return nil, err
		}

		rewardRecordID, err := rewardResult.LastInsertId()
		if err != nil {
			return nil, err
		}
		rewards = append(rewards, model.RewardRecord{
			ID:                rewardRecordID,
			MissionRecordID:   missionRecordID,
			MissionInstanceID: missionState.ID,
			PlayerID:          rewardPlayerID,
			RewardType:        rewardTypeSoftCurrency,
			Amount:            rewardAmountSoftCurrency,
			Status:            rewardRecordStatusPending,
			CreatedAt:         now,
		})
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &model.SettlementResult{
		Record: model.MissionRecord{
			ID:                  missionRecordID,
			MissionInstanceID:   missionState.ID,
			MissionID:           missionState.MissionID,
			SquadID:             missionState.SquadID,
			SubmittedByPlayerID: playerID,
			Nonce:               nonce,
			Status:              missionRecordStatusRecorded,
			CompletionSeconds:   completionSeconds,
			Score:               score,
			CreatedAt:           now,
			UpdatedAt:           now,
		},
		Rewards: rewards,
	}, nil
}

func validNonce(nonce string) bool {
	if len(nonce) < 16 || len(nonce) > 64 {
		return false
	}
	for _, character := range nonce {
		if character >= 'a' && character <= 'z' ||
			character >= 'A' && character <= 'Z' ||
			character >= '0' && character <= '9' ||
			character == '_' ||
			character == '-' {
			continue
		}
		return false
	}
	return true
}

func containsPlayer(playerIDs []int64, playerID int64) bool {
	for _, currentPlayerID := range playerIDs {
		if currentPlayerID == playerID {
			return true
		}
	}
	return false
}

func calculateScore(completionSeconds int64) int64 {
	score := int64(1000) - completionSeconds
	if score < 0 {
		return 0
	}
	return score
}
