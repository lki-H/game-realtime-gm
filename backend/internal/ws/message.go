package ws

import (
	"encoding/json"
	"time"
)

const (
	MessageTypeServerWelcome = "server.welcome"
	MessageTypeServerError   = "server.error"

	MessageTypeDebugEcho       = "debug.echo"
	MessageTypeDebugEchoResult = "debug.echo.result"
)

type ClientMessage struct {
	Type      string          `json:"type"`
	RequestID string          `json:"request_id,omitempty"`
	Data      json.RawMessage `json:"data,omitempty"`
}

type ServerMessage struct {
	Type       string    `json:"type"`
	RequestID  string    `json:"request_id,omitempty"`
	Code       int       `json:"code"`
	Message    string    `json:"message"`
	Data       any       `json:"data,omitempty"`
	ServerTime time.Time `json:"server_time"`
}

type WelcomeData struct {
	PlayerID      int64  `json:"player_id"`
	Username      string `json:"username"`
	OnlinePlayers int    `json:"online_players"`
	OnlineTTL     int    `json:"online_ttl_seconds"`
}

type EchoData struct {
	ReceivedType string          `json:"received_type"`
	ReceivedData json.RawMessage `json:"received_data,omitempty"`
}

func NewServerMessage(messageType string, requestID string, data any) ServerMessage {
	return ServerMessage{
		Type:       messageType,
		RequestID:  requestID,
		Code:       0,
		Message:    "ok",
		Data:       data,
		ServerTime: time.Now(),
	}
}

func NewErrorMessage(requestID string, code int, message string) ServerMessage {
	return ServerMessage{
		Type:       MessageTypeServerError,
		RequestID:  requestID,
		Code:       code,
		Message:    message,
		ServerTime: time.Now(),
	}
}
