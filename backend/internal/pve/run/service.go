package run

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"

	"game-realtime-gm/backend/internal/pve/store"
	"game-realtime-gm/backend/internal/pve/task"
)

var ErrInvalidCreate = store.Invalid
var ErrRunNotFound = store.NotFound
var ErrInvalidEvent = store.Invalid
var ErrEventConflict = store.Conflict
var ErrRunNotRunning = store.Conflict
var ErrSourceGeneration = store.Forbidden
var ErrRunAlreadyTerminal = store.Conflict

type Config struct{ Rules task.Rules }
type TaskSelection struct {
	TaskKey     string `json:"task_key"`
	TaskVersion string `json:"task_version"`
}
type CreateRequest struct {
	RuleVersion   string
	OperationName string
	Difficulty    string
	PlayerIDs     []int64
	Tasks         map[int64]TaskSelection
	Tickets       map[int64]string
}
type Event struct {
	EventID          string          `json:"event_id"`
	Source           string          `json:"source"`
	SourceGeneration int64           `json:"source_generation"`
	RunID            string          `json:"run_id"`
	Sequence         int64           `json:"sequence"`
	SchemaVersion    int             `json:"schema_version"`
	EventType        string          `json:"event_type"`
	ActorPlayerID    *int64          `json:"actor_player_id,omitempty"`
	Contributors     []int64         `json:"contributors,omitempty"`
	TargetID         string          `json:"target_id,omitempty"`
	Payload          json.RawMessage `json:"payload,omitempty"`
	OccurredAt       time.Time       `json:"occurred_at"`
}
type EventResult struct {
	EventID          string `json:"event_id"`
	Status           string `json:"status"`
	AppliedSequence  int64  `json:"applied_sequence"`
	ReceivedSequence int64  `json:"received_sequence"`
	Duplicate        bool   `json:"duplicate"`
	GapDetected      bool   `json:"gap_detected"`
	RunClosed        bool   `json:"run_closed"`
}
type Participant struct {
	ParticipantID  string           `json:"participant_id"`
	PlayerID       int64            `json:"player_id"`
	TicketID       string           `json:"ticket_id,omitempty"`
	Status         string           `json:"status"`
	LifeStatus     string           `json:"life_status"`
	Connected      bool             `json:"connected"`
	ConnectionID   string           `json:"connection_id,omitempty"`
	DisconnectedAt *time.Time       `json:"disconnected_at,omitempty"`
	LeftSequence   int64            `json:"left_sequence,omitempty"`
	Deaths         int              `json:"deaths"`
	Contribution   int              `json:"contribution"`
	TaskKey        string           `json:"task_key,omitempty"`
	TaskVersion    string           `json:"task_version,omitempty"`
	TaskPeriod     string           `json:"task_period,omitempty"`
	TaskAttemptID  string           `json:"task_attempt_id,omitempty"`
	Compatibility  string           `json:"compatibility_status"`
	TaskProgress   map[string]int64 `json:"task_progress,omitempty"`
	TaskCompleted  bool             `json:"task_completed"`
}
type Spawn struct {
	PlayerID int64     `json:"player_id"`
	Deaths   int       `json:"deaths"`
	Status   string    `json:"status"`
	Deadline time.Time `json:"deadline"`
}
type State struct {
	ID                  string           `json:"id"`
	OperationName       string           `json:"operation_name"`
	Difficulty          string           `json:"difficulty"`
	Status              string           `json:"status"`
	EndReason           string           `json:"end_reason,omitempty"`
	Rules               task.Rules       `json:"rules"`
	SourceGeneration    int64            `json:"source_generation"`
	Participants        []Participant    `json:"participants"`
	Objectives          map[string]int64 `json:"objectives"`
	Spawns              map[string]Spawn `json:"spawns"`
	Facts               map[string]bool  `json:"facts"`
	ReinforcementBudget int              `json:"reinforcement_budget"`
	ReinforcementUsed   int              `json:"reinforcement_used"`
	AppliedSequence     int64            `json:"applied_sequence"`
	ReceivedSequence    int64            `json:"received_sequence"`
	HighestSeen         int64            `json:"highest_seen_sequence"`
	FinalSequence       int64            `json:"final_sequence"`
	GapDeadline         *time.Time       `json:"gap_deadline,omitempty"`
	DeadlineAt          time.Time        `json:"deadline_at"`
	StartedAt           *time.Time       `json:"started_at,omitempty"`
	EndedAt             *time.Time       `json:"ended_at,omitempty"`
	Settled             bool             `json:"settled"`
}
type Snapshot = State
type Run = State
type Service struct {
	DB         *sql.DB
	Rules      task.Rules
	Now        func() time.Time
	Connection func(int64) string
}

