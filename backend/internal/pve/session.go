package pve

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"game-realtime-gm/backend/internal/auth"
	"game-realtime-gm/backend/internal/pve/store"
)

func SessionVersion(ctx context.Context, db *sql.DB, player int64) (int64, error) {
	var revoked time.Time
	err := db.QueryRowContext(ctx, "SELECT revoked_through FROM pve_session_revocations WHERE player_id=?", player).Scan(&revoked)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return revoked.UnixMilli(), err
}
func Authorize(ctx context.Context, db *sql.DB, claims *auth.Claims) error {
	if claims == nil || claims.SubjectType != auth.SubjectTypePlayer || claims.PlayerID <= 0 {
		return store.Forbidden
	}
	var status string
	err := db.QueryRowContext(ctx, "SELECT status FROM players WHERE id=?", claims.PlayerID).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return store.Forbidden
	}
	if err != nil {
		return err
	}
	if status != "normal" {
		return store.Forbidden
	}
	version, err := SessionVersion(ctx, db, claims.PlayerID)
	if err != nil {
		return err
	}
	if version != claims.SessionVersion {
		return store.Forbidden
	}
	return nil
}
