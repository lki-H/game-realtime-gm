package router

import (
	"database/sql"
	"net/http"

	"game-realtime-gm/backend/internal/config"
	pvehandler "game-realtime-gm/backend/internal/handler"
	"game-realtime-gm/backend/internal/pve"

	"github.com/gin-gonic/gin"
)

func NewPVEInternalHandler(db *sql.DB, cfg config.Config, app *pve.App) http.Handler {
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(func(c *gin.Context) { c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16384); c.Next() })
	handler := pvehandler.NewV2PVEHandler(app.Runs, cfg.PVE)
	r.POST("/internal/v2/runs/:run_id/events", handler.ApplyInternalEvent)
	return r
}
