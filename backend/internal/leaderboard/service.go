package leaderboard

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"game-realtime-gm/backend/internal/model"

	"github.com/redis/go-redis/v9"
)

const (
	DefaultListLimit     int64 = 10
	MaxListLimit         int64 = 50
	maxLeaderboardScore  int64 = 1000
	maxLeaderboardMillis int64 = 9_999_999_999_999
)

var (
	ErrInvalidMissionID        = errors.New("invalid leaderboard mission_id")
	ErrInvalidListLimit        = errors.New("invalid leaderboard limit")
	ErrInvalidPage             = errors.New("invalid history page")
	ErrPlayerNotRanked         = errors.New("player not ranked")
	ErrInvalidSettlement       = errors.New("invalid leaderboard settlement")
	ErrInvalidLeaderboardEntry = errors.New("invalid leaderboard entry")
	ErrPlayerProfileNotFound   = errors.New("leaderboard player profile not found")
)

var upsertBestScoreScript = redis.NewScript(`
local old_member = redis.call("HGET", KEYS[2], ARGV[1])
local new_score = tonumber(ARGV[2])

if old_member then
    local old_score = tonumber(redis.call("ZSCORE", KEYS[1], old_member))
    if old_score and old_score > new_score then
        return 0
    end
    if old_score and old_score == new_score and old_member >= ARGV[3] then
        return 0
    end
    redis.call("ZREM", KEYS[1], old_member)
end

redis.call("ZADD", KEYS[1], new_score, ARGV[3])
redis.call("HSET", KEYS[2], ARGV[1], ARGV[3])
return 1
`)

type Entry struct {
	Rank            int64     `json:"rank"`
	MissionID       string    `json:"mission_id"`
	PlayerID        int64     `json:"player_id"`
	Username        string    `json:"username"`
	Nickname        string    `json:"nickname"`
	Score           int64     `json:"score"`
	AchievedAt      time.Time `json:"achieved_at"`
	MissionRecordID int64     `json:"mission_record_id"`
}

type HistoryItem struct {
	MissionRecordID   int64     `json:"mission_record_id"`
	MissionInstanceID string    `json:"mission_instance_id"`
	MissionID         string    `json:"mission_id"`
	SquadID           string    `json:"squad_id"`
	Score             int64     `json:"score"`
	CompletionSeconds int64     `json:"completion_seconds"`
	SettledAt         time.Time `json:"settled_at"`
}

type HistoryPage struct {
	Items    []HistoryItem `json:"items"`
	Page     int           `json:"page"`
	PageSize int           `json:"page_size"`
	Total    int64         `json:"total"`
}

type Service struct {
	db          *sql.DB
	redisClient *redis.Client
}

type memberMetadata struct {
	PlayerID        int64
	MissionRecordID int64
	AchievedAt      time.Time
}

type playerProfile struct {
	Username string
	Nickname string
}

func NewService(db *sql.DB, redisClient *redis.Client) *Service {
	return &Service{
		db:          db,
		redisClient: redisClient,
	}
}

func (s *Service) SyncSettlement(ctx context.Context, result *model.SettlementResult) (int64, error) {
	if result == nil ||
		result.Record.ID <= 0 ||
		result.Record.Status != "settled" ||
		!validMissionID(result.Record.MissionID) ||
		result.Record.Score < 0 ||
		result.Record.Score > maxLeaderboardScore ||
		result.Record.CreatedAt.IsZero() {
		return 0, ErrInvalidSettlement
	}

	playerSet := make(map[int64]struct{})
	for _, reward := range result.Rewards {
		if reward.PlayerID > 0 && reward.Status == "granted" {
			playerSet[reward.PlayerID] = struct{}{}
		}
	}
	if len(playerSet) == 0 {
		return 0, ErrInvalidSettlement
	}

	playerIDs := make([]int64, 0, len(playerSet))
	for playerID := range playerSet {
		playerIDs = append(playerIDs, playerID)
	}
	slices.Sort(playerIDs)

	var updatedCount int64
	for _, playerID := range playerIDs {
		member, err := formatLeaderboardMember(
			playerID,
			result.Record.ID,
			result.Record.CreatedAt,
		)
		if err != nil {
			return updatedCount, err
		}

		updated, err := upsertBestScoreScript.Run(
			ctx,
			s.redisClient,
			[]string{
				scoresKey(result.Record.MissionID),
				playersKey(result.Record.MissionID),
			},
			strconv.FormatInt(playerID, 10),
			strconv.FormatInt(result.Record.Score, 10),
			member,
		).Int64()
		if err != nil {
			return updatedCount, err
		}
		updatedCount += updated
	}

	return updatedCount, nil
}

