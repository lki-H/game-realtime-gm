package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"game-realtime-gm/backend/internal/auth"
	"game-realtime-gm/backend/internal/middleware"
	"game-realtime-gm/backend/internal/pve"
	"game-realtime-gm/backend/internal/pve/party"
	"game-realtime-gm/backend/internal/pve/run"
	"game-realtime-gm/backend/internal/pve/social"
	"game-realtime-gm/backend/internal/pve/store"
	"game-realtime-gm/backend/internal/pve/task"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type V2Transport struct {
	App              *pve.App
	Secret           string
	mu               sync.Mutex
	lifecycle        [64]sync.Mutex
	clients          map[int64]*v2Client
	ctx              context.Context
	CommandRateLimit int
	MaxConnections   int
	MaxPerIP         int
	pending          map[int64]string
}
type v2Client struct {
	id     string
	claims *auth.Claims
	conn   *websocket.Conn
	send   chan any
	ip     string
}

func (client *v2Client) enqueue(value any) bool {
	select {
	case client.send <- value:
		return true
	default:
		_ = client.conn.Close()
		return false
	}
}

type v2Envelope struct {
	Type          string          `json:"type"`
	SchemaVersion int             `json:"schema_version"`
	RequestID     string          `json:"request_id,omitempty"`
	OperationID   string          `json:"operation_id,omitempty"`
	Data          json.RawMessage `json:"data"`
}

func NewV2Transport(ctx context.Context, app *pve.App, secret string) *V2Transport {
	transport := &V2Transport{App: app, Secret: secret, clients: map[int64]*v2Client{}, ctx: ctx, CommandRateLimit: 120, MaxConnections: 256, MaxPerIP: 64, pending: map[int64]string{}}
	app.Worker.Notify = transport.Notify
	app.Runs.Connection = func(player int64) string {
		transport.mu.Lock()
		defer transport.mu.Unlock()
		if client := transport.clients[player]; client != nil {
			return client.id
		}
		return ""
	}
	return transport
}
func (transport *V2Transport) Notify(players []int64, kind string, data json.RawMessage) {
	transport.mu.Lock()
	defer transport.mu.Unlock()
	for _, player := range players {
		if client := transport.clients[player]; client != nil {
			select {
			case client.send <- gin.H{"schema_version": 2, "type": kind, "data": data}:
			default:
				_ = client.conn.Close()
			}
		}
	}
}
func (transport *V2Transport) Close() {
	transport.mu.Lock()
	defer transport.mu.Unlock()
	for _, client := range transport.clients {
		_ = client.conn.Close()
	}
}
func (transport *V2Transport) Auth(c *gin.Context) {
	token := strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
	claims, err := auth.ParseToken(transport.Secret, token)
	if err != nil {
		c.AbortWithStatusJSON(401, gin.H{"code": 40104, "message": "invalid token"})
		return
	}
	if err := pve.Authorize(c.Request.Context(), transport.App.DB, claims); err != nil {
		v2SessionFailure(c, err)
		return
	}
	c.Set("v2_player", claims.PlayerID)
	c.Next()
}

