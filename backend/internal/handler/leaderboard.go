package handler

import (
	"errors"
	"net/http"

	"game-realtime-gm/backend/internal/leaderboard"

	"github.com/gin-gonic/gin"
)

type LeaderboardHandler struct {
	service *leaderboard.Service
}

func NewLeaderboardHandler(service *leaderboard.Service) *LeaderboardHandler {
	return &LeaderboardHandler{service: service}
}

func (h *LeaderboardHandler) List(c *gin.Context) {
	limit := int64(parsePositiveInt(
		c.DefaultQuery("limit", "10"),
		int(leaderboard.DefaultListLimit),
	))
	if limit > leaderboard.MaxListLimit {
		limit = leaderboard.MaxListLimit
	}

	missionID := c.Param("mission_id")
	items, err := h.service.List(c.Request.Context(), missionID, limit)
	if err != nil {
		writeLeaderboardError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "ok",
		"data": gin.H{
			"mission_id": missionID,
			"items":      items,
		},
	})
}

func (h *LeaderboardHandler) Me(c *gin.Context) {
	playerID, ok := requireCurrentPlayerID(c)
	if !ok {
		return
	}

	entry, err := h.service.GetPlayer(
		c.Request.Context(),
		c.Param("mission_id"),
		playerID,
	)
	if err != nil {
		writeLeaderboardError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "ok",
		"data":    entry,
	})
}

func (h *LeaderboardHandler) MyHistory(c *gin.Context) {
	playerID, ok := requireCurrentPlayerID(c)
	if !ok {
		return
	}

	page := parsePositiveInt(c.DefaultQuery("page", "1"), 1)
	pageSize := parsePositiveInt(c.DefaultQuery("page_size", "10"), 10)
	if pageSize > int(leaderboard.MaxListLimit) {
		pageSize = int(leaderboard.MaxListLimit)
	}

	result, err := h.service.ListHistory(
		c.Request.Context(),
		playerID,
		page,
		pageSize,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50091,
			"message": "query mission history failed",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "ok",
		"data":    result,
	})
}

func writeLeaderboardError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, leaderboard.ErrInvalidMissionID):
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40090,
			"message": "invalid leaderboard mission_id",
		})
	case errors.Is(err, leaderboard.ErrPlayerNotRanked):
		c.JSON(http.StatusNotFound, gin.H{
			"code":    40490,
			"message": "player not ranked",
		})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50090,
			"message": "query leaderboard failed",
		})
	}
}
