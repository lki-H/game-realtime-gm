package handler

import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"game-realtime-gm/backend/internal/middleware"
	"game-realtime-gm/backend/internal/observation"

	"github.com/gin-gonic/gin"
)

type ObservationHandler struct {
	service            *observation.Service
	leaderboardHandler *LeaderboardHandler
}

func NewObservationHandler(
	service *observation.Service,
	leaderboardHandler *LeaderboardHandler,
) *ObservationHandler {
	return &ObservationHandler{
		service:            service,
		leaderboardHandler: leaderboardHandler,
	}
}

func (h *ObservationHandler) Summary(c *gin.Context) {
	logGMObservation(c, "realtime.summary")

	summary, err := h.service.Summary(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50201,
			"message": "query realtime summary failed",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "ok",
		"data":    summary,
	})
}

func (h *ObservationHandler) Player(c *gin.Context) {
	logGMObservation(c, "realtime.player")

	playerID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || playerID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40092,
			"message": "invalid observation player id",
		})
		return
	}

	state, err := h.service.Player(c.Request.Context(), playerID)
	if errors.Is(err, observation.ErrPlayerNotFound) {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    40492,
			"message": "observation player not found",
		})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50202,
			"message": "query player observation failed",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "ok",
		"data":    state,
	})
}

func (h *ObservationHandler) Settlements(c *gin.Context) {
	logGMObservation(c, "settlements.list")

	page := parsePositiveInt(c.DefaultQuery("page", "1"), 1)
	pageSize := parsePositiveInt(c.DefaultQuery("page_size", "20"), 20)
	if pageSize > 50 {
		pageSize = 50
	}

	var playerID int64
	playerIDText := strings.TrimSpace(c.Query("player_id"))
	if playerIDText != "" {
		parsedPlayerID, err := strconv.ParseInt(playerIDText, 10, 64)
		if err != nil || parsedPlayerID <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    40093,
				"message": "invalid settlement player id",
			})
			return
		}
		playerID = parsedPlayerID
	}

	result, err := h.service.ListSettlements(
		c.Request.Context(),
		page,
		pageSize,
		c.Query("mission_id"),
		playerID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50203,
			"message": "query settlements failed",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "ok",
		"data":    result,
	})
}

func (h *ObservationHandler) Leaderboard(c *gin.Context) {
	logGMObservation(c, "leaderboard.list")
	h.leaderboardHandler.List(c)
}

func logGMObservation(c *gin.Context, action string) {
	adminID, _ := middleware.CurrentAdminID(c)
	log.Printf(
		"gm observation: request_id=%s admin_id=%d admin_username=%s action=%s",
		middleware.CurrentRequestID(c),
		adminID,
		middleware.CurrentAdminUsername(c),
		action,
	)
}
