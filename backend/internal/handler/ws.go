package handler

import (
	"context"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	tokenauth "game-realtime-gm/backend/internal/auth"
	realtimews "game-realtime-gm/backend/internal/ws"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
)

const (
	webSocketWriteWait       = 10 * time.Second
	webSocketPongWait        = 70 * time.Second
	webSocketPingPeriod      = 30 * time.Second
	webSocketMaxMessageBytes = 4096
)

type WSMessage struct {
	Type          string    `json:"type"`
	Content       string    `json:"content,omitempty"`
	ServerTime    time.Time `json:"server_time"`
	PlayerID      int64     `json:"player_id,omitempty"`
	Username      string    `json:"username,omitempty"`
	OnlinePlayers int       `json:"online_players,omitempty"`
	OnlineTTL     int       `json:"online_ttl_seconds,omitempty"`
}

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func WebSocketEcho(jwtSecret string, wsManager *realtimews.Manager, redisClient *redis.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims, ok := websocketPlayerClaims(c, jwtSecret)
		if !ok {
			return
		}

		conn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			log.Printf("websocket upgrade failed: %v", err)
			return
		}
		defer conn.Close()

		writeMu := &sync.Mutex{}
		conn.SetReadLimit(webSocketMaxMessageBytes)
		_ = conn.SetReadDeadline(time.Now().Add(webSocketPongWait))
		conn.SetPongHandler(func(appData string) error {
			log.Printf("websocket pong received: player_id=%d username=%s data=%s", claims.PlayerID, claims.Username, appData)
			return conn.SetReadDeadline(time.Now().Add(webSocketPongWait))
		})

		client := &realtimews.Client{
			PlayerID:    claims.PlayerID,
			Username:    claims.Username,
			Conn:        conn,
			ConnectedAt: time.Now(),
		}

		oldConn := wsManager.Register(client)
		if oldConn != nil {
			_ = oldConn.Close()
			log.Printf("websocket replaced old connection: player_id=%d username=%s", claims.PlayerID, claims.Username)
		}
		defer wsManager.Unregister(claims.PlayerID, conn)

		onlineCtx, stopOnlineRefresh := context.WithCancel(context.Background())
		defer stopOnlineRefresh()

		if err := refreshWebSocketOnlineStatus(onlineCtx, redisClient, claims.PlayerID); err != nil {
			log.Printf("websocket update redis online status failed: player_id=%d err=%v", claims.PlayerID, err)
			_ = writeWebSocketJSON(conn, writeMu, WSMessage{
				Type:       "error",
				Content:    "update online status failed",
				ServerTime: time.Now(),
				PlayerID:   claims.PlayerID,
				Username:   claims.Username,
			})
			return
		}

		go keepWebSocketOnlineStatus(onlineCtx, redisClient, claims.PlayerID)
		go keepWebSocketAlive(onlineCtx, conn, writeMu, claims.PlayerID, claims.Username)

		remoteAddr := conn.RemoteAddr().String()
		log.Printf("websocket connected: player_id=%d username=%s remote=%s online_players=%d", claims.PlayerID, claims.Username, remoteAddr, wsManager.Count())
		defer log.Printf("websocket disconnected: player_id=%d username=%s remote=%s", claims.PlayerID, claims.Username, remoteAddr)

		welcome := WSMessage{
			Type:          "welcome",
			Content:       "connected to game realtime server",
			ServerTime:    time.Now(),
			PlayerID:      claims.PlayerID,
			Username:      claims.Username,
			OnlinePlayers: wsManager.Count(),
			OnlineTTL:     int(onlineTTL.Seconds()),
		}

		if err := writeWebSocketJSON(conn, writeMu, welcome); err != nil {
			log.Printf("websocket write welcome failed: %v", err)
			return
		}

		for {
			messageType, message, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
					log.Printf("websocket read failed: player_id=%d err=%v", claims.PlayerID, err)
				}
				return
			}

			log.Printf("websocket received from player_id=%d: %s", claims.PlayerID, string(message))

			if err := writeWebSocketMessage(conn, writeMu, messageType, message); err != nil {
				log.Printf("websocket write echo failed: %v", err)
				return
			}
		}
	}
}

func websocketPlayerClaims(c *gin.Context, jwtSecret string) (*tokenauth.Claims, bool) {
	tokenString := strings.TrimSpace(c.Query("token"))
	if tokenString == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    40121,
			"message": "websocket token missing",
		})
		return nil, false
	}

	claims, err := tokenauth.ParseToken(jwtSecret, tokenString)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    40122,
			"message": "invalid websocket token",
		})
		return nil, false
	}

	if claims.SubjectType != tokenauth.SubjectTypePlayer || claims.PlayerID <= 0 {
		c.JSON(http.StatusForbidden, gin.H{
			"code":    40331,
			"message": "player token required",
		})
		return nil, false
	}

	return claims, true
}

func refreshWebSocketOnlineStatus(ctx context.Context, redisClient *redis.Client, playerID int64) error {
	key := onlinePlayerKey(playerID)
	return redisClient.Set(ctx, key, "1", onlineTTL).Err()
}

func keepWebSocketOnlineStatus(ctx context.Context, redisClient *redis.Client, playerID int64) {
	ticker := time.NewTicker(onlineTTL / 2)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := refreshWebSocketOnlineStatus(ctx, redisClient, playerID); err != nil {
				log.Printf("websocket refresh redis online status failed: player_id=%d err=%v", playerID, err)
			}
		}
	}
}

func keepWebSocketAlive(ctx context.Context, conn *websocket.Conn, writeMu *sync.Mutex, playerID int64, username string) {
	ticker := time.NewTicker(webSocketPingPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := writeWebSocketPing(conn, writeMu); err != nil {
				log.Printf("websocket ping failed: player_id=%d username=%s err=%v", playerID, username, err)
				_ = conn.Close()
				return
			}
		}
	}
}

func writeWebSocketJSON(conn *websocket.Conn, writeMu *sync.Mutex, value any) error {
	writeMu.Lock()
	defer writeMu.Unlock()

	if err := conn.SetWriteDeadline(time.Now().Add(webSocketWriteWait)); err != nil {
		return err
	}
	return conn.WriteJSON(value)
}

func writeWebSocketMessage(conn *websocket.Conn, writeMu *sync.Mutex, messageType int, message []byte) error {
	writeMu.Lock()
	defer writeMu.Unlock()

	if err := conn.SetWriteDeadline(time.Now().Add(webSocketWriteWait)); err != nil {
		return err
	}
	return conn.WriteMessage(messageType, message)
}

func writeWebSocketPing(conn *websocket.Conn, writeMu *sync.Mutex) error {
	writeMu.Lock()
	defer writeMu.Unlock()

	deadline := time.Now().Add(webSocketWriteWait)
	if err := conn.SetWriteDeadline(deadline); err != nil {
		return err
	}
	return conn.WriteControl(websocket.PingMessage, []byte("ping"), deadline)
}
