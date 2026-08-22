package router

import (
	"context"
	"errors"
	"log"
	"time"

	"database/sql"
	"net/http"

	"game-realtime-gm/backend/internal/config"
	"game-realtime-gm/backend/internal/handler"
	"game-realtime-gm/backend/internal/matchmaking"
	"game-realtime-gm/backend/internal/middleware"
	"game-realtime-gm/backend/internal/mission"
	"game-realtime-gm/backend/internal/squad"
	"game-realtime-gm/backend/internal/ws"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

func New(ctx context.Context, db *sql.DB, redisClient *redis.Client, cfg config.Config) http.Handler {
	r := gin.Default()

	authHandler := handler.NewAuthHandler(db, cfg.JWTSecret)
	adminAuthHandler := handler.NewAdminAuthHandler(db, cfg.JWTSecret)
	adminHandler := handler.NewAdminHandler(db, redisClient)
	playerHandler := handler.NewPlayerHandler(db)
	onlineHandler := handler.NewOnlineHandler(redisClient)
	wsManager := ws.NewManager()
	squadManager := squad.NewManager()
	missionManager := mission.NewManager()
	matchmakingManager := matchmaking.NewManager(redisClient)

	r.GET("/health", handler.Health)
	r.GET("/ws", handler.WebSocketEcho(
		cfg.JWTSecret,
		wsManager,
		redisClient,
		squadManager,
		missionManager,
		matchmakingManager,
	))

	go matchmakingManager.RunTimeoutLoop(
		ctx,
		time.Second,
		func(ticket *matchmaking.Ticket) {
			message := ws.NewServerMessage(
				ws.MessageTypeMatchmakingStateChanged,
				"",
				ws.MatchmakingStateChangedData{
					Event:  ws.MatchmakingEventTimeout,
					Ticket: ticket,
				},
			)

			if err := wsManager.SendToPlayer(ticket.PlayerID, message); err != nil && !errors.Is(err, ws.ErrClientNotConnected) {
				log.Printf("websocket notify matchmaking timeout failed: player_id=%d ticket_id=%s err=%v", ticket.PlayerID, ticket.ID, err)
			}
		},
		func(err error) {
			log.Printf("matchmaking timeout cleanup failed: %v", err)
		},
	)

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
