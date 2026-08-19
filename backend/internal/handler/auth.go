package handler

import (
	"database/sql"
	"errors"
	"net/http"

	tokenauth "game-realtime-gm/backend/internal/auth"
	"game-realtime-gm/backend/internal/model"

	"github.com/gin-gonic/gin"
	"github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/bcrypt"
)

type AuthHandler struct {
	db        *sql.DB
	jwtSecret string
}

func NewAuthHandler(db *sql.DB, jwtSecret string) *AuthHandler {
	return &AuthHandler{
		db:        db,
		jwtSecret: jwtSecret,
	}
}

type registerRequest struct {
	Username string `json:"username" binding:"required,min=3,max=64"`
	Password string `json:"password" binding:"required,min=6,max=72"`
	Nickname string `json:"nickname" binding:"required,max=64"`
}

type loginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

func (h *AuthHandler) Register(c *gin.Context) {
	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40001,
			"message": "invalid request",
		})
		return
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50001,
			"message": "generate password hash failed",
		})
		return
	}

	result, err := h.db.ExecContext(
		c.Request.Context(),
		`INSERT INTO players (username, password_hash, nickname, banned_reason)
		 VALUES (?, ?, ?, '')`,
		req.Username,
		string(passwordHash),
		req.Nickname,
	)
	if err != nil {
		var mysqlErr *mysql.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
			c.JSON(http.StatusConflict, gin.H{
				"code":    40901,
				"message": "username already exists",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50002,
			"message": "create player failed",
		})
		return
	}

	playerID, err := result.LastInsertId()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50002,
			"message": "create player failed",
		})
		return
	}

	var player model.Player
	err = h.db.QueryRowContext(
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
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50002,
			"message": "create player failed",
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"code":    0,
		"message": "register success",
		"data":    player,
	})
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40001,
			"message": "invalid request",
		})
		return
	}

	var player model.Player
	err := h.db.QueryRowContext(
		c.Request.Context(),
		`SELECT id, username, password_hash, nickname, created_at, updated_at, status, banned_reason, banned_at, banned_by_admin_id
		FROM players
		WHERE username = ?`,
		req.Username,
	).Scan(
		&player.ID,
		&player.Username,
		&player.PasswordHash,
		&player.Nickname,
		&player.CreatedAt,
		&player.UpdatedAt,
		&player.Status,
		&player.BannedReason,
		&player.BannedAt,
		&player.BannedByAdminID,
	)
	if errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    40101,
			"message": "username or password is wrong",
		})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50003,
			"message": "query player failed",
		})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(player.PasswordHash), []byte(req.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    40101,
			"message": "username or password is wrong",
		})
		return
	}

	if player.Status == "banned" {
		c.JSON(http.StatusForbidden, gin.H{
			"code":    40321,
			"message": "player is banned",
			"data": gin.H{
				"reason": player.BannedReason,
			},
		})
		return
	}

	token, err := tokenauth.GenerateToken(h.jwtSecret, player.ID, player.Username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50004,
			"message": "generate token failed",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "login success",
		"data": gin.H{
			"token": token,
			"player": gin.H{
				"id":       player.ID,
				"username": player.Username,
				"nickname": player.Nickname,
			},
		},
	})
}
