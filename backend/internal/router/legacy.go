package router

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"time"

	"game-realtime-gm/backend/internal/handler"
	"game-realtime-gm/backend/internal/leaderboard"
	"game-realtime-gm/backend/internal/matchmaking"
	"game-realtime-gm/backend/internal/mission"
	"game-realtime-gm/backend/internal/observation"
	"game-realtime-gm/backend/internal/settlement"
	"game-realtime-gm/backend/internal/squad"
	"game-realtime-gm/backend/internal/ws"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

func registerLegacy(ctx context.Context, routes *gin.Engine, db *sql.DB, cache *redis.Client, secret string, rankings *leaderboard.Service) *observation.Service {
	connections := ws.NewManager()
	groups := squad.NewManager()
	missions := mission.NewManager()
	queue := matchmaking.NewManager(cache)
	rewards := settlement.NewService(db, missions)
	routes.GET("/ws", handler.WebSocketEcho(secret, connections, cache, groups, missions, queue, rewards, rankings))
	go queue.RunTimeoutLoop(ctx, time.Second, func(ticket *matchmaking.Ticket) {
		message := ws.NewServerMessage(ws.MessageTypeMatchmakingStateChanged, "", ws.MatchmakingStateChangedData{Event: ws.MatchmakingEventTimeout, Ticket: ticket})
		if err := connections.SendToPlayer(ticket.PlayerID, message); err != nil && !errors.Is(err, ws.ErrClientNotConnected) {
			log.Printf("legacy matchmaking timeout notification failed: player_id=%d ticket_id=%s err=%v", ticket.PlayerID, ticket.ID, err)
		}
	}, func(err error) {
		log.Printf("legacy matchmaking timeout cleanup failed: %v", err)
	})
	return observation.NewService(db, connections, groups, missions, queue)
}
