package router

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"game-realtime-gm/backend/internal/handler"
)

func New() http.Handler {
	r := gin.Default()
	r.GET("/health", handler.Health)
	return r
}
