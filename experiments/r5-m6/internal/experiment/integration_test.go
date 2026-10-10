package experiment

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	reportingv1 "game-realtime-gm/experiments/r5-m6/gen/reporting/v1"
	amqp "github.com/rabbitmq/amqp091-go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type integrationFixture struct {
	cfg        Config
	source     Source
	projection Projection
	admin      *sql.DB
	broker     *Broker
	metrics    *Metrics
	report     Report
}

func integration(t *testing.T) *integrationFixture {
	t.Helper()
	if os.Getenv("R5_INTEGRATION") != "1" {
		t.Skip("set R5_INTEGRATION=1 with the isolated R5 configuration")
	}
	cfg, err := LoadConfig(os.Getenv("R5_CONFIG"))
	if err != nil {
		t.Fatal(err)
	}
	source, err := OpenDatabase(cfg.Source)
	if err != nil {
		t.Fatal("source setup failed")
	}
	projection, err := OpenDatabase(cfg.Projection)
	if err != nil {
		t.Fatal("projection setup failed")
	}
	t.Cleanup(func() { source.Close(); projection.Close() })
	var admin struct {
		Root   DatabaseConfig `json:"root_db"`
		Broker BrokerConfig   `json:"broker"`
	}
	data, err := os.ReadFile(os.Getenv("R5_ADMIN_CONFIG"))
	if err != nil {
		t.Fatal("isolated admin configuration missing")
	}
	if json.Unmarshal(data, &admin) != nil || admin.Root.Name != "game_realtime_v2_r5_restore" {
		t.Fatal("isolated admin required")
	}
	root, err := OpenDatabase(admin.Root)
	if err != nil {
		t.Fatal("admin setup failed")
	}
	t.Cleanup(func() { root.Close() })
	broker, err := OpenBroker(admin.Broker)
	if err != nil {
		t.Fatal("isolated RabbitMQ unavailable")
	}
	t.Cleanup(broker.Close)
	for _, queue := range []string{"r5.reports", "r5.retry.queue", "r5.dead.queue"} {
		if _, err := broker.Channel.QueuePurge(queue, false); err != nil {
			t.Fatal(err)
		}
	}
	for _, table := range []string{"r5_report_runs", "r5_receipts", "r5_deliveries", "r5_scan_state"} {
		if _, err := projection.Exec("DELETE FROM " + table); err != nil {
			t.Fatal(err)
		}
	}
	var runID string
	if err := source.QueryRow("SELECT id FROM r5_runs ORDER BY id LIMIT 1").Scan(&runID); err != nil {
		t.Fatal("real settled fixture missing")
	}
	result, err := (Source{DB: source}).Result(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	return &integrationFixture{cfg: cfg, source: Source{DB: source}, projection: Projection{DB: projection}, admin: root, broker: broker, metrics: &Metrics{}, report: ReportFrom(result)}
}
func (fixture *integrationFixture) envelope(id string) Envelope {
	return NewEnvelope(id, fixture.cfg.SourceName, time.Now().UTC(), fixture.report)
}
func (fixture *integrationFixture) publish(t *testing.T, envelope Envelope) {
	t.Helper()
	data, _ := json.Marshal(envelope)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := fixture.broker.Publish(ctx, "events", data, nil); err != nil {
		t.Fatal(err)
	}
}
func (fixture *integrationFixture) get(t *testing.T, queue string) amqp.Delivery {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		delivery, found, err := fixture.broker.Channel.Get(queue, false)
		if err != nil {
			t.Fatal(err)
		}
		if found {
			return delivery
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("delivery not available before deadline")
	return amqp.Delivery{}
}
func (fixture *integrationFixture) consumer() Consumer {
	return Consumer{Projection: fixture.projection, Sender: fixture.broker, SourceName: fixture.cfg.SourceName, MaxAge: fixture.cfg.MaxAge(), RetryLimit: fixture.cfg.RetryLimit, Metrics: fixture.metrics}
}
func TestIntegrationReadOnlyAndRealOutbox(t *testing.T) {
	fixture := integration(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	for _, db := range []*sql.DB{fixture.source.DB, fixture.projection.DB} {
		for _, statement := range []string{"UPDATE game_realtime_v2_r5_restore.player_assets SET soft_currency=soft_currency WHERE player_id=-1", "DELETE FROM game_realtime_v2_r5_restore.pve_runs WHERE id='missing'", "UPDATE game_realtime_v2_r5_restore.pve_outbox_records SET status='published' WHERE id=-1", "SELECT * FROM game_realtime_v2_r5_restore.players LIMIT 1"} {
			if _, err := db.ExecContext(ctx, statement); err == nil {
				t.Fatal("experiment account accessed protected facts")
			}
		}
	}
	var before string
	if err := fixture.admin.QueryRowContext(ctx, "SELECT GROUP_CONCAT(CONCAT(id,':',status) ORDER BY id) FROM pve_outbox_records").Scan(&before); err != nil {
		t.Fatal(err)
	}
	bridge := Bridge{Source: fixture.source, Projection: fixture.projection, SourceName: fixture.cfg.SourceName, Metrics: fixture.metrics}
	if err := bridge.Once(ctx); err != nil {
		t.Fatal(err)
	}
	if err := bridge.Once(ctx); err != nil {
		t.Fatal(err)
	}
	if err := bridge.Once(ctx); err != nil {
		t.Fatal(err)
	}
	var deliveries int
	fixture.projection.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM r5_deliveries").Scan(&deliveries)
	if deliveries < 1 {
		t.Fatal("transactional run result outbox not bridged")
	}
	pubBroker, err := OpenBroker(fixture.cfg.Publisher)
	if err != nil {
		t.Fatal(err)
	}
	defer pubBroker.Close()
	publisher := Publisher{Projection: fixture.projection, Sender: pubBroker, RetryLimit: fixture.cfg.RetryLimit, Metrics: fixture.metrics}
	for index := 0; index < deliveries; index++ {
		if err := publisher.Once(ctx); err != nil {
			t.Fatal(err)
		}
	}
	for index := 0; index < deliveries; index++ {
		delivery := fixture.get(t, "r5.reports")
		if err := fixture.consumer().Handle(ctx, delivery); err != nil {
			t.Fatal(err)
		}
	}
	var reports int
	fixture.projection.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM r5_report_runs").Scan(&reports)
	if reports < 1 {
		t.Fatal("projection missing")
	}
	var after string
	if err := fixture.admin.QueryRowContext(ctx, "SELECT GROUP_CONCAT(CONCAT(id,':',status) ORDER BY id) FROM pve_outbox_records").Scan(&after); err != nil || after != before {
		t.Fatal("bridge modified primary outbox state")
	}
	for _, table := range []string{"r5_report_runs", "r5_receipts", "r5_deliveries", "r5_scan_state"} {
		if _, err := fixture.projection.DB.ExecContext(ctx, "DELETE FROM "+table); err != nil {
			t.Fatal(err)
		}
	}
	if err := bridge.Once(ctx); err != nil {
		t.Fatal(err)
	}
	fixture.projection.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM r5_deliveries").Scan(&reports)
	if reports != deliveries {
		t.Fatal("projection rebuild did not recover source events")
	}
	for index := 0; index < deliveries; index++ {
		if err := publisher.Once(ctx); err != nil {
			t.Fatal(err)
		}
	}
	for index := 0; index < deliveries; index++ {
		if err := fixture.consumer().Handle(ctx, fixture.get(t, "r5.reports")); err != nil {
			t.Fatal(err)
		}
	}
	if err := fixture.projection.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM r5_report_runs").Scan(&reports); err != nil || reports != 1 {
		t.Fatal("full report rebuild did not recover the fixture")
	}
}
func TestIntegrationDuplicateConflictAndCrashBeforeACK(t *testing.T) {
	fixture := integration(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	envelope := fixture.envelope("duplicate_case")
	fixture.publish(t, envelope)
	channel, err := fixture.broker.Connection.Channel()
	if err != nil {
		t.Fatal(err)
	}
	var delivery amqp.Delivery
	for index := 0; index < 100; index++ {
		current, found, getErr := channel.Get("r5.reports", false)
		if getErr != nil {
			t.Fatal(getErr)
		}
		if found {
			delivery = current
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if delivery.MessageId == "" {
		t.Fatal("initial delivery unavailable")
	}
	parsed, report, err := DecodeEnvelope(delivery.Body, fixture.cfg.SourceName, time.Now().UTC(), fixture.cfg.MaxAge())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.projection.Apply(ctx, parsed, report); err != nil {
		t.Fatal(err)
	}
	channel.Close()
	redelivery := fixture.get(t, "r5.reports")
	if !redelivery.Redelivered {
		t.Fatal("channel loss did not requeue unacknowledged message")
	}
	if err := fixture.consumer().Handle(ctx, redelivery); err != nil {
		t.Fatal(err)
	}
	fixture.publish(t, envelope)
	if err := fixture.consumer().Handle(ctx, fixture.get(t, "r5.reports")); err != nil {
		t.Fatal(err)
	}
	changed := fixture.report
	changed.Participants = append([]Participant(nil), fixture.report.Participants...)
	changed.Participants[0].Reward++
	conflict := NewEnvelope(envelope.MessageID, fixture.cfg.SourceName, envelope.PublishedAt, changed)
	fixture.publish(t, conflict)
	if err := fixture.consumer().Handle(ctx, fixture.get(t, "r5.reports")); err != nil {
		t.Fatal(err)
	}
	dead := fixture.get(t, "r5.dead.queue")
	dead.Ack(false)
	var count int
	fixture.projection.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM r5_report_runs").Scan(&count)
	if count != 1 {
		t.Fatal("duplicate altered projection")
	}
	var data []byte
	fixture.projection.DB.QueryRowContext(ctx, "SELECT report FROM r5_report_runs").Scan(&data)
	var stored Report
	json.Unmarshal(data, &stored)
	if stored.Participants[0].Reward != fixture.report.Participants[0].Reward {
		t.Fatal("same-id conflict changed reward report")
	}
}
func TestIntegrationFiniteRetryDeadLetterExpiredAndConcurrent(t *testing.T) {
	fixture := integration(t)
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	consumer := fixture.consumer()
	consumer.Apply = func(context.Context, Envelope, Report) (bool, error) {
		return false, errors.New("simulated_projection_failure")
	}
	fixture.publish(t, fixture.envelope("retry_case"))
	for index := 0; index < fixture.cfg.RetryLimit; index++ {
		if err := consumer.Handle(ctx, fixture.get(t, "r5.reports")); err != nil {
			t.Fatal(err)
		}
	}
	dead := fixture.get(t, "r5.dead.queue")
	dead.Ack(false)
	var attempts int
	var state string
	fixture.projection.DB.QueryRowContext(ctx, "SELECT attempts,status FROM r5_receipts WHERE message_id='retry_case'").Scan(&attempts, &state)
	if attempts != fixture.cfg.RetryLimit || state != "dead" {
		t.Fatal("retry budget not durable or bounded")
	}
	expired := fixture.envelope("expired_case")
	expired.PublishedAt = time.Now().Add(-2 * fixture.cfg.MaxAge())
	fixture.publish(t, expired)
	if err := fixture.consumer().Handle(ctx, fixture.get(t, "r5.reports")); err != nil {
		t.Fatal(err)
	}
	dead = fixture.get(t, "r5.dead.queue")
	dead.Ack(false)
	envelope := fixture.envelope("concurrent_case")
	var group sync.WaitGroup
	errorsFound := make(chan error, 8)
	for index := 0; index < 8; index++ {
		group.Add(1)
		go func() {
			defer group.Done()
			_, err := fixture.projection.Apply(ctx, envelope, fixture.report)
			errorsFound <- err
		}()
	}
	group.Wait()
	close(errorsFound)
	for err := range errorsFound {
		if err != nil {
			t.Fatal(err)
		}
	}
	var count int
	fixture.projection.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM r5_report_runs").Scan(&count)
	if count != 1 {
		t.Fatal("concurrent replay changed projection")
	}
}
func TestIntegrationRPCRecoveryAndServicePermissions(t *testing.T) {
	fixture := integration(t)
	data, err := os.ReadFile(filepath.Join(filepath.Dir(os.Getenv("R5_CONFIG")), "clients.json"))
	if err != nil {
		t.Fatal(err)
	}
	var tokens map[string]string
	json.Unmarshal(data, &tokens)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := RPCServer(fixture.cfg, fixture.source, fixture.metrics)
	go server.Serve(listener)
	connection, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	defer server.Stop()
	client := reportingv1.NewReportingClient(connection)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ctx = metadata.AppendToOutgoingContext(ctx, "service-id", "r5-report-client", "authorization", "Bearer "+tokens["r5-report-client"])
	result, err := client.GetRunResult(ctx, &reportingv1.GetRunResultRequest{RunId: fixture.report.RunID, PlayerId: fixture.report.Participants[0].PlayerID})
	if err != nil || len(result.GetParticipants()) != 4 {
		t.Fatal("real settled run RPC failed")
	}
	if _, err := client.GetRunResult(ctx, &reportingv1.GetRunResultRequest{RunId: fixture.report.RunID, PlayerId: 99999999}); status.Code(err) != codes.PermissionDenied {
		t.Fatal("unscoped player queried")
	}
	leaderboard, err := client.GetLeaderboard(ctx, &reportingv1.GetLeaderboardRequest{Limit: 10})
	if err != nil || len(leaderboard.GetEntries()) != 4 {
		t.Fatal("real leaderboard RPC failed")
	}
	server.Stop()
	if _, err := client.GetLeaderboard(ctx, &reportingv1.GetLeaderboardRequest{Limit: 10}); status.Code(err) != codes.Unavailable {
		t.Fatal("stopped service still accepted query")
	}
	listener, err = net.Listen("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	server = RPCServer(fixture.cfg, fixture.source, fixture.metrics)
	defer server.Stop()
	go server.Serve(listener)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		call, cancelCall := context.WithTimeout(context.Background(), time.Second)
		call = metadata.AppendToOutgoingContext(call, "service-id", "r5-report-client", "authorization", "Bearer "+tokens["r5-report-client"])
		_, err = client.GetLeaderboard(call, &reportingv1.GetLeaderboardRequest{Limit: 1}, grpc.WaitForReady(true))
		cancelCall()
		if err == nil {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal(fmt.Sprintf("RPC did not recover: %v", status.Code(err)))
}
