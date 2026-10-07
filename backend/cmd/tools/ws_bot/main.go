package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"time"
)

func main() {
	config := parseFlags()
	if err := config.Validate(); err != nil {
		fmt.Fprintln(os.Stderr, "invalid configuration:", err)
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	measurements := newMetrics()
	runner := newRunner(config, measurements)
	startedAt := time.Now()
	err := runner.Run(ctx)
	measurements.WriteReport(os.Stdout, config, time.Since(startedAt))
	if err != nil {
		fmt.Fprintln(os.Stderr, "ws_bot failed:", err)
		os.Exit(1)
	}
}

func parseFlags() runConfig {
	var config runConfig
	flag.StringVar(&config.HTTPBaseURL, "http-url", "http://127.0.0.1:8080", "backend HTTP base URL")
	flag.StringVar(&config.WebSocketURL, "ws-url", "ws://127.0.0.1:8080/ws", "backend WebSocket URL without token")
	flag.StringVar(&config.UsernamePrefix, "username-prefix", "day34bot", "test account username prefix")
	flag.StringVar(&config.Password, "password", "123456", "test account password")
	flag.IntVar(&config.Clients, "clients", 8, "number of concurrent players")
	flag.IntVar(&config.SquadSize, "squad-size", 4, "players per squad, from 2 to 4")
	flag.IntVar(&config.EchoRounds, "echo-rounds", 20, "debug.echo requests per player")
	flag.BoolVar(&config.EnsureAccounts, "ensure-accounts", true, "register accounts before login; duplicate usernames are reused")
	flag.DurationVar(&config.RequestTimeout, "request-timeout", 10*time.Second, "timeout for each HTTP or WebSocket request")
	flag.DurationVar(&config.WarmupDuration, "warmup", 3*time.Second, "pause after squads are ready and before echo load")
	flag.DurationVar(&config.EchoInterval, "echo-interval", 0, "pause between echo requests from the same player")
	flag.DurationVar(&config.HoldDuration, "hold", 10*time.Second, "keep squads connected after echo load for observation")
	flag.Parse()
	return config
}
