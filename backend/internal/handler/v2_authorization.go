package handler

import (
	"database/sql"
	"errors"

	"game-realtime-gm/backend/internal/middleware"
	"game-realtime-gm/backend/internal/pve/store"
	"github.com/gin-gonic/gin"
)

func requireOperatorTx(c *gin.Context, transaction *sql.Tx, adminID int64) error {
	var username, role string
	if err := transaction.QueryRowContext(c.Request.Context(), "SELECT username,role FROM admins WHERE id=? FOR SHARE", adminID).Scan(&username, &role); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return store.Forbidden
		}
		return err
	}
	if role != "operator" {
		return store.Forbidden
	}
	c.Set(middleware.ContextKeyAdminUsername, username)
	c.Set(middleware.ContextKeyAdminRole, role)
	return nil
}
