package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"game-realtime-gm/backend/internal/pve/run"
	"game-realtime-gm/backend/internal/pve/store"
)

func main() {
	if err := execute(); err != nil {
		fmt.Fprintln(os.Stderr, "pve_event_bot:", err)
		os.Exit(1)
	}
}
func execute() error {
	endpoint := flag.String("endpoint", "http://127.0.0.1:8090", "internal test listener")
	runID := flag.String("run-id", "", "assigned run id")
	players := flag.String("players", "", "comma separated assigned player ids")
	start := flag.Int64("start-sequence", 1, "first trusted sequence")
	kind := flag.String("event", "scenario", "scenario or single event type")
	target := flag.String("target", "", "unique target/action id")
	flag.Parse()
	parsed, err := url.Parse(*endpoint)
	if err != nil || parsed.Scheme != "http" || parsed.Hostname() != "127.0.0.1" || *runID == "" {
		return fmt.Errorf("endpoint must use loopback HTTP and run-id is required")
	}
	token := os.Getenv("PVE_TEST_EVENTS_TOKEN")
	if len(token) < 24 {
		return fmt.Errorf("set PVE_TEST_EVENTS_TOKEN in environment")
	}
	ids := []int64{}
	for _, value := range strings.Split(*players, ",") {
		id, err := strconv.ParseInt(value, 10, 64)
		if err != nil || id <= 0 {
			return fmt.Errorf("invalid player ids")
		}
		ids = append(ids, id)
	}
	client := &http.Client{Timeout: 5 * time.Second}
	sequence := *start
	send := func(eventType string, actor int64, object string) error {
		event := run.Event{EventID: store.ID("bot"), Source: "pve_event_bot", SourceGeneration: 1, RunID: *runID, Sequence: sequence, SchemaVersion: 2, EventType: eventType, ActorPlayerID: &actor, Contributors: ids, TargetID: object, OccurredAt: time.Now().UTC()}
		body, _ := json.Marshal(event)
		request, err := http.NewRequestWithContext(context.Background(), http.MethodPost, strings.TrimSuffix(*endpoint, "/")+"/internal/v2/runs/"+*runID+"/events", bytes.NewReader(body))
		if err != nil {
			return err
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-PVE-Test-Token", token)
		response, err := client.Do(request)
		if err != nil {
			return err
		}
		defer response.Body.Close()
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
		if response.StatusCode != 200 {
			return fmt.Errorf("event rejected: HTTP %d sequence %d", response.StatusCode, sequence)
		}
		sequence++
		return nil
	}
	if *kind != "scenario" {
		return send(*kind, ids[0], *target)
	}
	for _, id := range ids {
		if err := send("loaded", id, ""); err != nil {
			return err
		}
	}
	for index := 0; index < 3; index++ {
		if err := send("kill", ids[index%len(ids)], "enemy_"+strconv.Itoa(index)); err != nil {
			return err
		}
	}
	if err := send("interact", ids[0], "terminal"); err != nil {
		return err
	}
	if err := send("reach", ids[len(ids)-1], "exit"); err != nil {
		return err
	}
	fmt.Println("trusted scenario submitted; query participant results after worker settlement")
	return nil
}
