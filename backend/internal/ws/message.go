package ws

import (
	"encoding/json"
	"time"

	"game-realtime-gm/backend/internal/squad"
)

const (
	MessageTypeServerWelcome = "server.welcome"
	MessageTypeServerError   = "server.error"

	MessageTypeDebugEcho         = "debug.echo"
	MessageTypeDebugEchoResult   = "debug.echo.result"
	MessageTypeSquadCreate       = "squad.create"
	MessageTypeSquadCreateResult = "squad.create.result"
	MessageTypeSquadJoin         = "squad.join"
	MessageTypeSquadJoinResult   = "squad.join.result"
	MessageTypeSquadLeave        = "squad.leave"
	MessageTypeSquadLeaveResult  = "squad.leave.result"
	MessageTypeSquadReady        = "squad.ready"
	MessageTypeSquadReadyResult  = "squad.ready.result"
	MessageTypeSquadMe           = "squad.me"
	MessageTypeSquadMeResult     = "squad.me.result"
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
	ConnectionID  string    `json:"connection_id"`
	PlayerID      int64     `json:"player_id"`
	Username      string    `json:"username"`
	ConnectedAt   time.Time `json:"connected_at"`
	LastPongAt    time.Time `json:"last_pong_at"`
	OnlinePlayers int       `json:"online_players"`
	OnlineTTL     int       `json:"online_ttl_seconds"`
}

type EchoData struct {
	ReceivedType string          `json:"received_type"`
	ReceivedData json.RawMessage `json:"received_data,omitempty"`
}
type SquadJoinRequest struct {
	SquadID string `json:"squad_id"`
}

type SquadReadyRequest struct {
	Ready bool `json:"ready"`
}

type SquadData struct {
	Squad *squad.Squad `json:"squad,omitempty"`
}

type SquadLeaveData struct {
	Squad     *squad.Squad `json:"squad,omitempty"`
	Disbanded bool         `json:"disbanded"`
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
