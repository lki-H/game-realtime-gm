package handler

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

const onlineTTL = 2 * time.Minute

type OnlineHandler struct {
	redisClient *redis.Client
}

func NewOnlineHandler(redisClient *redis.Client) *OnlineHandler {
	return &OnlineHandler{redisClient: redisClient}
}

func (h *OnlineHandler) Heartbeat(c *gin.Context) {
	playerID, ok := requireCurrentPlayerID(c)
	if !ok {
		return
	}

	key := onlinePlayerKey(playerID)
	if err := h.redisClient.Set(c.Request.Context(), key, "1", onlineTTL).Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50020,
			"message": "update online status failed",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "heartbeat success",
		"data": gin.H{
			"online":      true,
			"ttl_seconds": int(onlineTTL.Seconds()),
		},
	})
}

func (h *OnlineHandler) Status(c *gin.Context) {
	playerID, ok := requireCurrentPlayerID(c)
	if !ok {
		return
	}

	key := onlinePlayerKey(playerID)
	exists, err := h.redisClient.Exists(c.Request.Context(), key).Result()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50021,
			"message": "query online status failed",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "ok",
		"data": gin.H{
			"online": exists > 0,
		},
	})
}

func onlinePlayerKey(playerID int64) string {
	return fmt.Sprintf("online:player:%d", playerID)
}
