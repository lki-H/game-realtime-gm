package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	reportingv1 "game-realtime-gm/experiments/r5-m6/gen/reporting/v1"
	"game-realtime-gm/experiments/r5-m6/internal/experiment"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func main() {
	config := flag.String("config", "", "private runtime configuration")
	clients := flag.String("clients", "", "private client token file")
	runID := flag.String("run", "", "optional settled run")
	player := flag.Int64("player", 0, "scoped player for run query")
	flag.Parse()
	cfg, err := experiment.LoadConfig(*config)
	if err != nil {
		fail("configuration_rejected")
	}
	data, err := os.ReadFile(*clients)
	if err != nil {
		fail("identity_file_unavailable")
	}
	var tokens map[string]string
	if json.Unmarshal(data, &tokens) != nil || tokens["r5-report-client"] == "" {
		fail("identity_file_invalid")
	}
	connection, err := grpc.NewClient(cfg.RPCAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		fail("client_connection_failed")
	}
	defer connection.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ctx = metadata.AppendToOutgoingContext(ctx, "service-id", "r5-report-client", "authorization", "Bearer "+tokens["r5-report-client"])
	client := reportingv1.NewReportingClient(connection)
	var response any
	if *runID == "" {
		response, err = client.GetLeaderboard(ctx, &reportingv1.GetLeaderboardRequest{Limit: 10})
	} else {
		response, err = client.GetRunResult(ctx, &reportingv1.GetRunResultRequest{RunId: *runID, PlayerId: *player})
	}
	if err != nil {
		fail(status.Code(err).String())
	}
	json.NewEncoder(os.Stdout).Encode(response)
}
func fail(code string) { fmt.Fprintln(os.Stderr, "R5 query failed:", code); os.Exit(1) }
