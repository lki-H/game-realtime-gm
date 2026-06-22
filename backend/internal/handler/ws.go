package handler

import (
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type WSMessage struct {
	Type       string    `json:"type"`
	Content    string    `json:"content,omitempty"`
	ServerTime time.Time `json:"server_time"`
}

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func WebSocketEcho(c *gin.Context) {
	conn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("websocket upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	remoteAddr := conn.RemoteAddr().String()
	log.Printf("websocket connected: %s", remoteAddr)
	defer log.Printf("websocket disconnected: %s", remoteAddr)

	welcome := WSMessage{
		Type:       "welcome",
		Content:    "connected to game realtime server",
		ServerTime: time.Now(),
	}

	if err := conn.WriteJSON(welcome); err != nil {
		log.Printf("websocket write welcome failed: %v", err)
		return
	}

	for {
		messageType, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				log.Printf("websocket read failed: %v", err)
			}
			return
		}

		log.Printf("websocket received from %s: %s", remoteAddr, string(message))

		if err := conn.WriteMessage(messageType, message); err != nil {
			log.Printf("websocket write echo failed: %v", err)
			return
		}
	}
}
