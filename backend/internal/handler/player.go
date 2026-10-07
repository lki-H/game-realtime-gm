package handler

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"game-realtime-gm/backend/internal/middleware"
	"game-realtime-gm/backend/internal/model"

	"github.com/gin-gonic/gin"
)

type PlayerHandler struct {
	db *sql.DB
}

type updateNicknameRequest struct {
	Nickname string `json:"nickname" binding:"required,max=64"`
}

func NewPlayerHandler(db *sql.DB) *PlayerHandler {
	return &PlayerHandler{db: db}
}

func (h *PlayerHandler) Me(c *gin.Context) {
	playerID, ok := requireCurrentPlayerID(c)
	if !ok {
		return
	}

	player, err := h.findPlayerByID(c, playerID)
	if errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    40401,
			"message": "player not found",
		})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50010,
			"message": "query player failed",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "ok",
		"data":    player,
	})
}

func (h *PlayerHandler) UpdateNickname(c *gin.Context) {
	playerID, ok := requireCurrentPlayerID(c)
	if !ok {
		return
	}

	var req updateNicknameRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "invalid request",
		})
		return
	}

	nickname := strings.TrimSpace(req.Nickname)
	if nickname == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40003,
			"message": "nickname cannot be empty",
		})
		return
	}

	_, err := h.db.ExecContext(
		c.Request.Context(),
		`UPDATE players
         SET nickname = ?, updated_at = CURRENT_TIMESTAMP(3)
         WHERE id = ?`,
		nickname,
		playerID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50011,
			"message": "update nickname failed",
		})
		return
	}

	player, err := h.findPlayerByID(c, playerID)
	if errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    40401,
			"message": "player not found",
		})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50011,
			"message": "update nickname failed",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "update nickname success",
		"data":    player,
	})
}

func (h *PlayerHandler) GetByID(c *gin.Context) {
	playerID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || playerID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40004,
			"message": "invalid player id",
		})
		return
	}

	player, err := h.findPlayerByID(c, playerID)
	if errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    40401,
			"message": "player not found",
		})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50012,
			"message": "query player failed",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "ok",
		"data":    player,
	})
}

func (h *PlayerHandler) List(c *gin.Context) {
	page := parsePositiveInt(c.DefaultQuery("page", "1"), 1)
	pageSize := parsePositiveInt(c.DefaultQuery("page_size", "10"), 10)
	if pageSize > 50 {
		pageSize = 50
	}

	keyword := strings.TrimSpace(c.Query("keyword"))
	offset := (page - 1) * pageSize

	whereSQL := ""
	args := []any{}
	if keyword != "" {
		whereSQL = "WHERE username LIKE ? OR nickname LIKE ?"
		keywordPattern := "%" + keyword + "%"
		args = append(args, keywordPattern, keywordPattern)
	}

	var total int64
	countSQL := `SELECT COUNT(*) FROM players ` + whereSQL
	if err := h.db.QueryRowContext(c.Request.Context(), countSQL, args...).Scan(&total); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50013,
			"message": "count players failed",
		})
		return
	}

	listArgs := append(append([]any{}, args...), pageSize, offset)

	rows, err := h.db.QueryContext(
		c.Request.Context(),
		`SELECT id, username, nickname, created_at, updated_at, status, banned_reason, banned_at, banned_by_admin_id
         FROM players
         `+whereSQL+`
         ORDER BY id DESC
		 LIMIT ? OFFSET ?`,
		listArgs...,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50014,
			"message": "query player list failed",
		})
		return
	}
	defer rows.Close()

	players := make([]model.Player, 0)
	for rows.Next() {
		var player model.Player
		if err := rows.Scan(
			&player.ID,
			&player.Username,
			&player.Nickname,
			&player.CreatedAt,
			&player.UpdatedAt,
			&player.Status,
			&player.BannedReason,
			&player.BannedAt,
			&player.BannedByAdminID,
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    50015,
				"message": "scan player failed",
			})
			return
		}
		players = append(players, player)
	}

	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50016,
			"message": "read player rows failed",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "ok",
		"data": gin.H{
			"items":     players,
			"page":      page,
			"page_size": pageSize,
			"total":     total,
		},
	})
}

func (h *PlayerHandler) findPlayerByID(c *gin.Context, playerID int64) (model.Player, error) {
	var player model.Player
	err := h.db.QueryRowContext(
		c.Request.Context(),
		`SELECT id, username, nickname, created_at, updated_at, status, banned_reason, banned_at, banned_by_admin_id
         FROM players
		 WHERE id = ?`,
		playerID,
	).Scan(
		&player.ID,
		&player.Username,
		&player.Nickname,
		&player.CreatedAt,
		&player.UpdatedAt,
		&player.Status,
		&player.BannedReason,
		&player.BannedAt,
		&player.BannedByAdminID,
	)
	return player, err
}

func parsePositiveInt(value string, defaultValue int) int {
	parsedValue, err := strconv.Atoi(value)
	if err != nil || parsedValue <= 0 {
		return defaultValue
	}
	return parsedValue
}

func requireCurrentPlayerID(c *gin.Context) (int64, bool) {
	playerID, ok := middleware.CurrentPlayerID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    40105,
			"message": "player identity missing",
		})
		return 0, false
	}

	return playerID, true
}
