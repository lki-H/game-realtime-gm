package experiment

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"
)

type Projection struct{ DB *sql.DB }

func (projection Projection) Apply(ctx context.Context, envelope Envelope, report Report) (bool, error) {
	transaction, err := projection.DB.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer transaction.Rollback()
	_, err = transaction.ExecContext(ctx, "INSERT INTO r5_receipts(message_id,payload_hash,fingerprint,status) VALUES(?,?,?,'processing') ON DUPLICATE KEY UPDATE message_id=message_id", envelope.MessageID, envelope.PayloadHash, envelope.Fingerprint())
	if err != nil {
		return false, err
	}
	var hash, fingerprint, state string
	if err := transaction.QueryRowContext(ctx, "SELECT payload_hash,fingerprint,status FROM r5_receipts WHERE message_id=? FOR UPDATE", envelope.MessageID).Scan(&hash, &fingerprint, &state); err != nil {
		return false, err
	}
	if hash != envelope.PayloadHash || fingerprint != envelope.Fingerprint() {
		return false, ErrConflict
	}
	if state == "applied" {
		return true, transaction.Commit()
	}
	if state == "dead" {
		return false, ErrConflict
	}
	_, err = transaction.ExecContext(ctx, "INSERT INTO r5_report_runs(source,run_id,payload_hash,report) VALUES(?,?,?,?) ON DUPLICATE KEY UPDATE run_id=run_id", envelope.Source, report.RunID, envelope.PayloadHash, envelope.Payload)
	if err != nil {
		return false, err
	}
	var existing string
	if err := transaction.QueryRowContext(ctx, "SELECT payload_hash FROM r5_report_runs WHERE source=? AND run_id=? FOR UPDATE", envelope.Source, report.RunID).Scan(&existing); err != nil {
		return false, err
	}
	if existing != envelope.PayloadHash {
		return false, ErrConflict
	}
	_, err = transaction.ExecContext(ctx, "UPDATE r5_receipts SET status='applied',last_error=NULL,updated_at=UTC_TIMESTAMP(3) WHERE message_id=?", envelope.MessageID)
	if err != nil {
		return false, err
	}
	return false, transaction.Commit()
}
func (projection Projection) Failure(ctx context.Context, envelope Envelope, reason string, minimum, limit int, permanent bool) (int, bool, error) {
	transaction, err := projection.DB.BeginTx(ctx, nil)
	if err != nil {
		return minimum, permanent, err
	}
	defer transaction.Rollback()
	_, err = transaction.ExecContext(ctx, "INSERT INTO r5_receipts(message_id,payload_hash,fingerprint,status) VALUES(?,?,?,'retry') ON DUPLICATE KEY UPDATE message_id=message_id", envelope.MessageID, envelope.PayloadHash, envelope.Fingerprint())
	if err != nil {
		return minimum, permanent, err
	}
	var hash, fingerprint, state string
	var attempts int
	if err := transaction.QueryRowContext(ctx, "SELECT payload_hash,fingerprint,status,attempts FROM r5_receipts WHERE message_id=? FOR UPDATE", envelope.MessageID).Scan(&hash, &fingerprint, &state, &attempts); err != nil {
		return minimum, permanent, err
	}
	if hash != envelope.PayloadHash || fingerprint != envelope.Fingerprint() || state == "applied" {
		return minimum, true, ErrConflict
	}
	attempts++
	if attempts < minimum {
		attempts = minimum
	}
	dead := permanent || attempts >= limit || state == "dead"
	state = "retry"
	if dead {
		state = "dead"
	}
	_, err = transaction.ExecContext(ctx, "UPDATE r5_receipts SET status=?,attempts=?,last_error=?,updated_at=UTC_TIMESTAMP(3) WHERE message_id=?", state, attempts, reason, envelope.MessageID)
	if err != nil {
		return attempts, dead, err
	}
	return attempts, dead, transaction.Commit()
}

type Bridge struct {
	Source     Source
	Projection Projection
	SourceName string
	Metrics    *Metrics
}

