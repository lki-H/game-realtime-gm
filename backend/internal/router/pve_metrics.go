package router

import (
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"net/http"
	"strings"

	"game-realtime-gm/backend/internal/handler"
	"github.com/gin-gonic/gin"
)

func NewPVEMetricsHandler(db *sql.DB, token string) http.Handler {
	router := gin.New()
	router.Use(gin.Recovery())
	expected := sha256.Sum256([]byte(token))
	router.GET("/metrics", func(c *gin.Context) {
		provided := sha256.Sum256([]byte(strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")))
		if len(token) < 24 || !strings.HasPrefix(c.GetHeader("Authorization"), "Bearer ") || subtle.ConstantTimeCompare(expected[:], provided[:]) != 1 {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		handler.V2Metrics(db)(c)
	})
	return router
}
