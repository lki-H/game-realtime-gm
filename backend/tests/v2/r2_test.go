package v2_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"game-realtime-gm/backend/internal/auth"
	"game-realtime-gm/backend/internal/pve/party"
	"game-realtime-gm/backend/internal/pve/run"
	"game-realtime-gm/backend/internal/pve/store"
	"game-realtime-gm/backend/internal/pve/task"
)

func productRun(t *testing.T, fixture *fixture, keys []string) string {
	t.Helper()
	fixture.contributors = fixture.players[:len(keys)]
	rules, err := task.LoadProducts()
	if err != nil {
		t.Fatal(err)
	}
	selections := map[int64]run.TaskSelection{}
	for index, key := range keys {
		if key != "" {
			definition, ok := rules.Task(key)
			if !ok {
				t.Fatal(key)
			}
			selections[fixture.players[index]] = run.TaskSelection{TaskKey: key, TaskVersion: definition.LogicalVersion(rules)}
		}
	}
	state, err := fixture.app.Runs.Create(fixture.ctx, run.CreateRequest{OperationName: rules.Operation, Difficulty: rules.Difficulty, RuleVersion: rules.Version, PlayerIDs: fixture.players[:len(keys)], Tasks: selections})
	if err != nil {
		t.Fatal(err)
	}
	for _, player := range state.PlayerIDs() {
		if _, err := fixture.db.ExecContext(fixture.ctx, "INSERT INTO pve_player_activity_locks(player_id,activity_type,activity_id) VALUES(?,'run',?)", player, state.ID); err != nil {
			t.Fatal(err)
		}
	}
	for index, player := range state.PlayerIDs() {
		fixture.event(t, state.ID, int64(index+1), "loaded", player, "")
	}
	return state.ID
}

func TestR2RegroupRejectionExpiryAndConcurrentConsent(t *testing.T) {
	fixture := setup(t)
	id, _ := finishOldRun(t, fixture)
	input := map[string]any{"run_id": id, "owner_id": fixture.players[0], "player_ids": fixture.players}
	var declined struct {
		ProposalID string `json:"proposal_id"`
	}
	json.Unmarshal(fixture.command(t, fixture.players[0], "v2.party.regroup_propose", input), &declined)
	fixture.command(t, fixture.players[1], "v2.party.regroup_respond", map[string]any{"proposal_id": declined.ProposalID, "revision": 1, "accept": false})
	if _, err := fixture.app.Command(fixture.ctx, fixture.players[0], store.ID("cmd"), "v2.party.regroup_respond", store.JSON(map[string]any{"proposal_id": declined.ProposalID, "revision": 1, "accept": true})); !errors.Is(err, store.Conflict) {
		t.Fatalf("rejected regroup became active: %v", err)
	}
	var expired struct {
		ProposalID string `json:"proposal_id"`
	}
	json.Unmarshal(fixture.command(t, fixture.players[0], "v2.party.regroup_propose", input), &expired)
	fixture.db.ExecContext(fixture.ctx, "UPDATE pve_regroup_proposals SET expires_at=DATE_SUB(UTC_TIMESTAMP(3),INTERVAL 1 SECOND) WHERE id=?", expired.ProposalID)
	if _, err := fixture.app.Command(fixture.ctx, fixture.players[0], store.ID("cmd"), "v2.party.regroup_respond", store.JSON(map[string]any{"proposal_id": expired.ProposalID, "revision": 1, "accept": true})); !errors.Is(err, store.Conflict) {
		t.Fatalf("expired regroup accepted: %v", err)
	}
	var proposals [2]string
	for index := range proposals {
		var value struct {
			ProposalID string `json:"proposal_id"`
		}
		json.Unmarshal(fixture.command(t, fixture.players[0], "v2.party.regroup_propose", input), &value)
		proposals[index] = value.ProposalID
		for _, player := range fixture.players[:3] {
			fixture.command(t, player, "v2.party.regroup_respond", map[string]any{"proposal_id": value.ProposalID, "revision": 1, "accept": true})
		}
	}
	var wait sync.WaitGroup
	errs := make(chan error, 2)
	for _, proposal := range proposals {
		wait.Add(1)
		go func(id string) {
			defer wait.Done()
			_, err := fixture.app.Command(fixture.ctx, fixture.players[3], store.ID("cmd"), "v2.party.regroup_respond", store.JSON(map[string]any{"proposal_id": id, "revision": 1, "accept": true}))
			errs <- err
		}(proposal)
	}
	wait.Wait()
	close(errs)
	success := 0
	for err := range errs {
		if err == nil {
			success++
		} else if !errors.Is(err, store.Conflict) {
			t.Fatal(err)
		}
	}
	if success != 1 {
		t.Fatalf("competing regroup commits=%d", success)
	}
}

