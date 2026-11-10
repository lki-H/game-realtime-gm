package experiment

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRetryValidationPreservesIdentityAndRejectsChangedOrExpiredContent(t *testing.T) {
	cfg, err := aggregateTestConfig().ForRole("ops")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	envelope := NewEnvelope("manual_case", cfg.SourceName, now, fixtureReport())
	request := RetryRequest{MessageID: envelope.MessageID, ExpectedHash: envelope.PayloadHash, ExpectedFingerprint: envelope.Fingerprint(), Reason: "broker restored"}
	for _, test := range []struct {
		name   string
		mutate func(*Envelope, *RetryRequest)
		state  string
	}{
		{"valid", func(*Envelope, *RetryRequest) {}, "needs_repair"},
		{"hash", func(_ *Envelope, r *RetryRequest) { r.ExpectedHash = Hash([]byte("other")) }, "needs_repair"},
		{"time", func(e *Envelope, _ *RetryRequest) { e.PublishedAt = e.PublishedAt.Add(time.Second) }, "needs_repair"},
		{"expired", func(e *Envelope, _ *RetryRequest) { e.PublishedAt = now.Add(-2 * cfg.MaxAge()) }, "needs_repair"},
		{"source", func(e *Envelope, _ *RetryRequest) { e.Source = "another_source" }, "needs_repair"},
		{"state", func(*Envelope, *RetryRequest) {}, "published"},
	} {
		t.Run(test.name, func(t *testing.T) {
			value, req := envelope, request
			test.mutate(&value, &req)
			data, _ := json.Marshal(value)
			got, _, err := validateRetry(data, envelope.PayloadHash, test.state, 3, cfg, req, now)
			if test.name == "valid" {
				if err != nil || got.Fingerprint() != envelope.Fingerprint() {
					t.Fatal("original message changed")
				}
			} else if err == nil {
				t.Fatal("unsafe retry accepted")
			}
		})
	}
}
func TestPrivateAuditLocksSyncsAndBlocksUnresolvedMessage(t *testing.T) {
	repository := t.TempDir()
	directory := t.TempDir()
	if _, err := OpenAudit(filepath.Join(repository, "audit"), repository); err == nil {
		t.Fatal("repository audit accepted")
	}
	audit, err := OpenAudit(directory, repository)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := OpenAudit(directory, repository); err == nil {
		t.Fatal("concurrent audit writer accepted")
	}
	request := RetryRequest{MessageID: "audit_case", ExpectedHash: Hash([]byte("payload")), ExpectedFingerprint: Hash([]byte("envelope")), Reason: "broker restored", Execute: true}
	id, err := audit.Intent(request, DeliveryInspection{MessageID: request.MessageID, Status: "needs_repair", Attempts: 3})
	if err != nil {
		t.Fatal(err)
	}
	if err := audit.Result(id, request.MessageID, "unknown", DeliveryInspection{MessageID: request.MessageID, Status: "pending", Attempts: 3}); err != nil {
		t.Fatal(err)
	}
	if err := audit.Close(); err != nil {
		t.Fatal(err)
	}
	audit, err = OpenAudit(directory, repository)
	if err != nil {
		t.Fatal(err)
	}
	defer audit.Close()
	if _, err := audit.Intent(request, DeliveryInspection{}); err == nil {
		t.Fatal("unknown commit retried automatically")
	}
	other := request
	other.MessageID = "independent_case"
	id, err = audit.Intent(other, DeliveryInspection{})
	if err != nil {
		t.Fatal(err)
	}
	if err := audit.Result(id, other.MessageID, "rejected", DeliveryInspection{}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(directory, "manifest.jsonl"))
	if err != nil || len(data) == 0 {
		t.Fatal("durable manifest missing")
	}
}