func V2Session(db *sql.DB, secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims, err := auth.ParseToken(secret, strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer "))
		if err != nil {
			c.AbortWithStatusJSON(403, gin.H{"code": 40321, "message": "player session revoked or unavailable"})
			return
		}
		if err := pve.Authorize(c.Request.Context(), db, claims); err != nil {
			v2SessionFailure(c, err)
			return
		}
		c.Next()
	}
}
func v2SessionFailure(c *gin.Context, err error) {
	if errors.Is(err, store.Forbidden) {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": 40321, "message": "player session revoked"})
		return
	}
	c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"code": 50370, "message": "player session verification unavailable"})
}
func (transport *V2Transport) WebSocket(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		token = strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
	}
	claims, err := auth.ParseToken(transport.Secret, token)
	if err != nil {
		c.JSON(401, gin.H{"code": 40122, "message": "invalid websocket token"})
		return
	}
	if err := pve.Authorize(c.Request.Context(), transport.App.DB, claims); err != nil {
		v2SessionFailure(c, err)
		return
	}
	if !transport.reserve(claims.PlayerID, c.ClientIP()) {
		c.JSON(http.StatusTooManyRequests, gin.H{"code": 42972, "message": "websocket connection quota reached"})
		return
	}
	upgrader := websocket.Upgrader{CheckOrigin: func(request *http.Request) bool {
		origin := request.Header.Get("Origin")
		return origin == "" || origin == "http://"+request.Host || origin == "https://"+request.Host
	}}
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		transport.mu.Lock()
		delete(transport.pending, claims.PlayerID)
		transport.mu.Unlock()
		return
	}
	client := &v2Client{id: store.ID("connection"), claims: claims, conn: conn, send: make(chan any, 64), ip: c.ClientIP()}
	conn.SetReadLimit(16384)
	_ = conn.SetReadDeadline(time.Now().Add(70 * time.Second))
	conn.SetPongHandler(func(string) error { return conn.SetReadDeadline(time.Now().Add(70 * time.Second)) })
	defer func() {
		_ = conn.Close()
		lifecycle := &transport.lifecycle[uint64(claims.PlayerID)%uint64(len(transport.lifecycle))]
		lifecycle.Lock()
		defer lifecycle.Unlock()
		transport.mu.Lock()
		current := transport.clients[claims.PlayerID] == client
		if current {
			delete(transport.clients, claims.PlayerID)
		}
		transport.mu.Unlock()
		if current {
			cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			_, _ = transport.App.DB.ExecContext(cleanup, "UPDATE pve_party_members SET ready=0 WHERE player_id=? AND status='active'", claims.PlayerID)
			_, _ = transport.App.DB.ExecContext(cleanup, "UPDATE pve_recruitment_preferences SET ready=0 WHERE player_id=?", claims.PlayerID)
			_ = transport.App.Runs.Connect(cleanup, claims.PlayerID, client.id, false)
			cancel()
		}
	}()
	lifecycle := &transport.lifecycle[uint64(claims.PlayerID)%uint64(len(transport.lifecycle))]
	lifecycle.Lock()
	transport.mu.Lock()
	if prior := transport.clients[claims.PlayerID]; prior != nil {
		_ = prior.conn.Close()
	}
	transport.clients[claims.PlayerID] = client
	delete(transport.pending, claims.PlayerID)
	transport.mu.Unlock()
	connectionContext, cancelConnection := context.WithTimeout(transport.ctx, 3*time.Second)
	err = transport.App.Runs.Connect(connectionContext, claims.PlayerID, client.id, true)
	cancelConnection()
	lifecycle.Unlock()
	if err != nil {
		return
	}
	done := make(chan struct{})
	writerExited := make(chan struct{})
	defer func() { close(done); _ = conn.Close(); <-writerExited }()
	go func() {
		defer close(writerExited)
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-transport.ctx.Done():
				_ = conn.Close()
				return
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(transport.ctx, 2*time.Second)
				err := pve.Authorize(ctx, transport.App.DB, claims)
				cancel()
				if err != nil {
					_ = conn.Close()
					return
				}
				_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
				if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)); err != nil {
					_ = conn.Close()
					return
				}
			case value := <-client.send:
				_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
				if err := conn.WriteJSON(value); err != nil {
					_ = conn.Close()
					return
				}
			}
		}
	}()
	client.send <- gin.H{"schema_version": 2, "type": "server.welcome", "data": gin.H{"connection_id": client.id, "player_id": claims.PlayerID}}
	for {
		kind, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if kind != websocket.TextMessage {
			return
		}
		limitContext, limitCancel := context.WithTimeout(transport.ctx, 2*time.Second)
		allowed, limitErr := middleware.TakeRateLimit(limitContext, transport.App.Projection.Redis, "v2:ratelimit:ws-command:"+strconv.FormatInt(claims.PlayerID, 10), transport.CommandRateLimit, time.Minute)
		limitCancel()
		if limitErr != nil || !allowed {
			code := 42970
			if limitErr != nil {
				code = 50370
			}
			if !client.enqueue(gin.H{"type": "server.error", "schema_version": 2, "code": code, "message": "command rate limit unavailable or exceeded"}) {
				return
			}
			continue
		}
		var message v2Envelope
		if err := json.Unmarshal(data, &message); err != nil {
			if !client.enqueue(gin.H{"type": "server.error", "schema_version": 2, "code": 40024}) {
				return
			}
			continue
		}
		if message.SchemaVersion != 2 || !strings.HasPrefix(message.Type, "v2.") {
			if !client.enqueue(gin.H{"type": "server.error", "schema_version": 2, "code": 40971, "message": "v2 gameplay mode requires v2 messages"}) {
				return
			}
			continue
		}
		ctx, cancel := context.WithTimeout(transport.ctx, 5*time.Second)
		transport.mu.Lock()
		current := transport.clients[claims.PlayerID] == client
		transport.mu.Unlock()
		if !current {
			cancel()
			return
		}
		err = pve.Authorize(ctx, transport.App.DB, claims)
		var output any
		if err == nil {
			if message.Type == "v2.run.reconnect" || message.Type == "v2.run.result" {
				var input struct {
					RunID string `json:"run_id"`
				}
				err = json.Unmarshal(message.Data, &input)
				if err == nil {
					state, snapshotErr := transport.App.Runs.Snapshot(ctx, input.RunID)
					err = snapshotErr
					if err == nil {
						if state.Member(&claims.PlayerID) == nil {
							err = store.Forbidden
						} else {
							output = state.ForPlayer(claims.PlayerID)
						}
					}
				}
			} else {
				output, err = transport.App.Command(ctx, claims.PlayerID, message.OperationID, message.Type, message.Data)
			}
		}
		cancel()
		code := 0
		text := "ok"
		if err != nil {
			code = errorCode(err)
			text = errorText(err)
		}
		if !client.enqueue(gin.H{"type": message.Type + ".result", "schema_version": 2, "request_id": message.RequestID, "code": code, "message": text, "data": output}) {
			return
		}
	}
}
func (transport *V2Transport) Query(c *gin.Context) {
	player := c.GetInt64("v2_player")
	ctx := c.Request.Context()
	path := c.FullPath()
	var value any
	var err error
	switch {
	case strings.HasSuffix(path, "/recruitment/applications"):
		value, err = transport.App.Party.Applications(ctx, player)
	case strings.HasSuffix(path, "/operations/catalog"):
		value, err = task.Operations(ctx, transport.App.DB)
	case strings.HasSuffix(path, "/operations/preview"):
		version := c.DefaultQuery("rule_version", transport.App.Runs.Rules.Version)
		value, err = task.Preview(ctx, transport.App.DB, player, version, c.Query("task_key"))
	case strings.HasSuffix(path, "/operations/recommendations"):
		value, err = task.Recommendations(ctx, transport.App.DB, player, c.Query("task_key"))
	case strings.Contains(path, "/regroup/"):
		value, err = transport.App.Party.RegroupSnapshot(ctx, c.Param("proposal_id"), player)
	case strings.Contains(path, "/recruitment/parties/"):
		value, err = transport.App.Party.RecruitmentSnapshot(ctx, c.Param("party_id"), player)
	case strings.HasSuffix(path, "/me/activity"):
		value, err = transport.activitySnapshot(ctx, player)
	case strings.Contains(path, "players/search"):
		value, err = transport.App.Social.SearchPlayers(ctx, player, c.Query("q"), 20)
	case strings.HasSuffix(path, "friends/requests"):
		value, err = transport.App.Social.Query(ctx, player, "requests", 0)
	case strings.HasSuffix(path, "friends/blocks"):
		value, err = transport.App.Social.Query(ctx, player, "blocks", 0)
	case strings.HasSuffix(path, "friends"):
		value, err = transport.App.Social.Query(ctx, player, "friends", 0)
	case strings.HasSuffix(path, "messages/unread"):
		value, err = transport.App.Social.Query(ctx, player, "unread", 0)
	case strings.HasSuffix(path, "messages"):
		peer, _ := strconv.ParseInt(c.Query("peer_id"), 10, 64)
		value, err = transport.App.Social.Query(ctx, player, "messages", peer)
	case strings.Contains(path, "parties/"):
		value, err = transport.App.Party.Snapshot(ctx, c.Param("party_id"), player)
	case strings.Contains(path, "runs/"):
		state, snapshotErr := transport.App.Runs.Snapshot(ctx, c.Param("run_id"))
		err = snapshotErr
		if err == nil {
			if state.Member(&player) == nil {
				err = store.Forbidden
			} else {
				value = state.ForPlayer(player)
				if strings.HasSuffix(path, "/results") {
					value, err = transport.resultView(ctx, state, player)
				}
			}
		}
	case strings.Contains(path, "operations/"):
		var data []byte
		err = transport.App.DB.QueryRowContext(ctx, "SELECT result FROM pve_command_results WHERE player_id=? AND operation_id=?", player, c.Param("operation_id")).Scan(&data)
		var receipt struct {
			Archived bool `json:"_pve_archived"`
		}
		if err == nil && json.Unmarshal(data, &receipt) == nil && receipt.Archived {
			err = store.Archived
		}
		value = json.RawMessage(data)
	case strings.Contains(path, "proposals/"):
		var count int
		err = transport.App.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM pve_match_proposal_members WHERE proposal_id=? AND player_id=?", c.Param("proposal_id"), player).Scan(&count)
		if err == nil && count == 0 {
			err = store.Forbidden
		}
		if err == nil {
			rows, queryErr := transport.App.DB.QueryContext(ctx, "SELECT id,revision,status,run_id,deadline_at FROM pve_match_proposals WHERE id=?", c.Param("proposal_id"))
			err = queryErr
			if err == nil {
				value, err = social.Rows(rows)
				rows.Close()
			}
		}
	case strings.HasSuffix(path, "recruitment"):
		rows, queryErr := transport.App.DB.QueryContext(ctx, "SELECT p.id,p.operation_id,p.difficulty,GREATEST(0,4-(SELECT COUNT(*) FROM pve_party_members m WHERE m.party_id=p.party_id AND m.status='active')-(SELECT COUNT(*) FROM pve_recruitment_preferences q WHERE q.party_id=p.party_id AND q.expires_at>UTC_TIMESTAMP(3))) AS slots,p.tags,p.expires_at FROM pve_recruitment_posts p JOIN pve_parties g ON g.id=p.party_id WHERE p.status='open' AND g.status='open' AND p.owner_id=g.owner_id AND p.expires_at>UTC_TIMESTAMP(3) AND NOT EXISTS(SELECT 1 FROM pve_social_blocks b WHERE (b.blocker_id=? AND b.blocked_id=p.owner_id) OR (b.blocker_id=p.owner_id AND b.blocked_id=?)) ORDER BY p.created_at DESC LIMIT 50", player, player)
		err = queryErr
		if err == nil {
			value, err = social.Rows(rows)
			rows.Close()
		}
	case strings.HasSuffix(path, "tasks"):
		value, err = task.Catalogue(ctx, transport.App.DB, player, c.DefaultQuery("rule_version", transport.App.Runs.Rules.Version))
	default:
		err = store.NotFound
	}
	if err != nil {
		status := 500
		switch errorCode(err) {
		case 40070:
			status = 400
		case 40370:
			status = 403
		case 40470:
			status = 404
		case 40970:
			status = 409
		case 41071:
			status = 410
		}
		c.JSON(status, gin.H{"code": errorCode(err), "message": errorText(err)})
		return
	}
	c.JSON(200, gin.H{"code": 0, "message": "ok", "data": value})
}

