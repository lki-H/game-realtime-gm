package experiment

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestReviewQuorumRedeliveryExhaustionPreservesDeadLetter(t *testing.T) {
	fixture := integration(t)
	fixture.publish(t, fixture.envelope("repeated_crash"))
	for index := 0; index < 21; index++ {
		channel, err := fixture.broker.Connection.Channel()
		if err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(2 * time.Second)
		found := false
		for time.Now().Before(deadline) {
			delivery, available, getErr := channel.Get("r5.reports", false)
			if getErr != nil {
				t.Fatal(getErr)
			}
			if available {
				found = true
				if err := delivery.Reject(true); err != nil {
					t.Fatal(err)
				}
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		channel.Close()
		if !found {
			break
		}
	}
	dead := fixture.get(t, "r5.dead.queue")
	defer dead.Ack(false)
	if dead.MessageId != "repeated_crash" {
		t.Fatal("redelivery budget discarded the message")
	}
}
func TestReviewKnownReceiptSuppressesExpiredReplay(t *testing.T) {
	fixture := integration(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	envelope := fixture.envelope("expired_duplicate")
	envelope.PublishedAt = time.Now().Add(-2 * time.Minute)
	if _, err := fixture.projection.Apply(ctx, envelope, fixture.report); err != nil {
		t.Fatal(err)
	}
	consumer := fixture.consumer()
	consumer.MaxAge = time.Minute
	fixture.publish(t, envelope)
	if err := consumer.Handle(ctx, fixture.get(t, "r5.reports")); err != nil {
		t.Fatal(err)
	}
	var count int
	fixture.projection.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM r5_report_runs").Scan(&count)
	if count != 1 {
		t.Fatal("expired retry changed settled report")
	}
	if _, found, err := fixture.broker.Channel.Get("r5.dead.queue", false); err != nil || found {
		t.Fatal("already committed matching replay incorrectly quarantined")
	}
	changed := envelope
	changed.PublishedAt = changed.PublishedAt.Add(-time.Second)
	data, _ := json.Marshal(changed)
	if err := fixture.broker.Publish(ctx, "events", data, nil); err != nil {
		t.Fatal(err)
	}
	if err := consumer.Handle(ctx, fixture.get(t, "r5.reports")); err != nil {
		t.Fatal(err)
	}
	dead := fixture.get(t, "r5.dead.queue")
	dead.Ack(false)
}
