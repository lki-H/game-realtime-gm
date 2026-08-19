package middleware

import (
	"net/http"
	"strings"

	tokenauth "game-realtime-gm/backend/internal/auth"

	"github.com/gin-gonic/gin"
)

const (
	ContextKeyPlayerID      = "player_id"
	ContextKeyUsername      = "username"
	ContextKeyAdminID       = "admin_id"
	ContextKeyAdminUsername = "admin_username"
	ContextKeyAdminRole     = "admin_role"
)

func Auth(jwtSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenString, ok := bearerTokenFromHeader(c, 40102, 40103)
		if !ok {
			return
		}

		claims, err := tokenauth.ParseToken(jwtSecret, tokenString)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":    40104,
				"message": "invalid token",
			})
			c.Abort()
			return
		}

		if claims.SubjectType != tokenauth.SubjectTypePlayer || claims.PlayerID <= 0 {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    40301,
				"message": "player permission required",
			})
			c.Abort()
			return
		}

		c.Set(ContextKeyPlayerID, claims.PlayerID)
		c.Set(ContextKeyUsername, claims.Username)
		c.Next()
	}
}

func AdminAuth(jwtSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenString, ok := bearerTokenFromHeader(c, 40112, 40113)
		if !ok {
			return
		}

		claims, err := tokenauth.ParseToken(jwtSecret, tokenString)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":    40114,
				"message": "invalid token",
			})
			c.Abort()
			return
		}

		if claims.SubjectType != tokenauth.SubjectTypeAdmin || claims.AdminID <= 0 {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    40311,
				"message": "admin permission required",
			})
			c.Abort()
			return
		}

		c.Set(ContextKeyAdminID, claims.AdminID)
		c.Set(ContextKeyAdminUsername, claims.Username)
		c.Set(ContextKeyAdminRole, claims.Role)
		c.Next()
	}
}

func CurrentPlayerID(c *gin.Context) (int64, bool) {
	playerIDValue, exists := c.Get(ContextKeyPlayerID)
	if !exists {
		return 0, false
	}

	playerID, ok := playerIDValue.(int64)
	if !ok {
		return 0, false
	}

	return playerID, true
}

func CurrentAdminID(c *gin.Context) (int64, bool) {
	adminIDValue, exists := c.Get(ContextKeyAdminID)
	if !exists {
		return 0, false
	}

	adminID, ok := adminIDValue.(int64)
	if !ok {
		return 0, false
	}

	return adminID, true
}

func CurrentAdminUsername(c *gin.Context) string {
	usernameValue, exists := c.Get(ContextKeyAdminUsername)
	if !exists {
		return ""
	}

	username, ok := usernameValue.(string)
	if !ok {
		return ""
	}

	return username
}

func CurrentAdminRole(c *gin.Context) string {
	roleValue, exists := c.Get(ContextKeyAdminRole)
	if !exists {
		return ""
	}

	role, ok := roleValue.(string)
	if !ok {
		return ""
	}

	return role
}

func bearerTokenFromHeader(c *gin.Context, missingCode int, invalidCode int) (string, bool) {
	header := c.GetHeader("Authorization")
	if header == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    missingCode,
			"message": "authorization header missing",
		})
		c.Abort()
		return "", false
	}

	tokenString := strings.TrimSpace(strings.TrimPrefix(header, "Bearer"))
	tokenString = strings.TrimSpace(tokenString)
	if tokenString == "" || tokenString == header {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    invalidCode,
			"message": "invalid authorization header",
		})
		c.Abort()
		return "", false
	}

	return tokenString, true
}