func NewService(db *sql.DB, cfg Config) *Service {
	return &Service{DB: db, Rules: cfg.Rules, Now: time.Now}
}

func (s *Service) Create(ctx context.Context, request CreateRequest) (*State, error) {
	var state *State
	err := store.Transaction(ctx, s.DB, func(transaction *sql.Tx) error {
		var err error
		state, err = s.CreateTx(ctx, transaction, request)
		return err
	})
	return state, err
}
func (s *Service) CreateTx(ctx context.Context, transaction *sql.Tx, request CreateRequest) (*State, error) {
	version := request.RuleVersion
	if version == "" {
		version = s.Rules.Version
	}
	rules, err := task.RuleTx(ctx, transaction, request.OperationName, version)
	if err != nil {
		return nil, err
	}
	if request.OperationName != rules.Operation || request.Difficulty != rules.Difficulty || len(request.PlayerIDs) < 1 || len(request.PlayerIDs) > 4 {
		return nil, store.Invalid
	}
	seen := map[int64]bool{}
	for _, player := range request.PlayerIDs {
		var status string
		if err := transaction.QueryRowContext(ctx, "SELECT status FROM players WHERE id=?", player).Scan(&status); err != nil {
			return nil, err
		}
		if status != "normal" {
			return nil, store.Forbidden
		}
		if player <= 0 || seen[player] {
			return nil, store.Invalid
		}
		seen[player] = true
	}
	now := s.Now().UTC()
	state := &State{ID: store.ID("run"), OperationName: request.OperationName, Difficulty: request.Difficulty, Status: "loading", Rules: rules, SourceGeneration: 1, Objectives: map[string]int64{}, Spawns: map[string]Spawn{}, Facts: map[string]bool{}, ReinforcementBudget: rules.Reinforcements, DeadlineAt: now.Add(time.Duration(rules.Loading) * time.Second)}
	sort.Slice(request.PlayerIDs, func(first, second int) bool { return request.PlayerIDs[first] < request.PlayerIDs[second] })
	for _, player := range request.PlayerIDs {
		member := Participant{ParticipantID: store.ID("participant"), PlayerID: player, TicketID: request.Tickets[player], Status: "loading", LifeStatus: "alive", Connected: true, Compatibility: "none", TaskProgress: map[string]int64{}}
		if s.Connection != nil {
			member.ConnectionID = s.Connection(player)
			member.Connected = member.ConnectionID != ""
		}
		selection := request.Tasks[player]
		if selection.TaskKey != "" {
			definition, exists := rules.Task(selection.TaskKey)
			if !exists || selection.TaskVersion != definition.LogicalVersion(rules) {
				return nil, store.Invalid
			}
			member.TaskKey = selection.TaskKey
			member.TaskVersion = definition.LogicalVersion(rules)
			member.TaskPeriod = definition.Period(now)
			member.TaskAttemptID = store.ID("attempt")
			member.Compatibility = "compatible"
			if definition.Operation != rules.Operation {
				member.Compatibility = "incompatible"
			}
			progress, err := task.LoadProgress(ctx, transaction, player, definition, rules, member.TaskPeriod)
			if err != nil {
				return nil, err
			}
			if progress.Completed || !progress.Unlocked {
				return nil, store.Conflict
			}
			if progress.Paused {
				if _, err := transaction.ExecContext(ctx, "UPDATE pve_task_pauses SET paused=0 WHERE player_id=? AND task_key=? AND task_version=?", player, definition.Key, definition.LogicalVersion(rules)); err != nil {
					return nil, err
				}
			}
			member.TaskProgress = progress.Counts
			if _, err := transaction.ExecContext(ctx, "INSERT INTO pve_player_task_attempts(id,run_id,player_id,task_key,task_version,compatibility_status,status) VALUES(?,?,?,?,?,?,'provisional')", member.TaskAttemptID, state.ID, player, member.TaskKey, member.TaskVersion, member.Compatibility); err != nil {
				return nil, err
			}
			for _, objective := range definition.Objectives {
				if _, err := transaction.ExecContext(ctx, "INSERT INTO pve_player_task_progress(attempt_id,objective_key,objective_type,contribution_scope,required_count,progress) VALUES(?,?,?,?,?,?)", member.TaskAttemptID, objective.Key, objective.Type, objective.Scope, objective.Required, member.TaskProgress[objective.Key]); err != nil {
					return nil, err
				}
			}
		}
		state.Participants = append(state.Participants, member)
		if _, err := transaction.ExecContext(ctx, "INSERT INTO pve_run_participants(participant_id,run_id,player_id,source_ticket_id,status,life_status,task_attempt_id) VALUES(?,?,?,?,'loading','alive',?)", member.ParticipantID, state.ID, player, member.TicketID, null(member.TaskAttemptID)); err != nil {
			return nil, err
		}
	}
	if _, err := transaction.ExecContext(ctx, "INSERT INTO pve_runs(id,operation_name,difficulty,status,rule_version,state,source_generation,reinforcement_budget,deadline_at) VALUES(?,?,?,'loading',?,?,1,?,?)", state.ID, state.OperationName, state.Difficulty, rules.Version, store.JSON(state), state.ReinforcementBudget, state.DeadlineAt); err != nil {
		return nil, err
	}
	for _, objective := range rules.Objectives {
		if _, err := transaction.ExecContext(ctx, "INSERT INTO pve_run_objectives(run_id,objective_key,objective_type,required_count) VALUES(?,?,?,?)", state.ID, objective.Key, objective.Type, objective.Required); err != nil {
			return nil, err
		}
	}
	return state, nil
}
func (s *Service) CheckAdmissionTx(ctx context.Context, transaction *sql.Tx) error {
	var status, hash string
	if err := transaction.QueryRowContext(ctx, "SELECT status,content_hash FROM pve_rule_versions WHERE rule_key=? AND version=? FOR SHARE", s.Rules.Operation, s.Rules.Version).Scan(&status, &hash); err != nil {
		return err
	}
	if status != "published" || hash != store.Hash(store.JSON(s.Rules)) {
		return store.Conflict
	}
	return nil
}
func Load(ctx context.Context, transaction *sql.Tx, id string) (*State, error) {
	var data []byte
	err := transaction.QueryRowContext(ctx, "SELECT state FROM pve_runs WHERE id=? FOR UPDATE", id).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, store.NotFound
	}
	if err != nil {
		return nil, err
	}
	var state State
	err = json.Unmarshal(data, &state)
	return &state, err
}
func Save(ctx context.Context, transaction *sql.Tx, state *State) error {
	_, err := transaction.ExecContext(ctx, "UPDATE pve_runs SET state=?,status=?,end_reason=?,source_generation=?,reinforcement_used=?,reinforcement_reserved=?,started_at=?,deadline_at=?,final_sequence=?,ended_at=?,updated_at=UTC_TIMESTAMP(3) WHERE id=?", store.JSON(state), state.Status, null(state.EndReason), state.SourceGeneration, state.ReinforcementUsed, pendingSpawns(state), state.StartedAt, state.DeadlineAt, state.FinalSequence, state.EndedAt, state.ID)
	if err != nil {
		return err
	}
	for _, member := range state.Participants {
		if _, err := transaction.ExecContext(ctx, "UPDATE pve_run_participants SET status=?,life_status=?,left_sequence=?,updated_at=UTC_TIMESTAMP(3) WHERE participant_id=?", member.Status, member.LifeStatus, member.LeftSequence, member.ParticipantID); err != nil {
			return err
		}
		if member.TaskAttemptID != "" {
			status := ""
			if terminal(state) {
				status = "closed"
			} else if state.Status == "running" {
				status = "active"
			}
			if status != "" {
				if _, err := transaction.ExecContext(ctx, "UPDATE pve_player_task_attempts SET status=? WHERE id=? AND status='provisional'", status, member.TaskAttemptID); err != nil {
					return err
				}
			}
		}
	}
	for action, spawn := range state.Spawns {
		if _, err := transaction.ExecContext(ctx, "INSERT INTO pve_reinforcement_actions(action_id,run_id,player_id,status,deadline_at) VALUES(?,?,?,?,?) ON DUPLICATE KEY UPDATE status=VALUES(status),resolved_at=CASE WHEN VALUES(status)<>'pending_spawn' THEN UTC_TIMESTAMP(3) ELSE resolved_at END", action, state.ID, spawn.PlayerID, spawn.Status, spawn.Deadline); err != nil {
			return err
		}
	}
	for _, objective := range state.Rules.Objectives {
		status := "active"
		if state.Objectives[objective.Key] >= objective.Required {
			status = "completed"
		}
		if _, err := transaction.ExecContext(ctx, "UPDATE pve_run_objectives SET progress=?,status=? WHERE run_id=? AND objective_key=?", state.Objectives[objective.Key], status, state.ID, objective.Key); err != nil {
			return err
		}
	}
	return nil
}
func (s *Service) Snapshot(ctx context.Context, id string) (*State, error) {
	var data []byte
	err := s.DB.QueryRowContext(ctx, "SELECT state FROM pve_runs WHERE id=?", id).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, store.NotFound
	}
	if err != nil {
		return nil, err
	}
	var state State
	err = json.Unmarshal(data, &state)
	return &state, err
}