func (bridge Bridge) Once(ctx context.Context) error {
	transaction, err := bridge.Projection.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer transaction.Rollback()
	if _, err := transaction.ExecContext(ctx, "INSERT IGNORE INTO r5_scan_state(source) VALUES(?)", bridge.SourceName); err != nil {
		return err
	}
	var after int64
	if err := transaction.QueryRowContext(ctx, "SELECT after_id FROM r5_scan_state WHERE source=? FOR UPDATE", bridge.SourceName).Scan(&after); err != nil {
		return err
	}
	rows, err := bridge.Source.DB.QueryContext(ctx, "SELECT id,run_id,created_at FROM r5_outbox WHERE id>? ORDER BY id LIMIT 20", after)
	if err != nil {
		return err
	}
	type candidate struct {
		id    int64
		run   string
		stamp time.Time
	}
	candidates := []candidate{}
	for rows.Next() {
		var current candidate
		if err := rows.Scan(&current.id, &current.run, &current.stamp); err != nil {
			rows.Close()
			return err
		}
		candidates = append(candidates, current)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, current := range candidates {
		result, err := bridge.Source.Result(ctx, current.run)
		if err != nil {
			return err
		}
		envelope := NewEnvelope("outbox:"+bridge.SourceName+":"+strconv.FormatInt(current.id, 10), bridge.SourceName, time.Now().UTC(), ReportFrom(result))
		data, _ := json.Marshal(envelope)
		if _, err := transaction.ExecContext(ctx, "INSERT IGNORE INTO r5_deliveries(message_id,payload_hash,envelope) VALUES(?,?,?)", envelope.MessageID, envelope.PayloadHash, data); err != nil {
			return err
		}
		var existing string
		if err := transaction.QueryRowContext(ctx, "SELECT payload_hash FROM r5_deliveries WHERE message_id=?", envelope.MessageID).Scan(&existing); err != nil {
			return err
		}
		if existing != envelope.PayloadHash {
			return ErrConflict
		}
		after = current.id
	}
	if len(candidates) == 0 {
		after = 0
	}
	if _, err := transaction.ExecContext(ctx, "UPDATE r5_scan_state SET after_id=? WHERE source=?", after, bridge.SourceName); err != nil {
		return err
	}
	if err := transaction.Commit(); err != nil {
		return err
	}
	bridge.Metrics.Add("bridge_scanned", int64(len(candidates)))
	return nil
}

type Sender interface {
	Publish(context.Context, string, []byte, map[string]any) error
}
type Publisher struct {
	Projection Projection
	Sender     Sender
	RetryLimit int
	Metrics    *Metrics
}

func (publisher Publisher) Once(ctx context.Context) error {
	transaction, err := publisher.Projection.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer transaction.Rollback()
	var id string
	var data []byte
	var attempts int
	err = transaction.QueryRowContext(ctx, "SELECT message_id,envelope,attempts FROM r5_deliveries WHERE status IN ('pending','retry') AND next_attempt_at<=UTC_TIMESTAMP(3) ORDER BY next_attempt_at,message_id LIMIT 1 FOR UPDATE SKIP LOCKED").Scan(&id, &data, &attempts)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if err = publisher.Sender.Publish(ctx, "events", data, nil); err != nil {
		attempts++
		state := "retry"
		if attempts >= publisher.RetryLimit {
			state = "needs_repair"
		}
		if _, updateErr := transaction.ExecContext(ctx, "UPDATE r5_deliveries SET status=?,attempts=?,last_error='broker_publish_failed',next_attempt_at=? WHERE message_id=?", state, attempts, time.Now().UTC().Add(time.Duration(1<<attempts)*time.Second), id); updateErr != nil {
			return updateErr
		}
		if commitErr := transaction.Commit(); commitErr != nil {
			return commitErr
		}
		publisher.Metrics.Add("publisher_failures", 1)
		slog.Warn("r5 delivery deferred", "message_id", id, "status", state, "attempts", attempts)
		return fmt.Errorf("broker_publish_failed")
	}
	if _, err := transaction.ExecContext(ctx, "UPDATE r5_deliveries SET status='published',attempts=attempts+1,published_at=UTC_TIMESTAMP(3),last_error=NULL WHERE message_id=?", id); err != nil {
		return err
	}
	if err := transaction.Commit(); err != nil {
		return err
	}
	publisher.Metrics.Add("published", 1)
	slog.Info("r5 delivery confirmed", "message_id", id)
	return nil
}
