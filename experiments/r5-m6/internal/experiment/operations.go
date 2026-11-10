package experiment

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

var ErrCommitUnknown = errors.New("retry_commit_unknown_inspect_before_further_action")
var ErrAuditUnknown = errors.New("retry_audit_incomplete_inspect_before_further_action")

type DeliveryInspection struct {
	MessageID     string    `json:"message_id"`
	Status        string    `json:"status"`
	Attempts      int       `json:"attempts"`
	PayloadHash   string    `json:"payload_hash"`
	Fingerprint   string    `json:"fingerprint,omitempty"`
	PublishedAt   time.Time `json:"first_formed_at,omitempty"`
	NextAttemptAt time.Time `json:"next_attempt_at"`
	AgeSeconds    int64     `json:"age_seconds,omitempty"`
}
type Inspection struct {
	DeliveryCounts      map[string]int64     `json:"delivery_counts"`
	ReceiptCounts       map[string]int64     `json:"receipt_counts"`
	Due                 int64                `json:"due_deliveries"`
	ExpiredWaiting      int64                `json:"expired_waiting_deliveries"`
	EarliestNextAttempt *time.Time           `json:"earliest_next_attempt_at,omitempty"`
	OldestFormedAt      *time.Time           `json:"oldest_waiting_first_formed_at,omitempty"`
	InvalidTimes        int64                `json:"invalid_waiting_timestamps"`
	Deliveries          []DeliveryInspection `json:"deliveries"`
	// This is a database view, not the RabbitMQ dead queue depth.
}