func (s *Service) ApplyEvent(ctx context.Context, event Event) (*EventResult, error) {
	if strings.TrimSpace(event.EventID) == "" || len(event.EventID) > 128 || event.RunID == "" || event.Source != "pve_event_bot" || event.Sequence <= 0 || event.SchemaVersion != 2 || event.OccurredAt.IsZero() || event.SourceGeneration <= 0 || !eventType(event.EventType) || len(event.Contributors) > 4 || len(event.TargetID) > 128 {
		return nil, store.Invalid
	}
	var result *EventResult
	rejected := false
	var applicationError error
	err := store.Transaction(ctx, s.DB, func(transaction *sql.Tx) error {
		state, err := Load(ctx, transaction, event.RunID)
		if err != nil {
			return err
		}
		fingerprint := store.Hash(store.JSON(event))
		var priorHash, priorStatus string
		err = transaction.QueryRowContext(ctx, "SELECT fingerprint,status FROM pve_run_events WHERE event_id=? AND run_id=?", event.EventID, event.RunID).Scan(&priorHash, &priorStatus)
		if err == nil {
			if priorHash != fingerprint {
				return store.Conflict
			}
			if priorStatus == "rejected" {
				return store.Conflict
			}
			result = &EventResult{EventID: event.EventID, Status: priorStatus, Duplicate: true, AppliedSequence: state.AppliedSequence, ReceivedSequence: state.ReceivedSequence, GapDetected: state.GapDeadline != nil, RunClosed: terminal(state)}
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if terminal(state) {
			return store.Conflict
		}
		if event.SourceGeneration != state.SourceGeneration {
			return store.Forbidden
		}
		now := s.Now().UTC()
		if !now.Before(state.DeadlineAt) && state.Status == "loading" {
			return store.Conflict
		}
		if reason := expiredReason(state, now); reason != "" {
			finish(state, reason, state.AppliedSequence, now)
			if err := Save(ctx, transaction, state); err != nil {
				return err
			}
			if err := QueueSettlement(ctx, transaction, state); err != nil {
				return err
			}
			rejected = true
			return nil
		}
		contributors := map[int64]bool{}
		for _, player := range event.Contributors {
			member := state.Member(&player)
			if member == nil || contributors[player] {
				return store.Invalid
			}
			contributors[player] = true
		}
		if event.Sequence <= state.AppliedSequence || event.Sequence > state.AppliedSequence+128 {
			return store.Conflict
		}
		if event.Sequence == state.AppliedSequence+1 {
			var candidate State
			if err := json.Unmarshal(store.JSON(state), &candidate); err != nil {
				return err
			}
			if err := Apply(&candidate, event, now); err != nil {
				return err
			}
		}
		_, err = transaction.ExecContext(ctx, "INSERT INTO pve_run_events(event_id,run_id,source,source_generation,sequence_no,event_type,actor_player_id,contributors,target_id,payload,status,fingerprint,occurred_at) VALUES(?,?,?,?,?,?,?,?,?,?,'received',?,?)", event.EventID, event.RunID, event.Source, event.SourceGeneration, event.Sequence, event.EventType, event.ActorPlayerID, store.JSON(event.Contributors), event.TargetID, store.JSON(event), fingerprint, event.OccurredAt.UTC())
		if err != nil {
			return err
		}
		if event.Sequence > state.HighestSeen {
			state.HighestSeen = event.Sequence
		}
		for state.ReceivedSequence < state.HighestSeen {
			var exists int
			if err := transaction.QueryRowContext(ctx, "SELECT COUNT(*) FROM pve_run_events WHERE run_id=? AND source='pve_event_bot' AND source_generation=? AND sequence_no=?", state.ID, state.SourceGeneration, state.ReceivedSequence+1).Scan(&exists); err != nil {
				return err
			}
			if exists == 0 {
				break
			}
			state.ReceivedSequence++
		}
		for !terminal(state) {
			var data []byte
			err := transaction.QueryRowContext(ctx, "SELECT payload FROM pve_run_events WHERE run_id=? AND source='pve_event_bot' AND source_generation=? AND sequence_no=?", state.ID, state.SourceGeneration, state.AppliedSequence+1).Scan(&data)
			if errors.Is(err, sql.ErrNoRows) {
				break
			}
			if err != nil {
				return err
			}
			var next Event
			if err := json.Unmarshal(data, &next); err != nil {
				return err
			}
			previous := map[int64]map[string]int64{}
			previousContribution := map[int64]int{}
			for _, member := range state.Participants {
				previousContribution[member.PlayerID] = member.Contribution
				progress := map[string]int64{}
				for key, value := range member.TaskProgress {
					progress[key] = value
				}
				previous[member.PlayerID] = progress
			}
			copyData := store.JSON(state)
			eventStatus := "applied"
			if err := Apply(state, next, now); err != nil {
				if err := json.Unmarshal(copyData, state); err != nil {
					return err
				}
				eventStatus = "rejected"
				applicationError = err
				if _, updateErr := transaction.ExecContext(ctx, "UPDATE pve_run_events SET status='rejected' WHERE event_id=?", next.EventID); updateErr != nil {
					return updateErr
				}
				finish(state, "event_rejected", state.AppliedSequence, now)
				break
			}
			state.AppliedSequence = next.Sequence
			if _, err := transaction.ExecContext(ctx, "UPDATE pve_run_events SET status=? WHERE event_id=?", eventStatus, next.EventID); err != nil {
				return err
			}
			if err := persistProgress(ctx, transaction, state, next, previous, previousContribution); err != nil {
				return err
			}
		}
		if state.HighestSeen > state.AppliedSequence && !terminal(state) {
			if state.GapDeadline == nil {
				deadline := s.Now().UTC().Add(time.Duration(state.Rules.Gap) * time.Second)
				state.GapDeadline = &deadline
			}
		} else {
			state.GapDeadline = nil
		}
		if err := Save(ctx, transaction, state); err != nil {
			return err
		}
		if terminal(state) {
			if err := QueueSettlement(ctx, transaction, state); err != nil {
				return err
			}
		}
		if err := store.Notify(ctx, transaction, state.PlayerIDs(), "v2.run.changed", map[string]any{"run_id": state.ID, "status": state.Status, "applied_sequence": state.AppliedSequence}); err != nil {
			return err
		}
		result = &EventResult{EventID: event.EventID, Status: "received", AppliedSequence: state.AppliedSequence, ReceivedSequence: state.ReceivedSequence, GapDetected: state.GapDeadline != nil, RunClosed: terminal(state)}
		if state.AppliedSequence >= event.Sequence {
			result.Status = "applied"
		}
		return nil
	})
	if err == nil && rejected {
		return nil, store.Conflict
	}
	if err == nil && applicationError != nil {
		return result, applicationError
	}
	return result, err
}

func Apply(state *State, event Event, now time.Time) error {
	if state.Status == "ending" || state.Status == "closed" || state.Status == "aborted" {
		return store.Conflict
	}
	member := state.Member(event.ActorPlayerID)
	active := member != nil && member.Status == "participating"
	switch event.EventType {
	case "loaded":
		if state.Status != "loading" || member == nil || member.Status != "loading" {
			return store.Conflict
		}
		member.Status = "participating"
		all := true
		for _, participant := range state.Participants {
			all = all && participant.Status == "participating"
		}
		if all {
			state.Status = "running"
			state.StartedAt = &now
			state.DeadlineAt = now.Add(time.Duration(state.Rules.Duration) * time.Second)
		}
	case "kill", "interact", "reach", "damage", "heal", "rescue":
		if len(state.Facts) >= 10000 {
			return store.Conflict
		}
		if state.Status != "running" || !active || member.LifeStatus != "alive" || event.TargetID == "" {
			return store.Forbidden
		}
		key := event.EventType + ":" + event.TargetID
		if event.EventType == "damage" || event.EventType == "heal" || event.EventType == "rescue" {
			var effect struct {
				ActionID              string `json:"action_id"`
				TargetPlayerID        int64  `json:"target_player_id"`
				EffectiveAmount       int64  `json:"effective_amount"`
				HealthBefore          int64  `json:"health_before"`
				HealthAfter           int64  `json:"health_after"`
				HealthMaximum         int64  `json:"health_maximum"`
				ReinforcementActionID string `json:"reinforcement_action_id"`
			}
			if json.Unmarshal(event.Payload, &effect) != nil || !store.ValidID(effect.ActionID) || effect.EffectiveAmount <= 0 || effect.EffectiveAmount > 1000000000 {
				return store.Invalid
			}
			if event.EventType == "damage" && effect.TargetPlayerID != 0 {
				return store.Forbidden
			}
			if event.EventType != "damage" {
				target := state.Member(&effect.TargetPlayerID)
				if target == nil || target == member || target.Status != "participating" {
					return store.Forbidden
				}
				if target.LifeStatus != "alive" {
					return store.Forbidden
				}
				if event.EventType == "heal" && (effect.HealthBefore < 0 || effect.HealthAfter <= effect.HealthBefore || effect.HealthAfter > effect.HealthMaximum || effect.HealthAfter-effect.HealthBefore != effect.EffectiveAmount) {
					return store.Invalid
				}
				if event.EventType == "rescue" {
					if effect.ReinforcementActionID == "" {
						return store.Invalid
					}
					spawn, exists := state.Spawns[effect.ReinforcementActionID]
					if !exists || spawn.Status != "spawned" || spawn.PlayerID != target.PlayerID || spawn.Deaths != target.Deaths {
						return store.Conflict
					}
					effect.ActionID = effect.ReinforcementActionID
				}
			}
			key = event.EventType + ":" + effect.ActionID
		}
		if event.EventType == "interact" || event.EventType == "reach" {
			key += ":" + strconv.FormatInt(member.PlayerID, 10)
		}
		if state.Facts[key] {
			break
		}
		state.Facts[key] = true
		member.Contribution++
		for _, id := range event.Contributors {
			contributor := state.Member(&id)
			if contributor != nil && contributor != member && contributor.Status == "participating" {
				contributor.Contribution++
			}
		}
		activeObjectives := task.Active(state.Rules.Objectives, state.Objectives)
		for _, objective := range state.Rules.Objectives {
			if activeObjectives[objective.Key] && objective.Type == event.EventType && (objective.TargetID == "" || objective.TargetID == event.TargetID) && state.Objectives[objective.Key] < objective.Required {
				state.Objectives[objective.Key]++
			}
		}
		for index := range state.Participants {
			recipient := &state.Participants[index]
			if recipient.Status != "participating" || recipient.Compatibility != "compatible" {
				continue
			}
			definition, _ := state.Rules.Task(recipient.TaskKey)
			if recipient.TaskCompleted {
				continue
			}
			if recipient.TaskProgress == nil {
				recipient.TaskProgress = map[string]int64{}
			}
			activePersonal := task.Active(definition.Objectives, recipient.TaskProgress)
			for _, objective := range definition.Objectives {
				if !activePersonal[objective.Key] || objective.Type != event.EventType || (objective.TargetID != "" && objective.TargetID != event.TargetID) || recipient.TaskProgress[objective.Key] >= objective.Required {
					continue
				}
				eligible := objective.Scope == "team" || (objective.Scope == "self" && recipient.PlayerID == member.PlayerID)
				if objective.Scope == "eligible" {
					eligible = recipient.PlayerID == member.PlayerID
					for _, contributor := range event.Contributors {
						if contributor == recipient.PlayerID {
							eligible = true
						}
					}
				}
				if eligible {
					recipient.TaskProgress[objective.Key]++
				}
			}
		}
		complete := true
		for _, objective := range state.Rules.Objectives {
			complete = complete && state.Objectives[objective.Key] >= objective.Required
		}
		if complete {
			finish(state, "success", event.Sequence, now)
		}
	case "death":
		if state.Status != "running" || !active || member.LifeStatus != "alive" {
			return store.Conflict
		}
		member.LifeStatus = "dead"
		member.Deaths++
		wipe(state, event.Sequence, now)
	case "reinforcement.reserve":
		if _, exists := state.Spawns[event.TargetID]; exists {
			break
		}
		if state.Status != "running" || !active || member.LifeStatus != "dead" || event.TargetID == "" {
			return store.Conflict
		}
		if state.ReinforcementUsed+pendingSpawns(state) >= state.ReinforcementBudget {
			return store.Conflict
		}
		state.Spawns[event.TargetID] = Spawn{PlayerID: member.PlayerID, Deaths: member.Deaths, Status: "pending_spawn", Deadline: now.Add(time.Duration(state.Rules.Spawn) * time.Second)}
		member.LifeStatus = "awaiting_respawn"
	case "reinforcement.spawned", "reinforcement.failed":
		spawn, exists := state.Spawns[event.TargetID]
		if !exists || spawn.Status != "pending_spawn" || member == nil || spawn.PlayerID != member.PlayerID {
			return store.Conflict
		}
		if event.EventType == "reinforcement.spawned" {
			spawn.Status = "spawned"
			state.ReinforcementUsed++
			member.LifeStatus = "alive"
		} else {
			spawn.Status = "failed"
			member.LifeStatus = "dead"
		}
		state.Spawns[event.TargetID] = spawn
		wipe(state, event.Sequence, now)
	case "timeout":
		if state.Status != "running" || now.Before(state.DeadlineAt) {
			return store.Conflict
		}
		finish(state, "timeout", event.Sequence, now)
	case "system_abort":
		finish(state, "system_abort", event.Sequence, now)
	default:
		return store.Invalid
	}
	return nil
}
func persistProgress(ctx context.Context, transaction *sql.Tx, state *State, event Event, previous map[int64]map[string]int64, previousContribution map[int64]int) error {
	for _, member := range state.Participants {
		if member.TaskKey == "" || member.Compatibility != "compatible" {
			continue
		}
		definition, _ := state.Rules.Task(member.TaskKey)
		persistent := map[string]int64{}
		for _, objective := range definition.Objectives {
			progress := member.TaskProgress[objective.Key]
			if _, err := transaction.ExecContext(ctx, "UPDATE pve_player_task_progress SET progress=? WHERE attempt_id=? AND objective_key=?", progress, member.TaskAttemptID, objective.Key); err != nil {
				return err
			}
			if objective.Persistent && !definition.NoDeaths && !definition.RequiresSuccess {
				persistent[objective.Key] = progress
			}
			delta := progress - previous[member.PlayerID][objective.Key]
			if delta > 0 {
				if _, err := transaction.ExecContext(ctx, "INSERT INTO pve_event_applications(event_id,player_id,objective_key,progress_delta) VALUES(?,?,?,?)", event.EventID, member.PlayerID, member.TaskAttemptID+":"+objective.Key, delta); err != nil {
					return err
				}
			}
		}
		if len(persistent) > 0 && !member.TaskCompleted {
			if err := task.SaveProgress(ctx, transaction, member.PlayerID, definition, state.Rules, taskPeriod(&member), persistent, false); err != nil {
				return err
			}
		}
		complete := definition.Immediate() && !member.TaskCompleted
		for _, objective := range definition.Objectives {
			complete = complete && member.TaskProgress[objective.Key] >= objective.Required
		}
		if complete {
			if _, err := transaction.ExecContext(ctx, "INSERT IGNORE INTO pve_pending_operations(operation_id,operation_type,aggregate_id,payload,next_attempt_at) VALUES(?,'task_completion',?,?,UTC_TIMESTAMP(3))", "task:"+member.TaskAttemptID, state.ID, store.JSON(map[string]any{"player_id": member.PlayerID})); err != nil {
				return err
			}
		}
	}
	if task.ValidObjectiveType(event.EventType) {
		for _, member := range state.Participants {
			if member.Status != "participating" {
				continue
			}
			involved := event.ActorPlayerID != nil && *event.ActorPlayerID == member.PlayerID
			for _, contributor := range event.Contributors {
				involved = involved || contributor == member.PlayerID
			}
			if involved && member.Contribution > previousContribution[member.PlayerID] {
				if _, err := transaction.ExecContext(ctx, "INSERT IGNORE INTO pve_contribution_evidence(event_id,run_id,player_id,contribution_type,target_id) VALUES(?,?,?,?,?)", event.EventID, state.ID, member.PlayerID, event.EventType, event.TargetID); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
func taskPeriod(member *Participant) string {
	if member.TaskPeriod != "" {
		return member.TaskPeriod
	}
	return "once"
}
func QueueSettlement(ctx context.Context, transaction *sql.Tx, state *State) error {
	_, err := transaction.ExecContext(ctx, "INSERT IGNORE INTO pve_pending_operations(operation_id,operation_type,aggregate_id,payload,next_attempt_at) VALUES(?,'settlement',?,?,UTC_TIMESTAMP(3))", "settle:"+state.ID, state.ID, store.JSON(map[string]any{"run_id": state.ID}))
	return err
}
func (s *Service) Tick(ctx context.Context, id string) error {
	return store.Transaction(ctx, s.DB, func(transaction *sql.Tx) error {
		state, err := Load(ctx, transaction, id)
		if err != nil {
			return err
		}
		if terminal(state) {
			return nil
		}
		now := s.Now().UTC()
		reason := expiredReason(state, now)
		for index := range state.Participants {
			member := &state.Participants[index]
			if member.Status == "participating" && !member.Connected && member.DisconnectedAt != nil && now.Sub(*member.DisconnectedAt) >= time.Duration(state.Rules.Reconnect)*time.Second {
				member.Status = "left"
				member.LeftSequence = state.AppliedSequence + 1
			}
		}
		if !hasParticipants(state) {
			reason = "team_abandoned"
		}
		if reason == "" {
			wipe(state, state.AppliedSequence, now)
			if terminal(state) {
				if err := QueueSettlement(ctx, transaction, state); err != nil {
					return err
				}
			}
		}
		if reason != "" {
			finish(state, reason, state.AppliedSequence, now)
			if err := QueueSettlement(ctx, transaction, state); err != nil {
				return err
			}
		}
		return Save(ctx, transaction, state)
	})
}
func (s *Service) Connect(ctx context.Context, player int64, connection string, connected bool) error {
	rows, err := s.DB.QueryContext(ctx, "SELECT run_id FROM pve_run_participants WHERE player_id=? AND status IN ('loading','participating')", player)
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := store.Transaction(ctx, s.DB, func(transaction *sql.Tx) error {
			state, err := Load(ctx, transaction, id)
			if err != nil {
				return err
			}
			member := state.Member(&player)
			if member == nil || terminal(state) {
				return nil
			}
			if connected {
				if member.Status == "left" {
					return nil
				}
				if member.DisconnectedAt != nil && s.Now().Sub(*member.DisconnectedAt) >= time.Duration(state.Rules.Reconnect)*time.Second {
					member.Status = "left"
					member.LeftSequence = state.AppliedSequence + 1
					return Save(ctx, transaction, state)
				}
				member.Connected = true
				member.ConnectionID = connection
				member.DisconnectedAt = nil
			} else if member.ConnectionID == connection {
				member.Connected = false
				now := s.Now().UTC()
				member.DisconnectedAt = &now
			}
			return Save(ctx, transaction, state)
		}); err != nil {
			return err
		}
	}
	return nil
}
func (s *Service) Leave(ctx context.Context, player int64, id string) error {
	return store.Transaction(ctx, s.DB, func(transaction *sql.Tx) error {
		state, err := Load(ctx, transaction, id)
		if err != nil {
			return err
		}
		if err := LeaveTx(ctx, transaction, state, player, s.Now().UTC()); err != nil {
			return err
		}
		return Save(ctx, transaction, state)
	})
}

func LeaveTx(ctx context.Context, transaction *sql.Tx, state *State, player int64, now time.Time) error {
	member := state.Member(&player)
	if member == nil {
		return store.Forbidden
	}
	if terminal(state) || member.Status == "left" {
		return nil
	}
	if state.Status == "loading" || state.GapDeadline != nil {
		return store.Conflict
	}
	member.Status = "left"
	member.LeftSequence = state.AppliedSequence + 1
	if !hasParticipants(state) {
		finish(state, "team_abandoned", state.AppliedSequence, now)
	} else {
		wipe(state, state.AppliedSequence, now)
	}
	if terminal(state) {
		return QueueSettlement(ctx, transaction, state)
	}
	return nil
}
func expiredReason(state *State, now time.Time) string {
	if state.GapDeadline != nil && !now.Before(*state.GapDeadline) {
		return "event_gap"
	}
	if state.Status == "running" && !now.Before(state.DeadlineAt) {
		return "timeout"
	}
	for _, spawn := range state.Spawns {
		if spawn.Status == "pending_spawn" && !now.Before(spawn.Deadline) {
			return "spawn_unknown"
		}
	}
	return ""
}
func (state *State) Member(player *int64) *Participant {
	if player == nil {
		return nil
	}
	for index := range state.Participants {
		if state.Participants[index].PlayerID == *player {
			return &state.Participants[index]
		}
	}
	return nil
}
func (state *State) PlayerIDs() []int64 {
	players := make([]int64, 0, len(state.Participants))
	for _, member := range state.Participants {
		players = append(players, member.PlayerID)
	}
	return players
}
func (state *State) ForPlayer(player int64) *State {
	data := store.JSON(state)
	var view State
	_ = json.Unmarshal(data, &view)
	view.Rules.Tasks = nil
	view.Facts = nil
	for index := range view.Participants {
		member := &view.Participants[index]
		member.ConnectionID = ""
		if member.PlayerID != player {
			member.TaskKey = ""
			member.TaskAttemptID = ""
			member.TaskVersion = ""
			member.TaskPeriod = ""
			member.TaskProgress = nil
			member.TaskCompleted = false
			member.Compatibility = "none"
		}
	}
	return &view
}
func terminal(state *State) bool {
	return state.Status == "ending" || state.Status == "closed" || state.Status == "aborted"
}
func finish(state *State, reason string, sequence int64, now time.Time) {
	state.Status = "ending"
	state.EndReason = reason
	state.FinalSequence = sequence
	state.EndedAt = &now
}
func pendingSpawns(state *State) int {
	count := 0
	for _, spawn := range state.Spawns {
		if spawn.Status == "pending_spawn" {
			count++
		}
	}
	return count
}
func hasParticipants(state *State) bool {
	for _, member := range state.Participants {
		if member.Status == "participating" || member.Status == "loading" {
			return true
		}
	}
	return false
}
func wipe(state *State, sequence int64, now time.Time) {
	alive := false
	for _, member := range state.Participants {
		alive = alive || (member.Status == "participating" && member.LifeStatus == "alive")
	}
	if !alive && pendingSpawns(state) == 0 && state.ReinforcementUsed >= state.ReinforcementBudget {
		finish(state, "team_wipe", sequence, now)
	}
}
func eventType(kind string) bool {
	switch kind {
	case "kill", "interact", "reach", "damage", "heal", "rescue", "loaded", "death", "reinforcement.reserve", "reinforcement.spawned", "reinforcement.failed", "timeout", "system_abort":
		return true
	}
	return false
}
func null(value string) any {
	if value == "" {
		return nil
	}
	return value
}
