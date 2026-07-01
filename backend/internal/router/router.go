package router

import (
	"net/http"

	"game-realtime-gm/backend/internal/config"
	"game-realtime-gm/backend/internal/handler"
	"game-realtime-gm/backend/internal/middleware"
	"game-realtime-gm/backend/internal/squad"
	"game-realtime-gm/backend/internal/ws"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

func New(db *pgxpool.Pool, redisClient *redis.Client, cfg config.Config) http.Handler {
	r := gin.Default()

	authHandler := handler.NewAuthHandler(db, cfg.JWTSecret)
	adminAuthHandler := handler.NewAdminAuthHandler(db, cfg.JWTSecret)
	adminHandler := handler.NewAdminHandler(db, redisClient)
	playerHandler := handler.NewPlayerHandler(db)
	onlineHandler := handler.NewOnlineHandler(redisClient)
	wsManager := ws.NewManager()
	squadManager := squad.NewManager()

	r.GET("/health", handler.Health)
	r.GET("/ws", handler.WebSocketEcho(cfg.JWTSecret, wsManager, redisClient, squadManager))

	api := r.Group("/api")
	api.POST("/register", authHandler.Register)
	api.POST("/login", authHandler.Login)
	api.POST("/admin/login", adminAuthHandler.Login)

	protected := api.Group("")
	protected.Use(middleware.Auth(cfg.JWTSecret))
	protected.GET("/me", playerHandler.Me)
	protected.PATCH("/me/nickname", playerHandler.UpdateNickname)
	protected.GET("/players", playerHandler.List)
	protected.GET("/players/:id", playerHandler.GetByID)
	protected.POST("/online/heartbeat", onlineHandler.Heartbeat)
	protected.GET("/online/status", onlineHandler.Status)

	adminProtected := api.Group("/admin")
	adminProtected.Use(middleware.AdminAuth(cfg.JWTSecret))
	adminProtected.GET("/me", adminHandler.Me)
	adminProtected.GET("/dashboard/summary", adminHandler.DashboardSummary)
	adminProtected.GET("/dashboard/recent-operation-logs", adminHandler.RecentOperationLogs)
	adminProtected.GET("/players", adminHandler.ListPlayers)
	adminProtected.GET("/players/:id", adminHandler.GetPlayerByID)
	adminProtected.POST("/players/:id/ban", adminHandler.BanPlayer)
	adminProtected.POST("/players/:id/unban", adminHandler.UnbanPlayer)
	adminProtected.GET("/operation-log-actions", adminHandler.ListOperationLogActions)
	adminProtected.GET("/operation-logs", adminHandler.ListOperationLogs)
	adminProtected.GET("/operation-logs/:id", adminHandler.GetOperationLogByID)
	return r
}