func (s *Service) List(ctx context.Context, missionID string, limit int64) ([]Entry, error) {
	missionID = strings.TrimSpace(missionID)
	if !validMissionID(missionID) {
		return nil, ErrInvalidMissionID
	}
	if limit <= 0 || limit > MaxListLimit {
		return nil, ErrInvalidListLimit
	}

	values, err := s.redisClient.ZRevRangeWithScores(
		ctx,
		scoresKey(missionID),
		0,
		limit-1,
	).Result()
	if errors.Is(err, redis.Nil) {
		return []Entry{}, nil
	}
	if err != nil {
		return nil, err
	}

	entries := make([]Entry, 0, len(values))
	playerIDs := make([]int64, 0, len(values))
	for index, value := range values {
		member, ok := value.Member.(string)
		if !ok || math.Trunc(value.Score) != value.Score {
			return nil, ErrInvalidLeaderboardEntry
		}

		metadata, err := parseLeaderboardMember(member)
		if err != nil {
			return nil, err
		}
		playerIDs = append(playerIDs, metadata.PlayerID)
		entries = append(entries, Entry{
			Rank:            int64(index + 1),
			MissionID:       missionID,
			PlayerID:        metadata.PlayerID,
			Score:           int64(value.Score),
			AchievedAt:      metadata.AchievedAt,
			MissionRecordID: metadata.MissionRecordID,
		})
	}

	profiles, err := s.loadProfiles(ctx, playerIDs)
	if err != nil {
		return nil, err
	}
	for index := range entries {
		profile, exists := profiles[entries[index].PlayerID]
		if !exists {
			return nil, ErrPlayerProfileNotFound
		}
		entries[index].Username = profile.Username
		entries[index].Nickname = profile.Nickname
	}

	return entries, nil
}

func (s *Service) GetPlayer(ctx context.Context, missionID string, playerID int64) (*Entry, error) {
	missionID = strings.TrimSpace(missionID)
	if !validMissionID(missionID) {
		return nil, ErrInvalidMissionID
	}
	if playerID <= 0 {
		return nil, ErrInvalidLeaderboardEntry
	}

	member, err := s.redisClient.HGet(
		ctx,
		playersKey(missionID),
		strconv.FormatInt(playerID, 10),
	).Result()
	if errors.Is(err, redis.Nil) {
		return nil, ErrPlayerNotRanked
	}
	if err != nil {
		return nil, err
	}

	rank, err := s.redisClient.ZRevRank(ctx, scoresKey(missionID), member).Result()
	if errors.Is(err, redis.Nil) {
		return nil, ErrPlayerNotRanked
	}
	if err != nil {
		return nil, err
	}

	score, err := s.redisClient.ZScore(ctx, scoresKey(missionID), member).Result()
	if errors.Is(err, redis.Nil) {
		return nil, ErrPlayerNotRanked
	}
	if err != nil {
		return nil, err
	}
	if math.Trunc(score) != score {
		return nil, ErrInvalidLeaderboardEntry
	}

	metadata, err := parseLeaderboardMember(member)
	if err != nil {
		return nil, err
	}
	if metadata.PlayerID != playerID {
		return nil, ErrInvalidLeaderboardEntry
	}

	profile, err := s.loadProfile(ctx, playerID)
	if err != nil {
		return nil, err
	}

	return &Entry{
		Rank:            rank + 1,
		MissionID:       missionID,
		PlayerID:        playerID,
		Username:        profile.Username,
		Nickname:        profile.Nickname,
		Score:           int64(score),
		AchievedAt:      metadata.AchievedAt,
		MissionRecordID: metadata.MissionRecordID,
	}, nil
}

