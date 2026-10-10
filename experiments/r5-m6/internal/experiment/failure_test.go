package experiment

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

type recordingSender struct {
	destination string
	headers     map[string]any
	err         error
}

func (sender *recordingSender) Publish(ctx context.Context, destination string, data []byte, headers map[string]any) error {
	sender.destination = destination
	sender.headers = headers
	return sender.err
}

type recordingACK struct{ acked, nacked bool }

func (ack *recordingACK) Ack(tag uint64, multiple bool) error { ack.acked = true; return nil }
func (ack *recordingACK) Nack(tag uint64, multiple, requeue bool) error {
	ack.nacked = true
	return nil
}
func (ack *recordingACK) Reject(tag uint64, requeue bool) error { return nil }
func TestMalformedMessageRequiresConfirmedDeadLetter(t *testing.T) {
	for _, sendErr := range []error{nil, errors.New("broker unavailable")} {
		sender := &recordingSender{err: sendErr}
		ack := &recordingACK{}
		consumer := Consumer{Sender: sender, SourceName: "test_source", MaxAge: time.Hour, RetryLimit: 3, Metrics: &Metrics{}}
		err := consumer.Handle(context.Background(), amqp.Delivery{Body: []byte("invalid"), Acknowledger: ack})
		if sender.destination != "dead" {
			t.Fatal("malformed event was not quarantined")
		}
		if sendErr == nil && (err != nil || !ack.acked || ack.nacked) {
			t.Fatal("confirmed dead letter not acknowledged")
		}
		if sendErr != nil && (err == nil || ack.acked || !ack.nacked) {
			t.Fatal("event acknowledged before confirmed quarantine")
		}
	}
}
func TestConsumerBrokerRetryBudgetSurvivesDatabaseOutage(t *testing.T) {
	fixture := integration(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	closed, err := OpenDatabase(fixture.cfg.Projection)
	if err != nil {
		t.Fatal(err)
	}
	closed.Close()
	consumer := fixture.consumer()
	consumer.Projection.DB = closed
	fixture.publish(t, fixture.envelope("database_outage"))
	for index := 0; index < fixture.cfg.RetryLimit; index++ {
		if err := consumer.Handle(ctx, fixture.get(t, "r5.reports")); err != nil {
			t.Fatal(err)
		}
	}
	delivery := fixture.get(t, "r5.dead.queue")
	defer delivery.Ack(false)
	attempt, valid := retryNumber(delivery.Headers, fixture.cfg.RetryLimit)
	if !valid || attempt != fixture.cfg.RetryLimit {
		t.Fatal("database outage removed the durable broker retry budget")
	}
}
func TestIntegrationPublisherConfirmFailureBudgetAndUnroutable(t *testing.T) {
	fixture := integration(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	bridge := Bridge{Source: fixture.source, Projection: fixture.projection, SourceName: fixture.cfg.SourceName, Metrics: fixture.metrics}
	if err := bridge.Once(ctx); err != nil {
		t.Fatal(err)
	}
	failed := &recordingSender{err: errors.New("broker down")}
	publisher := Publisher{Projection: fixture.projection, Sender: failed, RetryLimit: 3, Metrics: fixture.metrics}
	for index := 0; index < 3; index++ {
		if err := publisher.Once(ctx); err == nil {
			t.Fatal("failed confirmation treated as published")
		}
		fixture.projection.DB.ExecContext(ctx, "UPDATE r5_deliveries SET next_attempt_at=UTC_TIMESTAMP(3)")
	}
	var state string
	var attempts int
	fixture.projection.DB.QueryRowContext(ctx, "SELECT status,attempts FROM r5_deliveries LIMIT 1").Scan(&state, &attempts)
	if state != "needs_repair" || attempts != 3 {
		t.Fatal("publisher retry not bounded")
	}
	if err := fixture.broker.Channel.ExchangeDeclare("r5.unbound", "direct", false, false, false, false, nil); err != nil {
		t.Fatal(err)
	}
	defer fixture.broker.Channel.ExchangeDelete("r5.unbound", false, false)
	if err := fixture.broker.Channel.QueueUnbind("r5.reports", "settled", "r5.events", nil); err != nil {
		t.Fatal(err)
	}
	defer fixture.broker.Channel.QueueBind("r5.reports", "settled", "r5.events", false, nil)
	envelope := fixture.envelope("unroutable_case")
	data, _ := json.Marshal(envelope)
	if err := fixture.broker.Publish(ctx, "events", data, nil); err == nil {
		t.Fatal("unroutable publish incorrectly confirmed")
	}
}

func TestIntegrationBrokerLeastPrivilege(t *testing.T) {
	fixture := integration(t)
	for _, cfg := range []BrokerConfig{fixture.cfg.Publisher, fixture.cfg.Consumer} {
		broker, err := OpenBroker(cfg)
		if err != nil {
			t.Fatal("runtime broker identity rejected")
		}
		_, err = broker.Channel.QueueDeclare("r5.unauthorized", true, false, false, false, nil)
		broker.Close()
		if err == nil {
			t.Fatal("runtime broker identity could configure topology")
		}
	}
	publisher, err := OpenBroker(fixture.cfg.Publisher)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = publisher.Channel.Get("r5.reports", false)
	publisher.Close()
	if err == nil {
		t.Fatal("publisher could consume")
	}
	consumer, err := OpenBroker(fixture.cfg.Consumer)
	if err != nil {
		t.Fatal(err)
	}
	defer consumer.Close()
	data, _ := json.Marshal(fixture.envelope("unauthorized_publish"))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := consumer.Publish(ctx, "events", data, nil); err == nil {
		t.Fatal("consumer could publish source events")
	}
}
func TestIntegrationOutOfOrderReportsAndDistinctIDs(t *testing.T) {
	fixture := integration(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	later := fixture.report
	later.RunID = "run_later_fixture"
	for _, envelope := range []Envelope{NewEnvelope("later", fixture.cfg.SourceName, time.Now().UTC(), later), fixture.envelope("earlier"), fixture.envelope("same_run_other_id")} {
		fixture.publish(t, envelope)
		if err := fixture.consumer().Handle(ctx, fixture.get(t, "r5.reports")); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	fixture.projection.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM r5_report_runs").Scan(&count)
	if count != 2 {
		t.Fatal("report ordering or alias event changed results")
	}
}
func TestConfigCannotTargetDevelopmentOrPrivilegedAccounts(t *testing.T) {
	cfg := Config{Source: DatabaseConfig{Address: "127.0.0.1:23306", Name: "game_realtime_v2_r5_restore", User: "r5_reader", Password: strings.Repeat("a", 32)}, Projection: DatabaseConfig{Address: "127.0.0.1:23306", Name: "game_realtime_r5_reports", User: "r5_projector", Password: strings.Repeat("b", 32)}, Publisher: BrokerConfig{Address: "127.0.0.1:25672", VHost: "r5", User: "r5_publisher", Password: strings.Repeat("c", 32)}, Consumer: BrokerConfig{Address: "127.0.0.1:25672", VHost: "r5", User: "r5_consumer", Password: strings.Repeat("d", 32)}, SourceName: "test_source", RPCAddress: "127.0.0.1:0", Metrics: map[string]string{"rpc": "127.0.0.1:0", "bridge": "127.0.0.1:0", "publisher": "127.0.0.1:0", "consumer": "127.0.0.1:0"}, Identities: testIdentities(), RetryLimit: 3, RetryDelayMS: 500, MaxAgeSeconds: 86400}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*Config){func(config *Config) { config.Source.Name = "game_realtime" }, func(config *Config) { config.Source.User = "root" }, func(config *Config) { config.RPCAddress = "0.0.0.0:18090" }, func(config *Config) { config.Consumer.User = "r5_admin" }} {
		changed := cfg
		mutate(&changed)
		if changed.Validate() == nil {
			t.Fatal("unsafe experiment target accepted")
		}
	}
}
