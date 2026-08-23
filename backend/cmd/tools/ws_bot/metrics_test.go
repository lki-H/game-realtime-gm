package main

import (
	"errors"
	"testing"
	"time"
)

func TestMetricsSnapshot(t *testing.T) {
	measurements := newMetrics()
	for value := 1; value <= 20; value++ {
		measurements.RecordSuccess("debug_echo", time.Duration(value)*time.Millisecond)
	}
	measurements.RecordFailure("debug_echo", withCategory("server_code_40926", errors.New("failed")))

	snapshots := measurements.Snapshots()
	if len(snapshots) != 1 {
		t.Fatalf("expected one snapshot, got %d", len(snapshots))
	}
	snapshot := snapshots[0]
	if snapshot.Success != 20 || snapshot.Failure != 1 {
		t.Fatalf("unexpected counts: %+v", snapshot)
	}
	if snapshot.Average != 10*time.Millisecond+500*time.Microsecond {
		t.Fatalf("unexpected average: %s", snapshot.Average)
	}
	if snapshot.P95 != 19*time.Millisecond {
		t.Fatalf("unexpected p95: %s", snapshot.P95)
	}
	if snapshot.Maximum != 20*time.Millisecond {
		t.Fatalf("unexpected maximum: %s", snapshot.Maximum)
	}

	failures := measurements.FailureSnapshots()
	if len(failures) != 1 || failures[0].Category != "server_code_40926" || failures[0].Count != 1 {
		t.Fatalf("unexpected failures: %+v", failures)
	}
}

func TestRunConfigValidate(t *testing.T) {
	valid := runConfig{
		HTTPBaseURL:    "http://127.0.0.1:8080",
		WebSocketURL:   "ws://127.0.0.1:8080/ws",
		UsernamePrefix: "day34bot",
		Password:       "123456",
		Clients:        8,
		SquadSize:      4,
		EchoRounds:     20,
		RequestTimeout: 10 * time.Second,
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}

	invalid := valid
	invalid.Clients = 7
	if err := invalid.Validate(); err == nil {
		t.Fatal("non-divisible client count should be rejected")
	}
}