func (s *Service) ListHistory(ctx context.Context, playerID int64, page int, pageSize int) (*HistoryPage, error) {
	if playerID <= 0 || page <= 0 || pageSize <= 0 || pageSize > int(MaxListLimit) {
		return nil, ErrInvalidPage
	}

	var total int64
	err := s.db.QueryRowContext(
		ctx,
		`SELECT COUNT(DISTINCT m.id)
         FROM mission_records m
         JOIN reward_records r ON r.mission_record_id = m.id
         WHERE r.player_id = ?
           AND m.status = 'settled'
           AND r.status = 'granted'`,
		playerID,
	).Scan(&total)
	if err != nil {
		return nil, err
	}

	offset := (page - 1) * pageSize
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT DISTINCT
            m.id,
            m.mission_instance_id,
            m.mission_id,
            m.squad_id,
            m.score,
            m.completion_seconds,
            m.created_at
         FROM mission_records m
         JOIN reward_records r ON r.mission_record_id = m.id
         WHERE r.player_id = ?
           AND m.status = 'settled'
           AND r.status = 'granted'
         ORDER BY m.created_at DESC, m.id DESC
         LIMIT ? OFFSET ?`,
		playerID,
		pageSize,
		offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]HistoryItem, 0, pageSize)
	for rows.Next() {
		var item HistoryItem
		if err := rows.Scan(
			&item.MissionRecordID,
			&item.MissionInstanceID,
			&item.MissionID,
			&item.SquadID,
			&item.Score,
			&item.CompletionSeconds,
			&item.SettledAt,
		); err != nil {
			return nil, err
		}
		item.SettledAt = model.LegacyDatetime(item.SettledAt)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return &HistoryPage{
		Items:    items,
		Page:     page,
		PageSize: pageSize,
		Total:    total,
	}, nil
}

func (s *Service) loadProfiles(ctx context.Context, playerIDs []int64) (map[int64]playerProfile, error) {
	profiles := make(map[int64]playerProfile, len(playerIDs))
	if len(playerIDs) == 0 {
		return profiles, nil
	}

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(playerIDs)), ",")
	args := make([]any, 0, len(playerIDs))
	for _, playerID := range playerIDs {
		args = append(args, playerID)
	}

	rows, err := s.db.QueryContext(
		ctx,
		`SELECT id, username, nickname
         FROM players
         WHERE id IN (`+placeholders+`)`,
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var playerID int64
		var profile playerProfile
		if err := rows.Scan(&playerID, &profile.Username, &profile.Nickname); err != nil {
			return nil, err
		}
		profiles[playerID] = profile
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return profiles, nil
}

func (s *Service) loadProfile(ctx context.Context, playerID int64) (playerProfile, error) {
	var profile playerProfile
	err := s.db.QueryRowContext(
		ctx,
		`SELECT username, nickname
         FROM players
         WHERE id = ?`,
		playerID,
	).Scan(&profile.Username, &profile.Nickname)
	if errors.Is(err, sql.ErrNoRows) {
		return playerProfile{}, ErrPlayerProfileNotFound
	}
	return profile, err
}

func formatLeaderboardMember(playerID int64, missionRecordID int64, achievedAt time.Time) (string, error) {
	achievedMillis := achievedAt.UnixMilli()
	if playerID <= 0 || missionRecordID <= 0 || achievedMillis <= 0 || achievedMillis > maxLeaderboardMillis {
		return "", ErrInvalidLeaderboardEntry
	}

	inverseMillis := maxLeaderboardMillis - achievedMillis
	return fmt.Sprintf(
		"%013d|%020d|%020d",
		inverseMillis,
		playerID,
		missionRecordID,
	), nil
}

func parseLeaderboardMember(member string) (memberMetadata, error) {
	parts := strings.Split(member, "|")
	if len(parts) != 3 {
		return memberMetadata{}, ErrInvalidLeaderboardEntry
	}

	inverseMillis, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || inverseMillis < 0 || inverseMillis >= maxLeaderboardMillis {
		return memberMetadata{}, ErrInvalidLeaderboardEntry
	}
	playerID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || playerID <= 0 {
		return memberMetadata{}, ErrInvalidLeaderboardEntry
	}
	missionRecordID, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil || missionRecordID <= 0 {
		return memberMetadata{}, ErrInvalidLeaderboardEntry
	}

	achievedMillis := maxLeaderboardMillis - inverseMillis
	return memberMetadata{
		PlayerID:        playerID,
		MissionRecordID: missionRecordID,
		AchievedAt:      time.UnixMilli(achievedMillis).In(time.Local),
	}, nil
}

func validMissionID(value string) bool {
	if value == "" || len(value) > 64 {
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

func scoresKey(missionID string) string {
	return "leaderboard:{" + missionID + "}:scores"
}

func playersKey(missionID string) string {
	return "leaderboard:{" + missionID + "}:players"
}