func TestR2TaskCompletionWorkerRecoversAfterAssetFailure(t *testing.T) {
	fixture := setup(t)
	id := productRun(t, fixture, []string{"daily_support"})
	fixture.event(t, id, 2, "interact", fixture.players[0], "terminal")
	fixture.app.Worker.Tick = nil
	fixture.db.ExecContext(fixture.ctx, "DELETE FROM player_assets WHERE player_id=?", fixture.players[0])
	if err := fixture.app.Worker.Once(fixture.ctx); err != nil {
		t.Fatal(err)
	}
	var grants int
	fixture.db.QueryRowContext(fixture.ctx, "SELECT COUNT(*) FROM pve_reward_grants WHERE run_id=?", id).Scan(&grants)
	if grants != 0 {
		t.Fatal("failed task reward partially committed")
	}
	fixture.db.ExecContext(fixture.ctx, "INSERT INTO player_assets(player_id) VALUES(?)", fixture.players[0])
	fixture.db.ExecContext(fixture.ctx, "UPDATE pve_pending_operations SET next_attempt_at=UTC_TIMESTAMP(3) WHERE aggregate_id=?", id)
	if err := fixture.app.Worker.Once(fixture.ctx); err != nil {
		t.Fatal(err)
	}
	var balance int64
	fixture.db.QueryRowContext(fixture.ctx, "SELECT soft_currency FROM player_assets WHERE player_id=?", fixture.players[0]).Scan(&balance)
	if balance != 15 {
		t.Fatalf("recovered task reward=%d", balance)
	}
	state, _ := fixture.app.Runs.Snapshot(fixture.ctx, id)
	if state.Status != "running" || !state.Participants[0].TaskCompleted {
		t.Fatal("task recovery released running instance")
	}
}
func finishOldRun(t *testing.T, fixture *fixture) (string, string) {
	t.Helper()
	id, room := fixture.assigned(t)
	for index, player := range fixture.players {
		fixture.event(t, id, int64(index+1), "loaded", player, "")
	}
	for index := 0; index < 3; index++ {
		fixture.event(t, id, int64(index+5), "kill", fixture.players[0], store.ID("enemy"))
	}
	fixture.event(t, id, 8, "interact", fixture.players[1], "terminal")
	fixture.event(t, id, 9, "reach", fixture.players[2], "exit")
	if err := fixture.app.Settlement.Settle(fixture.ctx, id); err != nil {
		t.Fatal(err)
	}
	return id, room
}
func playerGet(t *testing.T, fixture *fixture, player int64, path string) map[string]any {
	t.Helper()
	token, err := auth.GenerateToken(fixture.secret, player, "r2-query")
	if err != nil {
		t.Fatal(err)
	}
	request, _ := http.NewRequest("GET", fixture.server.URL+path, nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var envelope struct {
		Code int            `json:"code"`
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil || envelope.Code != 0 || response.StatusCode != 200 {
		t.Fatalf("query %s status=%d err=%v", path, response.StatusCode, err)
	}
	return envelope.Data
}

func TestR2PreviewPrivacyRecommendationAndPause(t *testing.T) {
	fixture := setup(t)
	if _, err := fixture.db.ExecContext(fixture.ctx, "INSERT INTO pve_player_tasks(player_id,task_key,version,progress,completed,unlocked) VALUES(?,'hunter','training_ground.v1','{\"kills\":2}',0,1)", fixture.players[0]); err != nil {
		t.Fatal(err)
	}
	first := playerGet(t, fixture, fixture.players[0], "/api/v2/operations/preview?rule_version=training_ground.v2&task_key=hunter")
	second := playerGet(t, fixture, fixture.players[1], "/api/v2/operations/preview?rule_version=training_ground.v2&task_key=hunter")
	if first["progress"].(map[string]any)["counts"].(map[string]any)["kills"] != float64(2) || len(second["progress"].(map[string]any)["counts"].(map[string]any)) != 0 {
		t.Fatal("preview leaked or lost private progress")
	}
	incompatible := playerGet(t, fixture, fixture.players[0], "/api/v2/operations/preview?rule_version=training_ground.v2&task_key=other_map")
	if incompatible["compatibility"] != "incompatible" || incompatible["admission_allowed"] != true || incompatible["base_reward_affected"] != false {
		t.Fatal("soft compatibility became hard admission")
	}
	recommended, err := task.Recommendations(fixture.ctx, fixture.db, fixture.players[0], "other_map")
	if err != nil || len(recommended) != 0 {
		t.Fatalf("incompatible recommendation=%+v %v", recommended, err)
	}
	fixture.command(t, fixture.players[0], "v2.task.pause", map[string]any{"task_key": "hunter", "task_version": "training_ground.v1"})
	after := playerGet(t, fixture, fixture.players[0], "/api/v2/operations/preview?task_key=hunter")
	if after["progress"].(map[string]any)["counts"].(map[string]any)["kills"] != float64(2) {
		t.Fatal("pause reset permanent progress")
	}
}

func TestR2ImmediateRewardRetainsRunAndDoesNotRepeatAtEnding(t *testing.T) {
	fixture := setup(t)
	id := productRun(t, fixture, []string{"daily_support", "field_engineer", "", "survivor"})
	fixture.event(t, id, 5, "interact", fixture.players[1], "terminal")
	fixture.app.Worker.Tick = nil
	if err := fixture.app.Worker.Once(fixture.ctx); err != nil {
		t.Fatal(err)
	}
	var balance int64
	if err := fixture.db.QueryRowContext(fixture.ctx, "SELECT soft_currency FROM player_assets WHERE player_id=?", fixture.players[0]).Scan(&balance); err != nil || balance != 15 {
		t.Fatalf("early reward=%d err=%v", balance, err)
	}
	state, err := fixture.app.Runs.Snapshot(fixture.ctx, id)
	if err != nil || state.Status != "running" || !state.Participants[0].TaskCompleted {
		t.Fatalf("early completion state=%+v err=%v", state, err)
	}
	var locks int
	fixture.db.QueryRowContext(fixture.ctx, "SELECT COUNT(*) FROM pve_player_activity_locks WHERE activity_id=?", id).Scan(&locks)
	if locks != 4 {
		t.Fatal("early task released run activity")
	}
	fixture.event(t, id, 6, "reach", fixture.players[1], "exit")
	for index := 0; index < 3; index++ {
		fixture.event(t, id, int64(index+7), "kill", fixture.players[2], store.ID("enemy"))
	}
	if err := fixture.app.Worker.Once(fixture.ctx); err != nil {
		t.Fatal(err)
	}
	if err := fixture.app.Settlement.Settle(fixture.ctx, id); err != nil {
		t.Fatal(err)
	}
	fixture.db.QueryRowContext(fixture.ctx, "SELECT soft_currency FROM player_assets WHERE player_id=?", fixture.players[0]).Scan(&balance)
	if balance != 115 {
		t.Fatalf("reward repeated: balance=%d", balance)
	}
}

func TestR2IntervalRewardWaitsForDeathBoundary(t *testing.T) {
	fixture := setup(t)
	id := productRun(t, fixture, []string{"survivor", ""})
	fixture.event(t, id, 3, "kill", fixture.players[0], "enemy1")
	fixture.app.Worker.Tick = nil
	if err := fixture.app.Worker.Once(fixture.ctx); err != nil {
		t.Fatal(err)
	}
	var amount int
	fixture.db.QueryRowContext(fixture.ctx, "SELECT COUNT(*) FROM pve_reward_grants WHERE run_id=?", id).Scan(&amount)
	if amount != 0 {
		t.Fatal("interval award paid before future death known")
	}
	fixture.event(t, id, 4, "death", fixture.players[0], "")
	fixture.event(t, id, 5, "system_abort", fixture.players[1], "")
	if err := fixture.app.Settlement.Settle(fixture.ctx, id); err != nil {
		t.Fatal(err)
	}
	fixture.db.QueryRowContext(fixture.ctx, "SELECT COUNT(*) FROM pve_reward_grants WHERE run_id=? AND reward_source='task:survivor'", id).Scan(&amount)
	if amount != 0 {
		t.Fatal("death interval rewarded")
	}
}

func TestR2HealingEvidenceAndRepeatedAction(t *testing.T) {
	fixture := setup(t)
	id := productRun(t, fixture, []string{"medic", ""})
	event := run.Event{EventID: store.ID("event"), Source: "pve_event_bot", SourceGeneration: 1, RunID: id, Sequence: 3, SchemaVersion: 2, EventType: "heal", ActorPlayerID: &fixture.players[0], TargetID: "teammate", Payload: store.JSON(map[string]any{"action_id": "healing_action_1", "target_player_id": fixture.players[1], "effective_amount": 10, "health_before": 50, "health_after": 60, "health_maximum": 100}), OccurredAt: time.Now().UTC()}
	if _, err := fixture.app.Runs.ApplyEvent(fixture.ctx, event); err != nil {
		t.Fatal(err)
	}
	event.EventID = store.ID("event")
	event.Sequence = 4
	if _, err := fixture.app.Runs.ApplyEvent(fixture.ctx, event); err != nil {
		t.Fatal(err)
	}
	state, err := fixture.app.Runs.Snapshot(fixture.ctx, id)
	if err != nil || state.Participants[0].TaskProgress["heal"] != 1 || state.Participants[0].Contribution != 1 {
		t.Fatalf("repeated healing counted: %+v %v", state, err)
	}
	event.EventID = store.ID("event")
	event.Sequence = 5
	event.Payload = store.JSON(map[string]any{"action_id": "healing_action_2", "target_player_id": fixture.players[0], "effective_amount": 10, "health_before": 50, "health_after": 60, "health_maximum": 100})
	if _, err := fixture.app.Runs.ApplyEvent(fixture.ctx, event); !errors.Is(err, store.Forbidden) {
		t.Fatalf("self healing err=%v", err)
	}
}

func TestR2RepeatPeriodHasIndependentCompletionIdentity(t *testing.T) {
	fixture := setup(t)
	id := productRun(t, fixture, []string{"daily_support"})
	fixture.event(t, id, 2, "interact", fixture.players[0], "terminal")
	fixture.event(t, id, 3, "system_abort", fixture.players[0], "")
	if err := fixture.app.Settlement.Settle(fixture.ctx, id); err != nil {
		t.Fatal(err)
	}
	request := run.CreateRequest{OperationName: "training_ground", Difficulty: "normal", RuleVersion: "training_ground.v2", PlayerIDs: fixture.players[:1], Tasks: map[int64]run.TaskSelection{fixture.players[0]: {TaskKey: "daily_support", TaskVersion: "training_ground.v2"}}}
	if _, err := fixture.app.Runs.Create(fixture.ctx, request); !errors.Is(err, store.Conflict) {
		t.Fatalf("same period task repeated: %v", err)
	}
	fixture.app.Runs.Now = func() time.Time { return time.Now().UTC().Add(24 * time.Hour) }
	next, err := fixture.app.Runs.Create(fixture.ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	previous, err := fixture.app.Runs.Snapshot(fixture.ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if next.Participants[0].TaskPeriod == previous.Participants[0].TaskPeriod || len(next.Participants[0].TaskProgress) != 0 {
		t.Fatal("period carried old completion or progress")
	}
}

func TestR2RegroupConsentAndConflictingProposal(t *testing.T) {
	fixture := setup(t)
	id, room := finishOldRun(t, fixture)
	input := map[string]any{"run_id": id, "owner_id": fixture.players[0], "player_ids": fixture.players}
	var first, second struct {
		ProposalID string `json:"proposal_id"`
	}
	json.Unmarshal(fixture.command(t, fixture.players[0], "v2.party.regroup_propose", input), &first)
	json.Unmarshal(fixture.command(t, fixture.players[0], "v2.party.regroup_propose", input), &second)
	fixture.command(t, fixture.players[0], "v2.party.regroup_respond", map[string]any{"proposal_id": first.ProposalID, "revision": 1, "accept": true})
	group, err := fixture.app.Party.Snapshot(fixture.ctx, room, fixture.players[0])
	if err != nil || len(group.Members) != 2 {
		t.Fatal("proposal moved members before all consent")
	}
	for _, player := range fixture.players[1:] {
		fixture.command(t, player, "v2.party.regroup_respond", map[string]any{"proposal_id": first.ProposalID, "revision": 1, "accept": true})
	}
	view := playerGet(t, fixture, fixture.players[0], "/api/v2/regroup/"+first.ProposalID)
	if view["status"] != "committed" || view["party_id"] == room {
		t.Fatalf("regroup not merged: %+v", view)
	}
	for _, player := range fixture.players[:3] {
		fixture.command(t, player, "v2.party.regroup_respond", map[string]any{"proposal_id": second.ProposalID, "revision": 1, "accept": true})
	}
	if _, err := fixture.app.Command(fixture.ctx, fixture.players[3], store.ID("cmd"), "v2.party.regroup_respond", store.JSON(map[string]any{"proposal_id": second.ProposalID, "revision": 1, "accept": true})); !errors.Is(err, store.Conflict) {
		t.Fatalf("second proposal overwrote new membership: %v", err)
	}
	var count int
	fixture.db.QueryRowContext(fixture.ctx, "SELECT COUNT(*) FROM pve_party_members WHERE status='active'").Scan(&count)
	if count != 4 {
		t.Fatalf("regroup duplicated membership=%d", count)
	}
}

func TestR2RecruitmentGuestReadyAndSeparateRejectionSource(t *testing.T) {
	fixture := setup(t)
	owner, guest := fixture.players[0], fixture.players[1]
	var group party.Party
	json.Unmarshal(fixture.command(t, owner, "v2.party.create", map[string]any{}), &group)
	fixture.command(t, owner, "v2.party.plan_update", map[string]any{"party_id": group.ID, "plan": party.Plan{Operation: "training_ground", Difficulty: "normal", RuleVersion: "training_ground.v2", FillPolicy: "public", AllowPartial: true}})
	var post struct {
		PostID string `json:"post_id"`
	}
	json.Unmarshal(fixture.command(t, owner, "v2.recruitment.publish", map[string]any{"party_id": group.ID}), &post)
	var application struct {
		ID int64 `json:"application_id"`
	}
	json.Unmarshal(fixture.command(t, guest, "v2.recruitment.apply", map[string]any{"post_id": post.PostID}), &application)
	fixture.command(t, owner, "v2.recruitment.respond", map[string]any{"application_id": application.ID, "accept": true})
	groupPtr, err := fixture.app.Party.Snapshot(fixture.ctx, group.ID, owner)
	if err != nil {
		t.Fatal(err)
	}
	fixture.command(t, owner, "v2.party.ready", map[string]any{"party_id": group.ID, "ready": true, "roster_version": groupPtr.RosterVersion, "plan_version": groupPtr.PlanVersion, "selection_version": 1})
	if _, err := fixture.app.Command(fixture.ctx, owner, store.ID("cmd"), "v2.match.enqueue", store.JSON(map[string]any{"party_id": group.ID})); !errors.Is(err, store.Conflict) {
		t.Fatalf("unready guest enqueued: %v", err)
	}
	fixture.command(t, guest, "v2.recruitment.ready", map[string]any{"party_id": group.ID, "ready": true, "roster_version": groupPtr.RosterVersion, "plan_version": groupPtr.PlanVersion, "selection_version": 1})
	fixture.command(t, owner, "v2.match.enqueue", map[string]any{"party_id": group.ID})
	for _, player := range fixture.players[2:] {
		fixture.command(t, player, "v2.match.enqueue", map[string]any{"plan": party.Plan{Operation: "training_ground", Difficulty: "normal", RuleVersion: "training_ground.v2", FillPolicy: "public", AllowPartial: true}})
	}
	if err := fixture.app.Match.Match(fixture.ctx); err != nil {
		t.Fatal(err)
	}
	var proposal string
	fixture.db.QueryRowContext(fixture.ctx, "SELECT id FROM pve_match_proposals WHERE status='pending'").Scan(&proposal)
	fixture.command(t, guest, "v2.match.proposal_reject", map[string]any{"proposal_id": proposal, "revision": 1})
	var ownerLocks, guestLocks int
	fixture.db.QueryRowContext(fixture.ctx, "SELECT COUNT(*) FROM pve_player_activity_locks WHERE player_id=?", owner).Scan(&ownerLocks)
	fixture.db.QueryRowContext(fixture.ctx, "SELECT COUNT(*) FROM pve_player_activity_locks WHERE player_id=?", guest).Scan(&guestLocks)
	if ownerLocks != 1 || guestLocks != 0 {
		t.Fatalf("guest rejection paused friend source: %d %d", ownerLocks, guestLocks)
	}
}

func TestR2DifferentRuleVersionsNeverShareProposal(t *testing.T) {
	fixture := setup(t)
	for index, version := range []string{"training_ground.v1", "training_ground.v2"} {
		fixture.command(t, fixture.players[index], "v2.match.enqueue", map[string]any{"plan": party.Plan{Operation: "training_ground", Difficulty: "normal", RuleVersion: version, FillPolicy: "public", AllowPartial: false}})
	}
	if err := fixture.app.Match.Match(fixture.ctx); err != nil {
		t.Fatal(err)
	}
	var proposals int
	fixture.db.QueryRowContext(fixture.ctx, "SELECT COUNT(*) FROM pve_match_proposals").Scan(&proposals)
	if proposals != 0 {
		t.Fatal("different semantic versions composed partial proposal")
	}
	fixture.app.Match.Now = func() time.Time { return time.Now().Add(time.Hour) }
	if err := fixture.app.Match.Match(fixture.ctx); err != nil {
		t.Fatal(err)
	}
	fixture.db.QueryRowContext(fixture.ctx, "SELECT COUNT(*) FROM pve_match_proposals").Scan(&proposals)
	if proposals != 0 {
		t.Fatal("timeout bypassed partial consent or rule version")
	}
}
