package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"sync"
	"time"
)

type categorizedError struct {
	category string
	err      error
}

func (err *categorizedError) Error() string {
	return err.err.Error()
}

func (err *categorizedError) Unwrap() error {
	return err.err
}

func withCategory(category string, err error) error {
	if err == nil {
		return nil
	}
	return &categorizedError{category: category, err: err}
}

type stageSnapshot struct {
	Stage   string
	Success int
	Failure int
	Average time.Duration
	P95     time.Duration
	Maximum time.Duration
}

type failureSnapshot struct {
	Stage    string
	Category string
	Count    int
}

type messageSnapshot struct {
	Type  string
	Count int
}

type metrics struct {
	mu        sync.Mutex
	durations map[string][]time.Duration
	failures  map[string]map[string]int
	messages  map[string]int
}

func newMetrics() *metrics {
	return &metrics{
		durations: make(map[string][]time.Duration),
		failures:  make(map[string]map[string]int),
		messages:  make(map[string]int),
	}
}

func (measurements *metrics) RecordSuccess(stage string, duration time.Duration) {
	measurements.mu.Lock()
	defer measurements.mu.Unlock()
	measurements.durations[stage] = append(measurements.durations[stage], duration)
}

func (measurements *metrics) RecordFailure(stage string, err error) {
	measurements.mu.Lock()
	defer measurements.mu.Unlock()

	category := failureCategory(err)
	if measurements.failures[stage] == nil {
		measurements.failures[stage] = make(map[string]int)
	}
	measurements.failures[stage][category]++
}

func (measurements *metrics) RecordMessage(messageType string) {
	measurements.mu.Lock()
	defer measurements.mu.Unlock()
	measurements.messages[messageType]++
}

func (measurements *metrics) Snapshots() []stageSnapshot {
	measurements.mu.Lock()
	defer measurements.mu.Unlock()

	stageNames := make(map[string]struct{})
	for stage := range measurements.durations {
		stageNames[stage] = struct{}{}
	}
	for stage := range measurements.failures {
		stageNames[stage] = struct{}{}
	}

	snapshots := make([]stageSnapshot, 0, len(stageNames))
	for stage := range stageNames {
		durations := append([]time.Duration(nil), measurements.durations[stage]...)
		sort.Slice(durations, func(first int, second int) bool {
			return durations[first] < durations[second]
		})

		failureCount := 0
		for _, count := range measurements.failures[stage] {
			failureCount += count
		}

		snapshot := stageSnapshot{
			Stage:   stage,
			Success: len(durations),
			Failure: failureCount,
		}
		if len(durations) > 0 {
			var total time.Duration
			for _, duration := range durations {
				total += duration
			}
			position := int(math.Ceil(float64(len(durations))*0.95)) - 1
			snapshot.Average = total / time.Duration(len(durations))
			snapshot.P95 = durations[position]
			snapshot.Maximum = durations[len(durations)-1]
		}
		snapshots = append(snapshots, snapshot)
	}

	sort.Slice(snapshots, func(first int, second int) bool {
		return snapshots[first].Stage < snapshots[second].Stage
	})
	return snapshots
}

func (measurements *metrics) FailureSnapshots() []failureSnapshot {
	measurements.mu.Lock()
	defer measurements.mu.Unlock()

	result := make([]failureSnapshot, 0)
	for stage, categories := range measurements.failures {
		for category, count := range categories {
			result = append(result, failureSnapshot{Stage: stage, Category: category, Count: count})
		}
	}
	sort.Slice(result, func(first int, second int) bool {
		if result[first].Stage == result[second].Stage {
			return result[first].Category < result[second].Category
		}
		return result[first].Stage < result[second].Stage
	})
	return result
}

func (measurements *metrics) MessageSnapshots() []messageSnapshot {
	measurements.mu.Lock()
	defer measurements.mu.Unlock()

	result := make([]messageSnapshot, 0, len(measurements.messages))
	for messageType, count := range measurements.messages {
		result = append(result, messageSnapshot{Type: messageType, Count: count})
	}
	sort.Slice(result, func(first int, second int) bool {
		return result[first].Type < result[second].Type
	})
	return result
}

func (measurements *metrics) WriteReport(writer io.Writer, config runConfig, elapsed time.Duration) {
	fmt.Fprintln(writer, "")
	fmt.Fprintln(writer, "ws_bot result")
	fmt.Fprintf(writer, "clients=%d squad_size=%d echo_rounds=%d elapsed=%s\n", config.Clients, config.SquadSize, config.EchoRounds, elapsed.Round(time.Millisecond))
	fmt.Fprintln(writer, "")
	fmt.Fprintf(writer, "%-20s %8s %8s %12s %12s %12s\n", "stage", "success", "failure", "average", "p95", "maximum")
	for _, snapshot := range measurements.Snapshots() {
		fmt.Fprintf(
			writer,
			"%-20s %8d %8d %12s %12s %12s\n",
			snapshot.Stage,
			snapshot.Success,
			snapshot.Failure,
			snapshot.Average.Round(time.Microsecond),
			snapshot.P95.Round(time.Microsecond),
			snapshot.Maximum.Round(time.Microsecond),
		)
	}

	failures := measurements.FailureSnapshots()
	if len(failures) > 0 {
		fmt.Fprintln(writer, "")
		fmt.Fprintln(writer, "failure categories")
		for _, failure := range failures {
			fmt.Fprintf(writer, "%s/%s=%d\n", failure.Stage, failure.Category, failure.Count)
		}
	}

	messages := measurements.MessageSnapshots()
	if len(messages) > 0 {
		fmt.Fprintln(writer, "")
		fmt.Fprintln(writer, "received message types")
		for _, message := range messages {
			fmt.Fprintf(writer, "%s=%d\n", message.Type, message.Count)
		}
	}
}

func failureCategory(err error) string {
	if err == nil {
		return "unknown"
	}
	var categorized *categorizedError
	if errors.As(err, &categorized) {
		return categorized.category
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	return "other"
}