func (transport *V2Transport) activitySnapshot(ctx context.Context, player int64) (map[string]any, error) {
	transaction, err := transport.App.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer transaction.Rollback()
	result := map[string]any{"player_id": player}
	var activityType, activityID string
	err = transaction.QueryRowContext(ctx, "SELECT activity_type,activity_id FROM pve_player_activity_locks WHERE player_id=?", player).Scan(&activityType, &activityID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if err == nil {
		result["activity_lock"] = map[string]string{"type": activityType, "id": activityID}
	}
	var partyID sql.NullString
	partyErr := transaction.QueryRowContext(ctx, "SELECT party_id FROM pve_party_members WHERE player_id=? AND status='active' LIMIT 1", player).Scan(&partyID)
	if partyErr != nil && !errors.Is(partyErr, sql.ErrNoRows) {
		return nil, partyErr
	}
	if partyErr == nil && partyID.Valid {
		partySnapshot, snapshotErr := party.Read(ctx, transaction, partyID.String, player)
		if snapshotErr != nil {
			return nil, snapshotErr
		}
		result["party"] = partySnapshot
		if activityID == "" && partySnapshot.Status == "in_run" {
			var runID string
			if err := transaction.QueryRowContext(ctx, "SELECT p.run_id FROM pve_run_participants p JOIN pve_runs r ON r.id=p.run_id WHERE p.player_id=? AND r.status='ending' ORDER BY r.created_at DESC LIMIT 1", player).Scan(&runID); err == nil {
				activityID = runID
				result["settlement_run_id"] = runID
			} else if !errors.Is(err, sql.ErrNoRows) {
				return nil, err
			}
		}
	}
	if activityType == "ticket" {
		var ticket map[string]any
		rows, queryErr := transaction.QueryContext(ctx, "SELECT id,source_party_id,operation_name,difficulty,status,fill_policy,allow_partial,queue_priority_since,stage_entered_at,deadline_at FROM pve_match_tickets WHERE id=?", activityID)
		if queryErr != nil {
			return nil, queryErr
		}
		items, rowsErr := social.Rows(rows)
		rows.Close()
		if rowsErr != nil {
			return nil, rowsErr
		}
		if len(items) > 0 {
			ticket = items[0]
			result["ticket"] = ticket
		}
	}
	if activityType == "run" {
		var content []byte
		snapshotErr := transaction.QueryRowContext(ctx, "SELECT state FROM pve_runs WHERE id=?", activityID).Scan(&content)
		if snapshotErr != nil {
			return nil, snapshotErr
		}
		var state run.State
		if err := json.Unmarshal(content, &state); err != nil {
			return nil, err
		}
		if state.Member(&player) == nil {
			return nil, store.Forbidden
		}
		result["run"] = state.ForPlayer(player)
		rows, queryErr := transaction.QueryContext(ctx, "SELECT result_status,contribution_qualified,task_completed,settled_at FROM pve_participant_results WHERE run_id=? AND player_id=?", activityID, player)
		if queryErr != nil {
			return nil, queryErr
		}
		items, rowsErr := social.Rows(rows)
		rows.Close()
		if rowsErr != nil {
			return nil, rowsErr
		}
		if len(items) > 0 {
			result["participant_result"] = items[0]
		}
	}
	rows, queryErr := transaction.QueryContext(ctx, "SELECT p.id,p.revision,p.status,p.run_id,p.deadline_at FROM pve_match_proposals p JOIN pve_match_proposal_members m ON m.proposal_id=p.id WHERE m.player_id=? AND p.status='pending' ORDER BY p.created_at DESC LIMIT 1", player)
	if queryErr != nil {
		return nil, queryErr
	}
	items, rowsErr := social.Rows(rows)
	rows.Close()
	if rowsErr != nil {
		return nil, rowsErr
	}
	if len(items) > 0 {
		result["proposal"] = items[0]
	}
	rows, queryErr = transaction.QueryContext(ctx, "SELECT operation_id,operation_type,aggregate_id,status,attempts,last_error FROM pve_pending_operations WHERE aggregate_id=? AND status NOT IN ('done') ORDER BY created_at LIMIT 20", activityID)
	if queryErr != nil {
		return nil, queryErr
	}
	items, rowsErr = social.Rows(rows)
	rows.Close()
	if rowsErr != nil {
		return nil, rowsErr
	}
	result["pending_operations"] = items
	rows, queryErr = transaction.QueryContext(ctx, "SELECT r.run_id,r.result_status,r.contribution_qualified,r.task_completed,r.settled_at FROM pve_participant_results r JOIN pve_runs v ON v.id=r.run_id WHERE r.player_id=? ORDER BY v.created_at DESC LIMIT 1", player)
	if queryErr != nil {
		return nil, queryErr
	}
	items, rowsErr = social.Rows(rows)
	rows.Close()
	if rowsErr != nil {
		return nil, rowsErr
	}
	if len(items) > 0 {
		result["latest_result"] = items[0]
	}
	rows, queryErr = transaction.QueryContext(ctx, "SELECT party_id,ready,selection_version,task_selection,expires_at FROM pve_recruitment_preferences WHERE player_id=? AND expires_at>UTC_TIMESTAMP(3)", player)
	if queryErr != nil {
		return nil, queryErr
	}
	items, rowsErr = social.Rows(rows)
	rows.Close()
	if rowsErr != nil {
		return nil, rowsErr
	}
	result["recruitment"] = items
	rows, queryErr = transaction.QueryContext(ctx, "SELECT p.id,p.revision,p.status,p.expires_at FROM pve_regroup_proposals p JOIN pve_regroup_members m ON m.proposal_id=p.id WHERE m.player_id=? AND p.status='pending' AND p.expires_at>UTC_TIMESTAMP(3) ORDER BY p.created_at DESC LIMIT 10", player)
	if queryErr != nil {
		return nil, queryErr
	}
	items, rowsErr = social.Rows(rows)
	rows.Close()
	if rowsErr != nil {
		return nil, rowsErr
	}
	result["regroup_proposals"] = items
	return result, transaction.Commit()
}
func (transport *V2Transport) resultView(ctx context.Context, state *run.State, player int64) (map[string]any, error) {
	value := map[string]any{}
	if err := json.Unmarshal(store.JSON(state.ForPlayer(player)), &value); err != nil {
		return nil, err
	}
	rows, err := transport.App.DB.QueryContext(ctx, "SELECT result_status,contribution_qualified,task_completed,settled_at FROM pve_participant_results WHERE run_id=? AND player_id=?", state.ID, player)
	if err != nil {
		return nil, err
	}
	items, err := social.Rows(rows)
	rows.Close()
	if err != nil {
		return nil, err
	}
	if len(items) > 0 {
		value["participant_result"] = items[0]
	}
	rows, err = transport.App.DB.QueryContext(ctx, "SELECT reward_source,asset_type,amount,status FROM pve_reward_grants WHERE run_id=? AND player_id=? ORDER BY id", state.ID, player)
	if err != nil {
		return nil, err
	}
	items, err = social.Rows(rows)
	rows.Close()
	if err != nil {
		return nil, err
	}
	value["reward_grants"] = items
	return value, nil
}
func errorCode(err error) int {
	switch {
	case errors.Is(err, store.Archived):
		return 41071
	case errors.Is(err, store.Invalid):
		return 40070
	case errors.Is(err, store.Forbidden):
		return 40370
	case errors.Is(err, store.NotFound), errors.Is(err, sql.ErrNoRows):
		return 40470
	case errors.Is(err, store.Conflict):
		return 40970
	}
	return 50070
}

func (transport *V2Transport) reserve(player int64, ip string) bool {
	transport.mu.Lock()
	defer transport.mu.Unlock()
	if _, exists := transport.pending[player]; exists {
		return false
	}
	prior := transport.clients[player]
	total := len(transport.clients) + len(transport.pending)
	if prior != nil {
		total--
	}
	if total >= transport.MaxConnections {
		return false
	}
	fromIP := 0
	for otherPlayer, client := range transport.clients {
		if otherPlayer != player && client.ip == ip {
			fromIP++
		}
	}
	for _, pendingIP := range transport.pending {
		if pendingIP == ip {
			fromIP++
		}
	}
	if fromIP >= transport.MaxPerIP {
		return false
	}
	transport.pending[player] = ip
	return true
}
func errorText(err error) string {
	if errorCode(err) == 50070 {
		return "v2 operation failed"
	}
	return err.Error()
}
