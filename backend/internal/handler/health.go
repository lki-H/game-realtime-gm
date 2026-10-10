package handler

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

func Health(c *gin.Context) {
	c.JSON(200, gin.H{
		"code":    0,
		"message": "ok",
	})
}

func Readiness(lifecycle context.Context, db *sql.DB, cache *redis.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		probe, cancel := context.WithTimeout(c.Request.Context(), time.Second)
		defer cancel()
		unavailable := func() {
			c.JSON(http.StatusServiceUnavailable, gin.H{"code": 50301, "message": "service not ready"})
		}
		if lifecycle.Err() != nil || db == nil || cache == nil || db.PingContext(probe) != nil {
			unavailable()
			return
		}
		deadline, _ := probe.Deadline()
		remaining := time.Until(deadline)
		if remaining <= 0 || cache.WithTimeout(remaining).Ping(probe).Err() != nil || lifecycle.Err() != nil {
			unavailable()
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok"})
	}
}
