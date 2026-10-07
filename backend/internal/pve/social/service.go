package social

import (
	"context"
	"database/sql"
	"strings"

	"game-realtime-gm/backend/internal/pve/store"
)

type Service struct {
	db            *sql.DB
	RetentionDays int
}
type Player struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Nickname string `json:"nickname"`
}

func NewService(db *sql.DB) *Service { return &Service{db: db, RetentionDays: 90} }
func (service *Service) SearchPlayers(ctx context.Context, requester int64, keyword string, limit int) ([]Player, error) {
	keyword = strings.TrimSpace(keyword)
	if requester <= 0 || keyword == "" || len(keyword) > 128 {
		return nil, store.Invalid
	}
	if limit <= 0 || limit > 20 {
		limit = 20
	}
	pattern := "%" + keyword + "%"
	rows, err := service.db.QueryContext(ctx, "SELECT id,username,nickname FROM players WHERE id<>? AND (username LIKE ? OR nickname LIKE ?) AND status='normal' AND NOT EXISTS(SELECT 1 FROM pve_social_blocks b WHERE (b.blocker_id=? AND b.blocked_id=players.id) OR (b.blocked_id=? AND b.blocker_id=players.id)) ORDER BY id LIMIT ?", requester, pattern, pattern, requester, requester, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Player{}
	for rows.Next() {
		var player Player
		if err := rows.Scan(&player.ID, &player.Username, &player.Nickname); err != nil {
			return nil, err
		}
		result = append(result, player)
	}
	return result, rows.Err()
}
func canonical(first, second int64) (int64, int64) {
	if first < second {
		return first, second
	}
	return second, first
}
