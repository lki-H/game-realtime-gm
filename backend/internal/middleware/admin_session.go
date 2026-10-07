package middleware

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

func AdminSession(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		adminID, ok := CurrentAdminID(c)
		if !ok {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": 40311, "message": "admin permission required"})
			return
		}
		var username, role string
		err := db.QueryRowContext(c.Request.Context(), "SELECT username,role FROM admins WHERE id=?", adminID).Scan(&username, &role)
		if errors.Is(err, sql.ErrNoRows) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": 40311, "message": "admin account unavailable"})
			return
		}
		if err != nil {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"code": 50370, "message": "admin session verification unavailable"})
			return
		}
		c.Set(ContextKeyAdminUsername, username)
		c.Set(ContextKeyAdminRole, role)
		c.Next()
	}
}
