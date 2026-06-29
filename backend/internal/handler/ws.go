package handler

import (
	"context"
	"encoding/json"
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
		connectedAt := time.Now()
		connectionID, err := realtimews.NewConnectionID(claims.PlayerID, connectedAt)
		if err != nil {
			log.Printf("websocket generate connection id failed: player_id=%d err=%v", claims.PlayerID, err)
			_ = writeWebSocketJSON(conn, writeMu, realtimews.NewErrorMessage("", 50025, "generate websocket connection id failed"))
			return
		}

		conn.SetReadLimit(webSocketMaxMessageBytes)
		_ = conn.SetReadDeadline(time.Now().Add(webSocketPongWait))
		conn.SetPongHandler(func(appData string) error {
			lastPongAt := time.Now()
			wsManager.UpdateLastPong(claims.PlayerID, connectionID, lastPongAt)
			log.Printf("websocket pong received: player_id=%d username=%s connection_id=%s data=%s", claims.PlayerID, claims.Username, connectionID, appData)
			return conn.SetReadDeadline(time.Now().Add(webSocketPongWait))
		})

		client := &realtimews.Client{
			ConnectionID: connectionID,
			PlayerID:     claims.PlayerID,
			Username:     claims.Username,
			Conn:         conn,
			ConnectedAt:  connectedAt,
			LastPongAt:   connectedAt,
		}

		oldConn := wsManager.Register(client)
		if oldConn != nil {
			_ = oldConn.Close()
			log.Printf("websocket replaced old connection: player_id=%d username=%s", claims.PlayerID, claims.Username)
		}
		defer wsManager.Unregister(claims.PlayerID, connectionID)

		onlineCtx, stopOnlineRefresh := context.WithCancel(context.Background())
		defer stopOnlineRefresh()

		if err := refreshWebSocketOnlineStatus(onlineCtx, redisClient, claims.PlayerID, connectionID); err != nil {
			log.Printf("websocket update redis online status failed: player_id=%d err=%v", claims.PlayerID, err)
			_ = writeWebSocketJSON(conn, writeMu, realtimews.NewErrorMessage("", 50024, "update online status failed"))
			return
		}

		go keepWebSocketOnlineStatus(onlineCtx, redisClient, claims.PlayerID, connectionID)
		go keepWebSocketAlive(onlineCtx, conn, writeMu, claims.PlayerID, claims.Username)

		remoteAddr := conn.RemoteAddr().String()
		log.Printf("websocket connected: player_id=%d username=%s connection_id=%s remote=%s online_players=%d", claims.PlayerID, claims.Username, connectionID, remoteAddr, wsManager.Count())
		defer log.Printf("websocket disconnected: player_id=%d username=%s connection_id=%s remote=%s", claims.PlayerID, claims.Username, connectionID, remoteAddr)

		welcome := realtimews.NewServerMessage(
			realtimews.MessageTypeServerWelcome,
			"",
			realtimews.WelcomeData{
				ConnectionID:  connectionID,
				PlayerID:      claims.PlayerID,
				Username:      claims.Username,
				ConnectedAt:   connectedAt,
				LastPongAt:    connectedAt,
				OnlinePlayers: wsManager.Count(),
				OnlineTTL:     int(onlineTTL.Seconds()),
			},
		)

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

			if messageType != websocket.TextMessage {
				errMsg := realtimews.NewErrorMessage("", 40026, "websocket only supports text json messages")
				if err := writeWebSocketJSON(conn, writeMu, errMsg); err != nil {
					log.Printf("websocket write message type error failed: %v", err)
					return
				}
				continue
			}

			var clientMessage realtimews.ClientMessage
			if err := json.Unmarshal(message, &clientMessage); err != nil {
				errMsg := realtimews.NewErrorMessage("", 40024, "invalid websocket message json")
				if err := writeWebSocketJSON(conn, writeMu, errMsg); err != nil {
					log.Printf("websocket write invalid json error failed: %v", err)
					return
				}
				continue
			}

			if strings.TrimSpace(clientMessage.Type) == "" {
				errMsg := realtimews.NewErrorMessage(clientMessage.RequestID, 40025, "websocket message type required")
				if err := writeWebSocketJSON(conn, writeMu, errMsg); err != nil {
					log.Printf("websocket write missing type error failed: %v", err)
					return
				}
				continue
			}

			switch clientMessage.Type {
			case realtimews.MessageTypeDebugEcho:
				response := realtimews.NewServerMessage(
					realtimews.MessageTypeDebugEchoResult,
					clientMessage.RequestID,
					realtimews.EchoData{
						ReceivedType: clientMessage.Type,
						ReceivedData: clientMessage.Data,
					},
				)
				if err := writeWebSocketJSON(conn, writeMu, response); err != nil {
					log.Printf("websocket write echo response failed: %v", err)
					return
				}
			default:
				errMsg := realtimews.NewErrorMessage(clientMessage.RequestID, 40424, "unsupported websocket message type")
				if err := writeWebSocketJSON(conn, writeMu, errMsg); err != nil {
					log.Printf("websocket write unsupported type error failed: %v", err)
					return
				}
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

func refreshWebSocketOnlineStatus(ctx context.Context, redisClient *redis.Client, playerID int64, connectionID string) error {
	key := onlinePlayerKey(playerID)
	return redisClient.Set(ctx, key, connectionID, onlineTTL).Err()
}

func keepWebSocketOnlineStatus(ctx context.Context, redisClient *redis.Client, playerID int64, connectionID string) {
	ticker := time.NewTicker(onlineTTL / 2)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := refreshWebSocketOnlineStatus(ctx, redisClient, playerID, connectionID); err != nil {
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

func writeWebSocketPing(conn *websocket.Conn, writeMu *sync.Mutex) error {
	writeMu.Lock()
	defer writeMu.Unlock()

	deadline := time.Now().Add(webSocketWriteWait)
	if err := conn.SetWriteDeadline(deadline); err != nil {
		return err
	}
	return conn.WriteControl(websocket.PingMessage, []byte("ping"), deadline)
}
