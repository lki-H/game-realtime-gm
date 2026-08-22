package ws

import (
	"encoding/json"
	"time"

	"game-realtime-gm/backend/internal/matchmaking"
	"game-realtime-gm/backend/internal/mission"
	"game-realtime-gm/backend/internal/squad"
)

const (
	MessageTypeServerWelcome = "server.welcome"
	MessageTypeServerError   = "server.error"

	MessageTypeDebugEcho                = "debug.echo"
	MessageTypeDebugEchoResult          = "debug.echo.result"
	MessageTypeSquadCreate              = "squad.create"
	MessageTypeSquadCreateResult        = "squad.create.result"
	MessageTypeSquadJoin                = "squad.join"
	MessageTypeSquadJoinResult          = "squad.join.result"
	MessageTypeSquadLeave               = "squad.leave"
	MessageTypeSquadLeaveResult         = "squad.leave.result"
	MessageTypeSquadReady               = "squad.ready"
	MessageTypeSquadReadyResult         = "squad.ready.result"
	MessageTypeSquadMe                  = "squad.me"
	MessageTypeSquadMeResult            = "squad.me.result"
	MessageTypeSquadStateChanged        = "squad.state.changed"
	MessageTypeMissionCreate            = "mission.create"
	MessageTypeMissionCreateResult      = "mission.create.result"
	MessageTypeMissionReady             = "mission.ready"
	MessageTypeMissionReadyResult       = "mission.ready.result"
	MessageTypeMissionStart             = "mission.start"
	MessageTypeMissionStartResult       = "mission.start.result"
	MessageTypeMissionFinish            = "mission.finish"
	MessageTypeMissionFinishResult      = "mission.finish.result"
	MessageTypeMissionCancel            = "mission.cancel"
	MessageTypeMissionCancelResult      = "mission.cancel.result"
	MessageTypeMissionMe                = "mission.me"
	MessageTypeMissionMeResult          = "mission.me.result"
	MessageTypeMissionStateChanged      = "mission.state.changed"
	MessageTypeMatchmakingEnqueue       = "matchmaking.enqueue"
	MessageTypeMatchmakingEnqueueResult = "matchmaking.enqueue.result"
	MessageTypeMatchmakingCancel        = "matchmaking.cancel"
	MessageTypeMatchmakingCancelResult  = "matchmaking.cancel.result"
	MessageTypeMatchmakingMe            = "matchmaking.me"
	MessageTypeMatchmakingMeResult      = "matchmaking.me.result"
	MessageTypeMatchmakingStateChanged  = "matchmaking.state.changed"
)

const (
	SquadEventMemberJoined       = "member_joined"
	SquadEventMemberLeft         = "member_left"
	SquadEventReadyChanged       = "ready_changed"
	SquadEventSquadDisbanded     = "squad_disbanded"
	SquadEventMemberDisconnected = "member_disconnected"
	SquadEventMemberReconnected  = "member_reconnected"
	SquadEventLeaderChanged      = "leader_changed"
)

const (
	MissionEventCreated  = "mission_created"
	MissionEventReady    = "mission_ready"
	MissionEventStarted  = "mission_started"
	MissionEventFinished = "mission_finished"
	MissionEventCanceled = "mission_canceled"
)

const (
	MatchmakingEventQueued   = "match_queued"
	MatchmakingEventCanceled = "match_canceled"
	MatchmakingEventTimeout  = "match_timeout"
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

type SquadStateChangedData struct {
	Event         string       `json:"event"`
	ActorPlayerID int64        `json:"actor_player_id,omitempty"`
	Squad         *squad.Squad `json:"squad,omitempty"`
	Disbanded     bool         `json:"disbanded,omitempty"`
}

type MissionCreateRequest struct {
	MissionID string `json:"mission_id"`
}

type MissionData struct {
	Mission *mission.Instance `json:"mission,omitempty"`
}

type MissionStateChangedData struct {
	Event         string            `json:"event"`
	ActorPlayerID int64             `json:"actor_player_id,omitempty"`
	Mission       *mission.Instance `json:"mission,omitempty"`
}

type MatchmakingEnqueueRequest struct {
	MissionID string `json:"mission_id"`
	Role      string `json:"role"`
}

type MatchmakingData struct {
	Ticket *matchmaking.Ticket `json:"ticket,omitempty"`
}

type MatchmakingStateChangedData struct {
	Event  string              `json:"event"`
	Ticket *matchmaking.Ticket `json:"ticket,omitempty"`
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