func (projection Projection) Inspect(ctx context.Context, limit int, maxAge time.Duration) (Inspection, error) {
	result := Inspection{DeliveryCounts: map[string]int64{}, ReceiptCounts: map[string]int64{}, Deliveries: []DeliveryInspection{}}
	if limit < 1 || limit > 100 {
		return result, errors.New("inspection_limit_must_be_1_to_100")
	}
	if maxAge < time.Minute || maxAge > 7*24*time.Hour {
		return result, errors.New("inspection_max_age_out_of_range")
	}
	tx, err := projection.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	for _, entry := range []struct {
		query  string
		target map[string]int64
	}{
		{"SELECT status,COUNT(*) FROM r5_deliveries GROUP BY status", result.DeliveryCounts},
		{"SELECT status,COUNT(*) FROM r5_receipts GROUP BY status", result.ReceiptCounts},
	} {
		rows, err := tx.QueryContext(ctx, entry.query)
		if err != nil {
			return result, err
		}
		for rows.Next() {
			var status string
			var count int64
			if err := rows.Scan(&status, &count); err != nil {
				rows.Close()
				return result, err
			}
			entry.target[status] = count
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return result, err
		}
	}
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM r5_deliveries WHERE status IN ('pending','retry') AND next_attempt_at<=UTC_TIMESTAMP(3)").Scan(&result.Due); err != nil {
		return result, err
	}
	// Parse RFC3339Nano rather than sorting variable-precision timestamp strings.
	// Only the isolated reporting database is scanned; context bounds the scan.
	rows, err := tx.QueryContext(ctx, "SELECT status,next_attempt_at,JSON_UNQUOTE(JSON_EXTRACT(envelope,'$.published_at')) FROM r5_deliveries WHERE status IN ('pending','retry','needs_repair')")
	if err != nil {
		return result, err
	}
	now := time.Now().UTC()
	for rows.Next() {
		var status string
		var next time.Time
		var raw sql.NullString
		if err := rows.Scan(&status, &next, &raw); err != nil {
			rows.Close()
			return result, err
		}
		result.observeWaiting(status, next, raw, now, maxAge)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	rows, err = tx.QueryContext(ctx, "SELECT message_id,status,attempts,payload_hash,envelope,next_attempt_at FROM r5_deliveries WHERE status IN ('pending','retry','needs_repair') ORDER BY next_attempt_at,message_id LIMIT ?", limit)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var item DeliveryInspection
		var data []byte
		if err := rows.Scan(&item.MessageID, &item.Status, &item.Attempts, &item.PayloadHash, &data, &item.NextAttemptAt); err != nil {
			rows.Close()
			return result, err
		}
		var envelope Envelope
		if strictJSON(data, &envelope) == nil {
			// Normalize payload as the consumer does before calculating fingerprint.
			var report Report
			if strictJSON(envelope.Payload, &report) == nil {
				envelope = normalizeEnvelope(envelope, report)
				item.Fingerprint = envelope.Fingerprint()
			}
			item.PublishedAt = envelope.PublishedAt
			if !item.PublishedAt.IsZero() {
				item.AgeSeconds = int64(now.Sub(item.PublishedAt).Seconds())
			}
		}
		result.Deliveries = append(result.Deliveries, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	return result, tx.Commit()
}

func (result *Inspection) observeWaiting(status string, next time.Time, raw sql.NullString, now time.Time, maxAge time.Duration) {
	// needs_repair is paused until explicit operator action, so its stale
	// schedule is not the next automatic publish attempt.
	if (status == "pending" || status == "retry") && (result.EarliestNextAttempt == nil || next.Before(*result.EarliestNextAttempt)) {
		value := next.UTC()
		result.EarliestNextAttempt = &value
	}
	stamp, err := time.Parse(time.RFC3339Nano, raw.String)
	if !raw.Valid || err != nil || stamp.IsZero() {
		result.InvalidTimes++
		return
	}
	if result.OldestFormedAt == nil || stamp.Before(*result.OldestFormedAt) {
		value := stamp.UTC()
		result.OldestFormedAt = &value
	}
	if now.Sub(stamp) > maxAge {
		result.ExpiredWaiting++
	}
}

type RetryRequest struct {
	MessageID           string `json:"message_id"`
	ExpectedHash        string `json:"expected_hash"`
	ExpectedFingerprint string `json:"expected_fingerprint"`
	Reason              string `json:"reason"`
	Execute             bool   `json:"-"`
}
type RetryResult struct {
	OperationID string             `json:"operation_id,omitempty"`
	Executed    bool               `json:"executed"`
	Before      DeliveryInspection `json:"before"`
	After       DeliveryInspection `json:"after"`
}

func validDigest(value string) bool {
	return len(value) == 64 && strings.Trim(value, "0123456789abcdef") == ""
}

func validateRetry(data []byte, storedHash, status string, attempts int, cfg RoleConfig, request RetryRequest, now time.Time) (Envelope, Report, error) {
	if !messageIdentifier.MatchString(request.MessageID) || !validDigest(request.ExpectedHash) || !validDigest(request.ExpectedFingerprint) || len(strings.TrimSpace(request.Reason)) < 3 || len(request.Reason) > 256 || strings.ContainsAny(request.Reason, "\r\n\x00") {
		return Envelope{}, Report{}, errors.New("retry_request_rejected")
	}
	if status != "needs_repair" || attempts < 0 {
		return Envelope{}, Report{}, errors.New("retry_requires_needs_repair")
	}
	envelope, report, err := DecodeEnvelope(data, cfg.SourceName, now, cfg.MaxAge())
	if err != nil {
		return envelope, report, err
	}
	if envelope.MessageID != request.MessageID || storedHash != request.ExpectedHash || envelope.PayloadHash != request.ExpectedHash || envelope.Fingerprint() != request.ExpectedFingerprint {
		return envelope, report, ErrConflict
	}
	return envelope, report, nil
}

func (projection Projection) ManualRetry(ctx context.Context, cfg RoleConfig, request RetryRequest, audit *AuditLog) (RetryResult, error) {
	var result RetryResult
	if err := cfg.ValidateForRole("ops"); err != nil {
		return result, err
	}
	if request.Execute && audit == nil {
		return result, errors.New("private_external_audit_required")
	}
	tx, err := projection.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: !request.Execute})
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	query := "SELECT message_id,status,attempts,payload_hash,envelope,next_attempt_at FROM r5_deliveries WHERE message_id=?"
	if request.Execute {
		query += " FOR UPDATE"
	}
	var data []byte
	before := &result.Before
	if err := tx.QueryRowContext(ctx, query, request.MessageID).Scan(&before.MessageID, &before.Status, &before.Attempts, &before.PayloadHash, &data, &before.NextAttemptAt); err != nil {
		return result, err
	}
	now := time.Now().UTC()
	envelope, report, err := validateRetry(data, before.PayloadHash, before.Status, before.Attempts, cfg, request, now)
	if err != nil {
		return result, err
	}
	before.PublishedAt, before.Fingerprint = envelope.PublishedAt, envelope.Fingerprint()
	before.AgeSeconds = int64(now.Sub(envelope.PublishedAt).Seconds())
	var receiptFingerprint, receiptStatus string
	err = tx.QueryRowContext(ctx, "SELECT fingerprint,status FROM r5_receipts WHERE message_id=?", request.MessageID).Scan(&receiptFingerprint, &receiptStatus)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return result, err
	}
	if err == nil && (receiptFingerprint != before.Fingerprint || receiptStatus == "applied" || receiptStatus == "dead") {
		return result, ErrConflict
	}
	var reportHash string
	err = tx.QueryRowContext(ctx, "SELECT payload_hash FROM r5_report_runs WHERE source=? AND run_id=?", envelope.Source, report.RunID).Scan(&reportHash)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return result, err
	}
	if err == nil && reportHash != envelope.PayloadHash {
		return result, ErrConflict
	}
	result.After = result.Before
	result.After.Status, result.After.NextAttemptAt = "pending", now.Truncate(time.Millisecond)
	if !request.Execute {
		return result, tx.Commit()
	}
	operationID, err := audit.Intent(request, result.Before)
	if err != nil {
		return result, err
	}
	result.OperationID = operationID
	update, err := tx.ExecContext(ctx, "UPDATE r5_deliveries SET status='pending',next_attempt_at=? WHERE message_id=? AND status='needs_repair' AND attempts=? AND payload_hash=?", result.After.NextAttemptAt, request.MessageID, before.Attempts, request.ExpectedHash)
	if err == nil {
		var count int64
		count, err = update.RowsAffected()
		if err == nil && count != 1 {
			err = ErrConflict
		}
	}
	if err != nil {
		phase := "rejected"
		if tx.Rollback() != nil {
			phase = "unknown"
		}
		if audit.Result(operationID, request.MessageID, phase, result.Before) != nil {
			return result, ErrAuditUnknown
		}
		return result, err
	}
	if tx.Commit() != nil {
		_ = audit.Result(operationID, request.MessageID, "unknown", result.After)
		return result, ErrCommitUnknown
	}
	result.Executed = true
	if audit.Result(operationID, request.MessageID, "committed", result.After) != nil {
		return result, ErrAuditUnknown
	}
	return result, nil
}
