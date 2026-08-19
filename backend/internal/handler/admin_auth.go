package handler

import (
	"database/sql"
	"errors"
	"net/http"

	tokenauth "game-realtime-gm/backend/internal/auth"
	"game-realtime-gm/backend/internal/model"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

type AdminAuthHandler struct {
	db        *sql.DB
	jwtSecret string
}

func NewAdminAuthHandler(db *sql.DB, jwtSecret string) *AdminAuthHandler {
	return &AdminAuthHandler{
		db:        db,
		jwtSecret: jwtSecret,
	}
}

type adminLoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

func (h *AdminAuthHandler) Login(c *gin.Context) {
	var req adminLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40011,
			"message": "invalid request",
		})
		return
	}

	var admin model.Admin
	err := h.db.QueryRowContext(
		c.Request.Context(),
		`SELECT id, username, password_hash, display_name, role, created_at, updated_at
         FROM admins
		 WHERE username = ?`,
		req.Username,
	).Scan(
		&admin.ID,
		&admin.Username,
		&admin.PasswordHash,
		&admin.DisplayName,
		&admin.Role,
		&admin.CreatedAt,
		&admin.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    40111,
			"message": "username or password is wrong",
		})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50031,
			"message": "query admin failed",
		})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(req.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    40111,
			"message": "username or password is wrong",
		})
		return
	}

	token, err := tokenauth.GenerateAdminToken(h.jwtSecret, admin.ID, admin.Username, admin.Role)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50032,
			"message": "generate admin token failed",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "admin login success",
		"data": gin.H{
			"token": token,
			"admin": gin.H{
				"id":           admin.ID,
				"username":     admin.Username,
				"display_name": admin.DisplayName,
				"role":         admin.Role,
			},
		},
	})
}
