package router

import (
	"context"
	"time"

	"database/sql"
	"net/http"

	"game-realtime-gm/backend/internal/config"
	"game-realtime-gm/backend/internal/handler"
	"game-realtime-gm/backend/internal/leaderboard"
	"game-realtime-gm/backend/internal/middleware"
	"game-realtime-gm/backend/internal/observation"
	"game-realtime-gm/backend/internal/pve"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

func New(ctx context.Context, db *sql.DB, redisClient *redis.Client, cfg config.Config, apps ...*pve.App) http.Handler {
	r := gin.New()
	_ = r.SetTrustedProxies(nil)
	r.Use(middleware.RequestID())
	r.Use(middleware.AccessLog())
	r.Use(middleware.BodyLimit(cfg.HTTP.MaxBodyBytes))
	r.Use(gin.Recovery())

	authHandler := handler.NewAuthHandler(db, cfg.JWTSecret)
	authHandler.Versioned = cfg.GameplayMode == "v2"
	adminAuthHandler := handler.NewAdminAuthHandler(db, cfg.JWTSecret)
	adminHandler := handler.NewAdminHandler(db, redisClient)
	adminHandler.Versioned = cfg.GameplayMode == "v2"
	playerHandler := handler.NewPlayerHandler(db)
	onlineHandler := handler.NewOnlineHandler(redisClient)
	leaderboardService := leaderboard.NewService(db, redisClient)
	leaderboardHandler := handler.NewLeaderboardHandler(leaderboardService)
	observationService := observation.NewService(db, nil, nil, nil, nil)
	if cfg.GameplayMode == "legacy" {
		observationService = registerLegacy(ctx, r, db, redisClient, cfg.JWTSecret, leaderboardService)
	}
	observationHandler := handler.NewObservationHandler(
		observationService,
		leaderboardHandler,
	)

	r.GET("/health", handler.Health)
	api := r.Group("/api")
	api.POST("/register", middleware.RedisRateLimit(redisClient, "register", cfg.HTTP.AuthRateLimit, time.Minute), authHandler.Register)
	api.POST("/login", middleware.RedisRateLimit(redisClient, "login", cfg.HTTP.AuthRateLimit, time.Minute), authHandler.Login)
	api.POST("/admin/login", middleware.RedisRateLimit(redisClient, "admin-login", cfg.HTTP.AuthRateLimit, time.Minute), adminAuthHandler.Login)

	protected := api.Group("")
	protected.Use(middleware.Auth(cfg.JWTSecret))
	if cfg.GameplayMode == "v2" {
		protected.Use(handler.V2Session(db, cfg.JWTSecret))
	}
	protected.GET("/me", playerHandler.Me)
	protected.PATCH("/me/nickname", playerHandler.UpdateNickname)
	protected.GET("/players", playerHandler.List)
	protected.GET("/players/:id", playerHandler.GetByID)
	protected.POST("/online/heartbeat", onlineHandler.Heartbeat)
	protected.GET("/online/status", onlineHandler.Status)
	protected.GET("/leaderboards/:mission_id", leaderboardHandler.List)
	protected.GET("/leaderboards/:mission_id/me", leaderboardHandler.Me)
	protected.GET("/me/mission-records", leaderboardHandler.MyHistory)
	if cfg.GameplayMode == "v2" && len(apps) > 0 {
		transport := handler.NewV2Transport(ctx, apps[0], cfg.JWTSecret)
		if cfg.HTTP.WebSocketMaxConnections > 0 {
			transport.MaxConnections = cfg.HTTP.WebSocketMaxConnections
		}
		if cfg.HTTP.WebSocketMaxPerIP > 0 {
			transport.MaxPerIP = cfg.HTTP.WebSocketMaxPerIP
		}
		if cfg.HTTP.WebSocketCommandRateLimit > 0 {
			transport.CommandRateLimit = cfg.HTTP.WebSocketCommandRateLimit
		}
		r.GET("/ws", middleware.RedisRateLimit(redisClient, "ws", cfg.HTTP.WebSocketRateLimit, time.Minute), transport.WebSocket)
		queries := api.Group("/v2")
		queries.Use(transport.Auth)
		for _, path := range []string{"/players/search", "/friends", "/friends/requests", "/friends/blocks", "/messages", "/messages/unread", "/me/activity", "/parties/:party_id", "/parties/:party_id/snapshot", "/match/proposals/:proposal_id", "/runs/:run_id", "/runs/:run_id/snapshot", "/runs/:run_id/results", "/operations/catalog", "/operations/preview", "/operations/recommendations", "/operations/:operation_id", "/recruitment", "/recruitment/applications", "/recruitment/parties/:party_id", "/regroup/:proposal_id", "/tasks"} {
			queries.GET(path, transport.Query)
		}
		go func() { <-ctx.Done(); transport.Close() }()
	}

	adminProtected := api.Group("/admin")
	adminProtected.Use(middleware.AdminAuth(cfg.JWTSecret))
	if cfg.GameplayMode == "v2" {
		adminProtected.Use(middleware.AdminSession(db))
	}
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
	if cfg.GameplayMode != "v2" {
		adminProtected.GET("/realtime/summary", observationHandler.Summary)
		adminProtected.GET("/realtime/players/:id", observationHandler.Player)
	}
	adminProtected.GET("/settlements", observationHandler.Settlements)
	adminProtected.GET("/leaderboards/:mission_id", observationHandler.Leaderboard)
	if cfg.GameplayMode == "v2" {
		if len(apps) > 0 {
			adminHandler.V2App = apps[0]
		}
		adminProtected.POST("/v2/operations/:operation_id/retry", middleware.RedisRateLimit(redisClient, "v2-repair", cfg.HTTP.AuthRateLimit, time.Minute), adminHandler.V2Retry)
		adminProtected.GET("/v2/control", adminHandler.V2ControlState)
		adminProtected.POST("/v2/control", middleware.RedisRateLimit(redisClient, "v2-control", cfg.HTTP.AuthRateLimit, time.Minute), adminHandler.V2Control)
		adminProtected.GET("/v2/observations/:entity", handler.V2Observation(db))
		adminProtected.GET("/v2/metrics", handler.V2Metrics(db))
	}
	return r
}