func TestAuditWriteFailureKeepsLockForManualInspection(t *testing.T) {
	repository, directory := t.TempDir(), t.TempDir()
	audit, err := OpenAudit(directory, repository)
	if err != nil {
		t.Fatal(err)
	}
	if err := audit.file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := audit.Intent(RetryRequest{MessageID: "audit_write_failure", Reason: "broker restored"}, DeliveryInspection{}); err == nil {
		t.Fatal("audit write failure accepted")
	}
	_ = audit.Close()
	if _, err := os.Stat(filepath.Join(directory, "manifest.lock")); err != nil {
		t.Fatal("uncertain audit lost its inspection lock")
	}
	if _, err := OpenAudit(directory, repository); err == nil {
		t.Fatal("uncertain audit reopened without manual review")
	}
}
func TestIntegrationManualRetryKeepsBudgetAndReportsOnly(t *testing.T) {
	fixture := integration(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	envelope := fixture.envelope("manual_budget_case")
	data, _ := json.Marshal(envelope)
	if _, err := fixture.projection.DB.ExecContext(ctx, "INSERT INTO r5_deliveries(message_id,payload_hash,envelope,status,attempts) VALUES(?,?,?,'needs_repair',?)", envelope.MessageID, envelope.PayloadHash, data, fixture.cfg.RetryLimit); err != nil {
		t.Fatal(err)
	}
	cfg, err := fixture.cfg.ForRole("ops")
	if err != nil {
		t.Fatal(err)
	}
	request := RetryRequest{MessageID: envelope.MessageID, ExpectedHash: envelope.PayloadHash, ExpectedFingerprint: envelope.Fingerprint(), Reason: "broker restored"}
	result, err := fixture.projection.ManualRetry(ctx, cfg, request, nil)
	if err != nil || result.Executed || result.After.Attempts != fixture.cfg.RetryLimit {
		t.Fatal("dry run changed retry budget")
	}
	var state string
	fixture.projection.DB.QueryRowContext(ctx, "SELECT status FROM r5_deliveries WHERE message_id=?", envelope.MessageID).Scan(&state)
	if state != "needs_repair" {
		t.Fatal("dry run wrote database")
	}
	repository, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	audit, err := OpenAudit(t.TempDir(), repository)
	if err != nil {
		t.Fatal(err)
	}
	defer audit.Close()
	request.Execute = true
	result, err = fixture.projection.ManualRetry(ctx, cfg, request, audit)
	if err != nil || !result.Executed || result.After.Attempts != fixture.cfg.RetryLimit || result.After.Fingerprint != envelope.Fingerprint() {
		t.Fatal("manual retry changed immutable message or budget")
	}
	if _, err := fixture.projection.ManualRetry(ctx, cfg, request, audit); err == nil {
		t.Fatal("CAS accepted duplicate retry")
	}
	publisher := Publisher{Projection: fixture.projection, Sender: &recordingSender{err: errors.New("broker still down")}, RetryLimit: fixture.cfg.RetryLimit, Metrics: fixture.metrics}
	if publisher.Once(ctx) == nil {
		t.Fatal("failed manual opportunity reported success")
	}
	var attempts int
	var afterData []byte
	if err := fixture.projection.DB.QueryRowContext(ctx, "SELECT status,attempts,envelope FROM r5_deliveries WHERE message_id=?", envelope.MessageID).Scan(&state, &attempts, &afterData); err != nil {
		t.Fatal(err)
	}
	if state != "needs_repair" || attempts != fixture.cfg.RetryLimit+1 {
		t.Fatal("manual retry escaped finite budget")
	}
	parsed, _, err := DecodeEnvelope(afterData, cfg.SourceName, time.Now().UTC(), cfg.MaxAge())
	if err != nil || parsed.Fingerprint() != envelope.Fingerprint() {
		t.Fatal("manual retry changed original message")
	}
	inspection, err := fixture.projection.Inspect(ctx, 10, cfg.MaxAge())
	if err != nil || inspection.DeliveryCounts["needs_repair"] != 1 || inspection.OldestFormedAt == nil {
		t.Fatal("reporting database inspection incomplete")
	}
}

func TestInspectionSeparatesExpiredMessagesFromRetrySchedule(t *testing.T) {
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	var result Inspection
	observe := func(status string, next, formed time.Time) {
		result.observeWaiting(status, next, sql.NullString{String: formed.Format(time.RFC3339Nano), Valid: true}, now, time.Hour)
	}
	observe("needs_repair", now.Add(-24*time.Hour), now.Add(-2*time.Hour))
	observe("pending", now.Add(time.Minute), now.Add(-30*time.Minute))
	observe("retry", now.Add(-time.Minute), now.Add(-time.Hour))
	result.observeWaiting("retry", now.Add(2*time.Minute), sql.NullString{String: "bad", Valid: true}, now, time.Hour)
	if result.ExpiredWaiting != 1 || result.InvalidTimes != 1 || result.OldestFormedAt == nil || !result.OldestFormedAt.Equal(now.Add(-2*time.Hour)) {
		t.Fatal("expiry/oldest/invalid timestamp counts disagree")
	}
	if result.EarliestNextAttempt == nil || !result.EarliestNextAttempt.Equal(now.Add(-time.Minute)) {
		t.Fatal("paused repair schedule displaced next automatic retry")
	}
}

func TestAuditRejectsRepositoryAncestorsBeforeCreatingManifest(t *testing.T) {
	ancestor := t.TempDir()
	repository := filepath.Join(ancestor, "repo")
	if err := os.Mkdir(repository, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenAudit(ancestor, repository); err == nil {
		t.Fatal("repository ancestor accepted as audit directory")
	}
	if _, err := os.Stat(filepath.Join(ancestor, "manifest.jsonl")); !os.IsNotExist(err) {
		t.Fatal("rejected ancestor was mutated")
	}
	if _, err := os.Stat(filepath.Join(ancestor, "manifest.lock")); !os.IsNotExist(err) {
		t.Fatal("rejected ancestor received a lock")
	}
}
