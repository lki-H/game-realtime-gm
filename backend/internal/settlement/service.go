package settlement

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"slices"
	"strings"
	"time"

	"game-realtime-gm/backend/internal/mission"
	"game-realtime-gm/backend/internal/model"

	"github.com/go-sql-driver/mysql"
)

const (
	missionRecordStatusSettled = "settled"
	rewardRecordStatusGranted  = "granted"
	rewardTypeSoftCurrency     = "soft_currency"
	rewardAmountSoftCurrency   = int64(100)
	ledgerReasonMissionReward  = "mission_reward"
)

const missionRecordSelect = `SELECT
    id,
    mission_instance_id,
    mission_id,
    squad_id,
    submitted_by_player_id,
    nonce,
    idempotency_key,
    status,
    completion_seconds,
    score,
    created_at,
    updated_at
FROM mission_records`

var (
	ErrMissionInstanceIDRequired = errors.New("mission_instance_id required")
	ErrNonceRequired             = errors.New("settlement nonce required")
	ErrInvalidNonce              = errors.New("invalid settlement nonce")
	ErrIdempotencyKeyRequired    = errors.New("idempotency_key required")
	ErrInvalidIdempotencyKey     = errors.New("invalid idempotency_key")
	ErrIdempotencyKeyConflict    = errors.New("idempotency_key belongs to another mission")
	ErrMissionNotFound           = errors.New("settlement mission not found")
	ErrMissionSquadChanged       = errors.New("settlement mission squad changed")
	ErrPlayerNotInMission        = errors.New("player not in mission")
	ErrMissionNotFinished        = errors.New("mission not finished")
	ErrInvalidMissionTimes       = errors.New("invalid mission times")
	ErrMissionHasNoPlayers       = errors.New("mission has no players")
	ErrNonceReplayed             = errors.New("settlement nonce replayed")
	ErrPlayerAssetNotFound       = errors.New("player asset not found")
	ErrInvalidAssetBalance       = errors.New("invalid asset balance")
	ErrAssetUpdateFailed         = errors.New("asset update failed")
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

func (s *Service) Create(
	ctx context.Context,
	playerID int64,
	squadID string,
	missionInstanceID string,
	idempotencyKey string,
	nonce string,
) (*model.SettlementResult, bool, error) {
	missionInstanceID = strings.TrimSpace(missionInstanceID)
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	nonce = strings.TrimSpace(nonce)

	if missionInstanceID == "" {
		return nil, false, ErrMissionInstanceIDRequired
	}
	if idempotencyKey == "" {
		return nil, false, ErrIdempotencyKeyRequired
	}
	if !validRequestKey(idempotencyKey) {
		return nil, false, ErrInvalidIdempotencyKey
	}
	if nonce == "" {
		return nil, false, ErrNonceRequired
	}
	if !validRequestKey(nonce) {
		return nil, false, ErrInvalidNonce
	}

	missionState, err := s.missionManager.GetByID(missionInstanceID)
	if err != nil {
		return nil, false, ErrMissionNotFound
	}
	if missionState.SquadID != squadID {
		return nil, false, ErrMissionSquadChanged
	}
	if !containsPlayer(missionState.PlayerIDs, playerID) {
		return nil, false, ErrPlayerNotInMission
	}
	if missionState.Status != mission.StatusFinished || missionState.StartedAt == nil || missionState.FinishedAt == nil {
		return nil, false, ErrMissionNotFinished
	}
	if len(missionState.PlayerIDs) == 0 {
		return nil, false, ErrMissionHasNoPlayers
	}

	existing, found, err := s.findExistingResult(ctx, missionInstanceID, idempotencyKey)
	if err != nil {
		return nil, false, err
	}
	if found {
		return existing, false, nil
	}

	duration := missionState.FinishedAt.Sub(*missionState.StartedAt)
	if duration < 0 {
		return nil, false, ErrInvalidMissionTimes
	}
	completionSeconds := int64(math.Ceil(duration.Seconds()))
	score := calculateScore(completionSeconds)
	now := time.Now()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, err
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
            idempotency_key,
            status,
            completion_seconds,
            score,
            created_at,
            updated_at
        ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		missionState.ID,
		missionState.MissionID,
		missionState.SquadID,
		playerID,
		nonce,
		idempotencyKey,
		missionRecordStatusSettled,
		completionSeconds,
		score,
		now,
		now,
	)
	if err != nil {
		if isDuplicateKey(err) {
			_ = tx.Rollback()
			return s.resolveDuplicate(
				ctx,
				playerID,
				missionInstanceID,
				idempotencyKey,
				nonce,
				err,
			)
		}
		return nil, false, err
	}

	missionRecordID, err := insertResult.LastInsertId()
	if err != nil {
		return nil, false, err
	}

	rewardPlayerIDs := append([]int64(nil), missionState.PlayerIDs...)
	slices.Sort(rewardPlayerIDs)

	rewards := make([]model.RewardRecord, 0, len(rewardPlayerIDs))
	for _, rewardPlayerID := range rewardPlayerIDs {
		var balanceBefore int64
		err := tx.QueryRowContext(
			ctx,
			`SELECT soft_currency
             FROM player_assets
             WHERE player_id = ?
             FOR UPDATE`,
			rewardPlayerID,
		).Scan(&balanceBefore)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, false, ErrPlayerAssetNotFound
		}
		if err != nil {
			return nil, false, err
		}

		balanceAfter, err := nextAssetBalance(balanceBefore, rewardAmountSoftCurrency)
		if err != nil {
			return nil, false, err
		}

		rewardResult, err := tx.ExecContext(
			ctx,
			`INSERT INTO reward_records (
                mission_record_id,
                mission_instance_id,
                player_id,
                reward_type,
                amount,
                status,
                granted_at,
                created_at,
                updated_at
            ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			missionRecordID,
			missionState.ID,
			rewardPlayerID,
			rewardTypeSoftCurrency,
			rewardAmountSoftCurrency,
			rewardRecordStatusGranted,
			now,
			now,
			now,
		)
		if err != nil {
			return nil, false, err
		}

		rewardRecordID, err := rewardResult.LastInsertId()
		if err != nil {
			return nil, false, err
		}

		assetUpdateResult, err := tx.ExecContext(
			ctx,
			`UPDATE player_assets
             SET soft_currency = ?, updated_at = ?
             WHERE player_id = ?`,
			balanceAfter,
			now,
			rewardPlayerID,
		)
		if err != nil {
			return nil, false, err
		}
		rowsAffected, err := assetUpdateResult.RowsAffected()
		if err != nil {
			return nil, false, err
		}
		if rowsAffected != 1 {
			return nil, false, ErrAssetUpdateFailed
		}

		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO asset_ledger (
                player_id,
                mission_record_id,
                reward_record_id,
                mission_instance_id,
                asset_type,
                delta,
                balance_before,
                balance_after,
                reason,
                created_at
            ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			rewardPlayerID,
			missionRecordID,
			rewardRecordID,
			missionState.ID,
			rewardTypeSoftCurrency,
			rewardAmountSoftCurrency,
			balanceBefore,
			balanceAfter,
			ledgerReasonMissionReward,
			now,
		); err != nil {
			return nil, false, err
		}

		grantedAt := now
		rewards = append(rewards, model.RewardRecord{
			ID:                rewardRecordID,
			MissionRecordID:   missionRecordID,
			MissionInstanceID: missionState.ID,
			PlayerID:          rewardPlayerID,
			RewardType:        rewardTypeSoftCurrency,
			Amount:            rewardAmountSoftCurrency,
			Status:            rewardRecordStatusGranted,
			GrantedAt:         &grantedAt,
			CreatedAt:         now,
			UpdatedAt:         now,
		})
	}

	if err := tx.Commit(); err != nil {
		return nil, false, err
	}

	return &model.SettlementResult{
		Record: model.MissionRecord{
			ID:                  missionRecordID,
			MissionInstanceID:   missionState.ID,
			MissionID:           missionState.MissionID,
			SquadID:             missionState.SquadID,
			SubmittedByPlayerID: playerID,
			Nonce:               nonce,
			IdempotencyKey:      idempotencyKey,
			Status:              missionRecordStatusSettled,
			CompletionSeconds:   completionSeconds,
			Score:               score,
			CreatedAt:           now,
			UpdatedAt:           now,
		},
		Rewards: rewards,
	}, true, nil
}

func (s *Service) findExistingResult(
	ctx context.Context,
	missionInstanceID string,
	idempotencyKey string,
) (*model.SettlementResult, bool, error) {
	resultByKey, found, err := s.loadResult(ctx, "idempotency_key", idempotencyKey)
	if err != nil {
		return nil, false, err
	}
	if found {
		if resultByKey.Record.MissionInstanceID != missionInstanceID {
			return nil, false, ErrIdempotencyKeyConflict
		}
		return resultByKey, true, nil
	}

	return s.loadResult(ctx, "mission_instance_id", missionInstanceID)
}

func (s *Service) resolveDuplicate(
	ctx context.Context,
	playerID int64,
	missionInstanceID string,
	idempotencyKey string,
	nonce string,
	originalErr error,
) (*model.SettlementResult, bool, error) {
	existing, found, err := s.findExistingResult(ctx, missionInstanceID, idempotencyKey)
	if err != nil {
		return nil, false, err
	}
	if found {
		return existing, false, nil
	}

	var nonceMissionInstanceID string
	err = s.db.QueryRowContext(
		ctx,
		`SELECT mission_instance_id
         FROM mission_records
         WHERE submitted_by_player_id = ? AND nonce = ?
         LIMIT 1`,
		playerID,
		nonce,
	).Scan(&nonceMissionInstanceID)
	if err == nil {
		return nil, false, ErrNonceReplayed
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, false, err
	}

	return nil, false, originalErr
}

func (s *Service) loadResult(
	ctx context.Context,
	lookupColumn string,
	lookupValue string,
) (*model.SettlementResult, bool, error) {
	var query string
	switch lookupColumn {
	case "idempotency_key":
		query = missionRecordSelect + " WHERE idempotency_key = ? LIMIT 1"
	case "mission_instance_id":
		query = missionRecordSelect + " WHERE mission_instance_id = ? LIMIT 1"
	default:
		return nil, false, errors.New("unsupported settlement lookup")
	}

	var record model.MissionRecord
	err := s.db.QueryRowContext(ctx, query, lookupValue).Scan(
		&record.ID,
		&record.MissionInstanceID,
		&record.MissionID,
		&record.SquadID,
		&record.SubmittedByPlayerID,
		&record.Nonce,
		&record.IdempotencyKey,
		&record.Status,
		&record.CompletionSeconds,
		&record.Score,
		&record.CreatedAt,
		&record.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}

	rewards, err := s.loadRewards(ctx, record.ID)
	if err != nil {
		return nil, false, err
	}

	return &model.SettlementResult{
		Record:  record,
		Rewards: rewards,
	}, true, nil
}

func (s *Service) loadRewards(ctx context.Context, missionRecordID int64) ([]model.RewardRecord, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT
            id,
            mission_record_id,
            mission_instance_id,
            player_id,
            reward_type,
            amount,
            status,
            granted_at,
            created_at,
            updated_at
         FROM reward_records
         WHERE mission_record_id = ?
         ORDER BY player_id, id`,
		missionRecordID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	rewards := make([]model.RewardRecord, 0)
	for rows.Next() {
		var reward model.RewardRecord
		if err := rows.Scan(
			&reward.ID,
			&reward.MissionRecordID,
			&reward.MissionInstanceID,
			&reward.PlayerID,
			&reward.RewardType,
			&reward.Amount,
			&reward.Status,
			&reward.GrantedAt,
			&reward.CreatedAt,
			&reward.UpdatedAt,
		); err != nil {
			return nil, err
		}
		rewards = append(rewards, reward)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return rewards, nil
}

func validRequestKey(value string) bool {
	if len(value) < 16 || len(value) > 64 {
		return false
	}
	for _, character := range value {
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

func nextAssetBalance(current int64, delta int64) (int64, error) {
	if current < 0 || delta <= 0 || current > math.MaxInt64-delta {
		return 0, ErrInvalidAssetBalance
	}
	return current + delta, nil
}

func isDuplicateKey(err error) bool {
	var mysqlErr *mysql.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == 1062
}
