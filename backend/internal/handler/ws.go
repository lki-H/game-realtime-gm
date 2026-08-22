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
	gamematchmaking "game-realtime-gm/backend/internal/matchmaking"
	gamemission "game-realtime-gm/backend/internal/mission"
	gamesquad "game-realtime-gm/backend/internal/squad"
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

func WebSocketEcho(
	jwtSecret string,
	wsManager *realtimews.Manager,
	redisClient *redis.Client,
	squadManager *gamesquad.Manager,
	missionManager *gamemission.Manager,
	matchmakingManager *gamematchmaking.Manager,
) gin.HandlerFunc {
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
			WriteMu:      writeMu,
			ConnectedAt:  connectedAt,
			LastPongAt:   connectedAt,
		}

		oldConn := wsManager.Register(client)
		if oldConn != nil {
			_ = oldConn.Close()
			log.Printf("websocket replaced old connection: player_id=%d username=%s", claims.PlayerID, claims.Username)
		}
		defer func() {
			if !wsManager.Unregister(claims.PlayerID, connectionID) {
				return
			}

			squadState, changed, leaderChanged, err := squadManager.HandleDisconnect(claims.PlayerID)
			if err != nil || !changed {
				return
			}

			event := realtimews.SquadEventMemberDisconnected
			if leaderChanged {
				event = realtimews.SquadEventLeaderChanged
			}
			broadcastSquadState(wsManager, squadState, claims.PlayerID, event, false)
		}()

		onlineCtx, stopOnlineRefresh := context.WithCancel(context.Background())
		defer stopOnlineRefresh()

		if err := refreshWebSocketOnlineStatus(onlineCtx, redisClient, claims.PlayerID, connectionID); err != nil {
			log.Printf("websocket update redis online status failed: player_id=%d err=%v", claims.PlayerID, err)
			_ = writeWebSocketJSON(conn, writeMu, realtimews.NewErrorMessage("", 50024, "update online status failed"))
			return
		}

		go keepWebSocketOnlineStatus(onlineCtx, redisClient, claims.PlayerID, connectionID)
		go keepWebSocketAlive(onlineCtx, conn, writeMu, claims.PlayerID, claims.Username)

		if squadState, changed, err := squadManager.HandleReconnect(claims.PlayerID); err == nil && changed {
			broadcastSquadState(wsManager, squadState, claims.PlayerID, realtimews.SquadEventMemberReconnected, false)
		}

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

			case realtimews.MessageTypeSquadJoin:
				var request realtimews.SquadJoinRequest
				if err := json.Unmarshal(clientMessage.Data, &request); err != nil {
					errMsg := realtimews.NewErrorMessage(clientMessage.RequestID, 40027, "invalid squad join data")
					if err := writeWebSocketJSON(conn, writeMu, errMsg); err != nil {
						log.Printf("websocket write squad join invalid data failed: %v", err)
						return
					}
					continue
				}

				if strings.TrimSpace(request.SquadID) == "" {
					errMsg := realtimews.NewErrorMessage(clientMessage.RequestID, 40028, "squad_id required")
					if err := writeWebSocketJSON(conn, writeMu, errMsg); err != nil {
						log.Printf("websocket write squad join missing squad_id failed: %v", err)
						return
					}
					continue
				}

				joinedSquad, err := squadManager.Join(request.SquadID, claims.PlayerID, claims.Username)
				if err != nil {
					errMsg := squadErrorMessage(clientMessage.RequestID, err)
					if err := writeWebSocketJSON(conn, writeMu, errMsg); err != nil {
						log.Printf("websocket write squad join error failed: %v", err)
						return
					}
					continue
				}

				response := realtimews.NewServerMessage(
					realtimews.MessageTypeSquadJoinResult,
					clientMessage.RequestID,
					realtimews.SquadData{Squad: joinedSquad},
				)
				if err := writeWebSocketJSON(conn, writeMu, response); err != nil {
					log.Printf("websocket write squad join response failed: %v", err)
					return
				}

				broadcastSquadState(wsManager, joinedSquad, claims.PlayerID, realtimews.SquadEventMemberJoined, false)

			case realtimews.MessageTypeSquadLeave:
				squadBeforeLeave, _ := squadManager.GetByPlayer(claims.PlayerID)
				wasLeader := squadBeforeLeave != nil && squadBeforeLeave.LeaderID == claims.PlayerID

				leftSquad, disbanded, err := squadManager.Leave(claims.PlayerID)
				if err != nil {
					errMsg := squadErrorMessage(clientMessage.RequestID, err)
					if err := writeWebSocketJSON(conn, writeMu, errMsg); err != nil {
						log.Printf("websocket write squad leave error failed: %v", err)
						return
					}
					continue
				}

				response := realtimews.NewServerMessage(
					realtimews.MessageTypeSquadLeaveResult,
					clientMessage.RequestID,
					realtimews.SquadLeaveData{
						Squad:     leftSquad,
						Disbanded: disbanded,
					},
				)
				if err := writeWebSocketJSON(conn, writeMu, response); err != nil {
					log.Printf("websocket write squad leave response failed: %v", err)
					return
				}

				if !disbanded {
					event := realtimews.SquadEventMemberLeft
					if wasLeader {
						event = realtimews.SquadEventLeaderChanged
					}

					broadcastSquadState(
						wsManager,
						leftSquad,
						claims.PlayerID,
						event,
						false,
					)
				}

			case realtimews.MessageTypeSquadReady:
				var request realtimews.SquadReadyRequest
				if err := json.Unmarshal(clientMessage.Data, &request); err != nil {
					errMsg := realtimews.NewErrorMessage(clientMessage.RequestID, 40029, "invalid squad ready data")
					if err := writeWebSocketJSON(conn, writeMu, errMsg); err != nil {
						log.Printf("websocket write squad ready invalid data failed: %v", err)
						return
					}
					continue
				}

				updatedSquad, err := squadManager.SetReady(claims.PlayerID, request.Ready)
				if err != nil {
					errMsg := squadErrorMessage(clientMessage.RequestID, err)
					if err := writeWebSocketJSON(conn, writeMu, errMsg); err != nil {
						log.Printf("websocket write squad ready error failed: %v", err)
						return
					}
					continue
				}

				response := realtimews.NewServerMessage(
					realtimews.MessageTypeSquadReadyResult,
					clientMessage.RequestID,
					realtimews.SquadData{Squad: updatedSquad},
				)
				if err := writeWebSocketJSON(conn, writeMu, response); err != nil {
					log.Printf("websocket write squad ready response failed: %v", err)
					return
				}

				broadcastSquadState(wsManager, updatedSquad, claims.PlayerID, realtimews.SquadEventReadyChanged, false)

			case realtimews.MessageTypeSquadMe:
				currentSquad, err := squadManager.GetByPlayer(claims.PlayerID)
				if err != nil {
					errMsg := squadErrorMessage(clientMessage.RequestID, err)
					if err := writeWebSocketJSON(conn, writeMu, errMsg); err != nil {
						log.Printf("websocket write squad me error failed: %v", err)
						return
					}
					continue
				}

				response := realtimews.NewServerMessage(
					realtimews.MessageTypeSquadMeResult,
					clientMessage.RequestID,
					realtimews.SquadData{Squad: currentSquad},
				)
				if err := writeWebSocketJSON(conn, writeMu, response); err != nil {
					log.Printf("websocket write squad me response failed: %v", err)
					return
				}

			case realtimews.MessageTypeSquadCreate:
				createdSquad, err := squadManager.Create(claims.PlayerID, claims.Username)
				if err != nil {
					errMsg := squadErrorMessage(clientMessage.RequestID, err)
					if err := writeWebSocketJSON(conn, writeMu, errMsg); err != nil {
						log.Printf("websocket write squad create error failed: %v", err)
						return
					}
					continue
				}

				response := realtimews.NewServerMessage(
					realtimews.MessageTypeSquadCreateResult,
					clientMessage.RequestID,
					realtimews.SquadData{Squad: createdSquad},
				)
				if err := writeWebSocketJSON(conn, writeMu, response); err != nil {
					log.Printf("websocket write squad create response failed: %v", err)
					return
				}

			case realtimews.MessageTypeMissionCreate:
				var request realtimews.MissionCreateRequest
				if err := json.Unmarshal(clientMessage.Data, &request); err != nil {
					errMsg := realtimews.NewErrorMessage(clientMessage.RequestID, 40030, "invalid mission create data")
					if err := writeWebSocketJSON(conn, writeMu, errMsg); err != nil {
						log.Printf("websocket write mission create invalid data failed: %v", err)
						return
					}
					continue
				}
				request.MissionID = strings.TrimSpace(request.MissionID)
				if request.MissionID == "" {
					errMsg := realtimews.NewErrorMessage(clientMessage.RequestID, 40031, "mission_id required")
					if err := writeWebSocketJSON(conn, writeMu, errMsg); err != nil {
						log.Printf("websocket write mission_id required failed: %v", err)
						return
					}
					continue
				}

				squadState, err := squadManager.GetByPlayer(claims.PlayerID)
				if err != nil {
					if err := writeWebSocketJSON(conn, writeMu, squadErrorMessage(clientMessage.RequestID, err)); err != nil {
						log.Printf("websocket write mission create squad error failed: %v", err)
						return
					}
					continue
				}
				if squadState.LeaderID != claims.PlayerID {
					errMsg := realtimews.NewErrorMessage(clientMessage.RequestID, 40332, "squad leader required")
					if err := writeWebSocketJSON(conn, writeMu, errMsg); err != nil {
						log.Printf("websocket write squad leader required failed: %v", err)
						return
					}
					continue
				}

				playerIDs := make([]int64, 0, len(squadState.Members))
				for _, member := range squadState.Members {
					playerIDs = append(playerIDs, member.PlayerID)
				}

				createdMission, err := missionManager.Create(request.MissionID, squadState.ID, playerIDs)
				if err != nil {
					if err := writeWebSocketJSON(conn, writeMu, missionErrorMessage(clientMessage.RequestID, err)); err != nil {
						log.Printf("websocket write mission create error failed: %v", err)
						return
					}
					continue
				}

				response := realtimews.NewServerMessage(
					realtimews.MessageTypeMissionCreateResult,
					clientMessage.RequestID,
					realtimews.MissionData{Mission: createdMission},
				)
				if err := writeWebSocketJSON(conn, writeMu, response); err != nil {
					log.Printf("websocket write mission create response failed: %v", err)
					return
				}

				broadcastMissionState(wsManager, createdMission, claims.PlayerID, realtimews.MissionEventCreated)

			case realtimews.MessageTypeMissionReady:
				if err := handleMissionTransition(
					conn,
					writeMu,
					clientMessage.RequestID,
					claims.PlayerID,
					gamemission.StatusReady,
					realtimews.MessageTypeMissionReadyResult,
					realtimews.MissionEventReady,
					true,
					wsManager,
					squadManager,
					missionManager,
				); err != nil {
					log.Printf("websocket handle mission ready failed: %v", err)
					return
				}

			case realtimews.MessageTypeMissionStart:
				if err := handleMissionTransition(
					conn,
					writeMu,
					clientMessage.RequestID,
					claims.PlayerID,
					gamemission.StatusRunning,
					realtimews.MessageTypeMissionStartResult,
					realtimews.MissionEventStarted,
					true,
					wsManager,
					squadManager,
					missionManager,
				); err != nil {
					log.Printf("websocket handle mission start failed: %v", err)
					return
				}

			case realtimews.MessageTypeMissionFinish:
				if err := handleMissionTransition(
					conn,
					writeMu,
					clientMessage.RequestID,
					claims.PlayerID,
					gamemission.StatusFinished,
					realtimews.MessageTypeMissionFinishResult,
					realtimews.MissionEventFinished,
					false,
					wsManager,
					squadManager,
					missionManager,
				); err != nil {
					log.Printf("websocket handle mission finish failed: %v", err)
					return
				}

			case realtimews.MessageTypeMissionCancel:
				if err := handleMissionTransition(
					conn,
					writeMu,
					clientMessage.RequestID,
					claims.PlayerID,
					gamemission.StatusCanceled,
					realtimews.MessageTypeMissionCancelResult,
					realtimews.MissionEventCanceled,
					false,
					wsManager,
					squadManager,
					missionManager,
				); err != nil {
					log.Printf("websocket handle mission cancel failed: %v", err)
					return
				}

			case realtimews.MessageTypeMissionMe:
				currentMission, err := missionManager.GetByPlayer(claims.PlayerID)
				if err != nil {
					if err := writeWebSocketJSON(conn, writeMu, missionErrorMessage(clientMessage.RequestID, err)); err != nil {
						log.Printf("websocket write mission me error failed: %v", err)
						return
					}
					continue
				}

				response := realtimews.NewServerMessage(
					realtimews.MessageTypeMissionMeResult,
					clientMessage.RequestID,
					realtimews.MissionData{Mission: currentMission},
				)
				if err := writeWebSocketJSON(conn, writeMu, response); err != nil {
					log.Printf("websocket write mission me response failed: %v", err)
					return
				}

			case realtimews.MessageTypeMatchmakingEnqueue:
				var request realtimews.MatchmakingEnqueueRequest
				if err := json.Unmarshal(clientMessage.Data, &request); err != nil {
					errMsg := realtimews.NewErrorMessage(clientMessage.RequestID, 40032, "invalid matchmaking enqueue data")
					if err := writeWebSocketJSON(conn, writeMu, errMsg); err != nil {
						log.Printf("websocket write matchmaking enqueue invalid data failed: %v", err)
						return
					}
					continue
				}

				squadID := ""
				if squadState, err := squadManager.GetByPlayer(claims.PlayerID); err == nil {
					squadID = squadState.ID
				}

				ticket, err := matchmakingManager.Enqueue(
					c.Request.Context(),
					request.MissionID,
					claims.PlayerID,
					squadID,
					request.Role,
				)
				if err != nil {
					if err := writeWebSocketJSON(conn, writeMu, matchmakingErrorMessage(clientMessage.RequestID, err)); err != nil {
						log.Printf("websocket write matchmaking enqueue error failed: %v", err)
						return
					}
					continue
				}

				response := realtimews.NewServerMessage(
					realtimews.MessageTypeMatchmakingEnqueueResult,
					clientMessage.RequestID,
					realtimews.MatchmakingData{Ticket: ticket},
				)
				if err := writeWebSocketJSON(conn, writeMu, response); err != nil {
					log.Printf("websocket write matchmaking enqueue response failed: %v", err)
					return
				}

			case realtimews.MessageTypeMatchmakingCancel:
				ticket, err := matchmakingManager.Cancel(c.Request.Context(), claims.PlayerID)
				if err != nil {
					if err := writeWebSocketJSON(conn, writeMu, matchmakingErrorMessage(clientMessage.RequestID, err)); err != nil {
						log.Printf("websocket write matchmaking cancel error failed: %v", err)
						return
					}
					continue
				}

				response := realtimews.NewServerMessage(
					realtimews.MessageTypeMatchmakingCancelResult,
					clientMessage.RequestID,
					realtimews.MatchmakingData{Ticket: ticket},
				)
				if err := writeWebSocketJSON(conn, writeMu, response); err != nil {
					log.Printf("websocket write matchmaking cancel response failed: %v", err)
					return
				}

			case realtimews.MessageTypeMatchmakingMe:
				ticket, err := matchmakingManager.GetByPlayer(c.Request.Context(), claims.PlayerID)
				if err != nil {
					if err := writeWebSocketJSON(conn, writeMu, matchmakingErrorMessage(clientMessage.RequestID, err)); err != nil {
						log.Printf("websocket write matchmaking me error failed: %v", err)
						return
					}
					continue
				}

				response := realtimews.NewServerMessage(
					realtimews.MessageTypeMatchmakingMeResult,
					clientMessage.RequestID,
					realtimews.MatchmakingData{Ticket: ticket},
				)
				if err := writeWebSocketJSON(conn, writeMu, response); err != nil {
					log.Printf("websocket write matchmaking me response failed: %v", err)
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

func broadcastSquadState(wsManager *realtimews.Manager, squadState *gamesquad.Squad, actorPlayerID int64, event string, disbanded bool) {
	if squadState == nil {
		return
	}

	playerIDs := make([]int64, 0, len(squadState.Members))
	for _, member := range squadState.Members {
		if member.PlayerID != actorPlayerID && member.Online {
			playerIDs = append(playerIDs, member.PlayerID)
		}
	}

	if len(playerIDs) == 0 {
		return
	}

	message := realtimews.NewServerMessage(
		realtimews.MessageTypeSquadStateChanged,
		"",
		realtimews.SquadStateChangedData{
			Event:         event,
			ActorPlayerID: actorPlayerID,
			Squad:         squadState,
			Disbanded:     disbanded,
		},
	)

	failedPlayerIDs := wsManager.BroadcastToPlayers(playerIDs, message)
	if len(failedPlayerIDs) > 0 {
		log.Printf("websocket broadcast squad state failed: event=%s actor_player_id=%d failed_player_ids=%v", event, actorPlayerID, failedPlayerIDs)
	}
}

func broadcastMissionState(
	wsManager *realtimews.Manager,
	missionState *gamemission.Instance,
	actorPlayerID int64,
	event string,
) {
	if missionState == nil {
		return
	}

	playerIDs := make([]int64, 0, len(missionState.PlayerIDs))
	for _, playerID := range missionState.PlayerIDs {
		if playerID != actorPlayerID {
			playerIDs = append(playerIDs, playerID)
		}
	}
	if len(playerIDs) == 0 {
		return
	}

	message := realtimews.NewServerMessage(
		realtimews.MessageTypeMissionStateChanged,
		"",
		realtimews.MissionStateChangedData{
			Event:         event,
			ActorPlayerID: actorPlayerID,
			Mission:       missionState,
		},
	)

	failedPlayerIDs := wsManager.BroadcastToPlayers(playerIDs, message)
	if len(failedPlayerIDs) > 0 {
		log.Printf("websocket broadcast mission state failed: event=%s actor_player_id=%d failed_player_ids=%v", event, actorPlayerID, failedPlayerIDs)
	}
}

func handleMissionTransition(
	conn *websocket.Conn,
	writeMu *sync.Mutex,
	requestID string,
	playerID int64,
	target gamemission.Status,
	resultType string,
	event string,
	requireSquadReady bool,
	wsManager *realtimews.Manager,
	squadManager *gamesquad.Manager,
	missionManager *gamemission.Manager,
) error {
	squadState, err := squadManager.GetByPlayer(playerID)
	if err != nil {
		return writeWebSocketJSON(conn, writeMu, squadErrorMessage(requestID, err))
	}
	if squadState.LeaderID != playerID {
		return writeWebSocketJSON(conn, writeMu, realtimews.NewErrorMessage(requestID, 40332, "squad leader required"))
	}

	if requireSquadReady {
		if errMsg := validateSquadReady(requestID, squadState); errMsg != nil {
			return writeWebSocketJSON(conn, writeMu, *errMsg)
		}
	}

	currentMission, err := missionManager.GetByPlayer(playerID)
	if err != nil {
		return writeWebSocketJSON(conn, writeMu, missionErrorMessage(requestID, err))
	}
	if currentMission.SquadID != squadState.ID {
		return writeWebSocketJSON(conn, writeMu, realtimews.NewErrorMessage(requestID, 40932, "mission squad changed"))
	}

	updatedMission, err := missionManager.Transition(currentMission.ID, target)
	if err != nil {
		return writeWebSocketJSON(conn, writeMu, missionErrorMessage(requestID, err))
	}

	response := realtimews.NewServerMessage(
		resultType,
		requestID,
		realtimews.MissionData{Mission: updatedMission},
	)
	if err := writeWebSocketJSON(conn, writeMu, response); err != nil {
		return err
	}

	broadcastMissionState(wsManager, updatedMission, playerID, event)
	return nil
}

func validateSquadReady(requestID string, squadState *gamesquad.Squad) *realtimews.ServerMessage {
	for _, member := range squadState.Members {
		if !member.Online {
			message := realtimews.NewErrorMessage(requestID, 40928, "squad member is offline")
			return &message
		}
		if !member.Ready {
			message := realtimews.NewErrorMessage(requestID, 40929, "squad members are not ready")
			return &message
		}
	}
	return nil
}

func squadErrorMessage(requestID string, err error) realtimews.ServerMessage {
	switch err {
	case gamesquad.ErrPlayerAlreadyInSquad:
		return realtimews.NewErrorMessage(requestID, 40926, "player already in squad")
	case gamesquad.ErrPlayerNotInSquad:
		return realtimews.NewErrorMessage(requestID, 40426, "player not in squad")
	case gamesquad.ErrSquadNotFound:
		return realtimews.NewErrorMessage(requestID, 40427, "squad not found")
	case gamesquad.ErrSquadFull:
		return realtimews.NewErrorMessage(requestID, 40927, "squad is full")
	default:
		return realtimews.NewErrorMessage(requestID, 50026, "squad operation failed")
	}
}

func missionErrorMessage(requestID string, err error) realtimews.ServerMessage {
	switch err {
	case gamemission.ErrMissionIDRequired:
		return realtimews.NewErrorMessage(requestID, 40031, "mission_id required")
	case gamemission.ErrMissionNotFound:
		return realtimews.NewErrorMessage(requestID, 40428, "mission instance not found")
	case gamemission.ErrSquadAlreadyInMission:
		return realtimews.NewErrorMessage(requestID, 40930, "squad already has active mission")
	case gamemission.ErrInvalidStateTransition:
		return realtimews.NewErrorMessage(requestID, 40931, "invalid mission state transition")
	default:
		return realtimews.NewErrorMessage(requestID, 50027, "mission operation failed")
	}
}

func matchmakingErrorMessage(requestID string, err error) realtimews.ServerMessage {
	switch err {
	case gamematchmaking.ErrMissionIDRequired:
		return realtimews.NewErrorMessage(requestID, 40033, "matchmaking mission_id required")
	case gamematchmaking.ErrRoleRequired:
		return realtimews.NewErrorMessage(requestID, 40034, "matchmaking role required")
	case gamematchmaking.ErrInvalidMissionID, gamematchmaking.ErrInvalidRole:
		return realtimews.NewErrorMessage(requestID, 40035, "invalid matchmaking value")
	case gamematchmaking.ErrTicketNotFound:
		return realtimews.NewErrorMessage(requestID, 40429, "matchmaking ticket not found")
	case gamematchmaking.ErrAlreadyQueued:
		return realtimews.NewErrorMessage(requestID, 40933, "player already queued")
	case gamematchmaking.ErrTicketNotQueued:
		return realtimews.NewErrorMessage(requestID, 40934, "matchmaking ticket not queued")
	case gamematchmaking.ErrTicketExpired:
		return realtimews.NewErrorMessage(requestID, 40935, "matchmaking ticket expired")
	default:
		return realtimews.NewErrorMessage(requestID, 50028, "matchmaking operation failed")
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
