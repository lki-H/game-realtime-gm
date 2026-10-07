package handler

import (
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"

	"game-realtime-gm/backend/internal/config"
	"game-realtime-gm/backend/internal/pve/run"
	"game-realtime-gm/backend/internal/pve/store"
	"github.com/gin-gonic/gin"
)

type V2PVEHandler struct {
	service *run.Service
	config  config.PVEConfig
}

func NewV2PVEHandler(service *run.Service, pveConfig config.PVEConfig) *V2PVEHandler {
	return &V2PVEHandler{service: service, config: pveConfig}
}
func (handler *V2PVEHandler) ApplyInternalEvent(c *gin.Context) {
	if !handler.config.TestEventsEnabled || strings.TrimSpace(handler.config.TestEventsToken) == "" {
		c.JSON(http.StatusNotFound, gin.H{"code": 40464, "message": "v2 test event endpoint disabled"})
		return
	}
	if subtle.ConstantTimeCompare([]byte(c.GetHeader("X-PVE-Test-Token")), []byte(handler.config.TestEventsToken)) != 1 {
		c.JSON(http.StatusForbidden, gin.H{"code": 40362, "message": "v2 test event permission required"})
		return
	}
	var event run.Event
	if err := c.ShouldBindJSON(&event); err != nil {
		c.JSON(400, gin.H{"code": 40063, "message": "invalid v2 event"})
		return
	}
	if event.RunID == "" {
		event.RunID = c.Param("run_id")
	}
	if event.RunID != c.Param("run_id") {
		c.JSON(400, gin.H{"code": 40064, "message": "run path mismatch"})
		return
	}
	result, err := handler.service.ApplyEvent(c.Request.Context(), event)
	if err != nil {
		status, code, text := 500, 50063, "apply v2 event failed"
		switch {
		case errors.Is(err, store.Invalid):
			status, code, text = 400, 40064, "invalid v2 event"
		case errors.Is(err, store.NotFound):
			status, code, text = 404, 40462, "v2 run not found"
		case errors.Is(err, store.Forbidden):
			status, code, text = 403, 40362, "v2 source or participant permission required"
		case errors.Is(err, store.Conflict):
			status, code, text = 409, 40962, "v2 event identity or state conflict"
		}
		c.JSON(status, gin.H{"code": code, "message": text, "data": result})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": result.Status, "data": result})
}
