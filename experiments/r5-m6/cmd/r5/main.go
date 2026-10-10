package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"game-realtime-gm/experiments/r5-m6/internal/experiment"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	mode := flag.String("mode", "rpc", "rpc, bridge, publisher or consumer")
	configuration := flag.String("config", "", "local private configuration file")
	flag.Parse()
	cfg, err := experiment.LoadConfig(*configuration)
	if err != nil {
		fail("configuration_rejected")
	}
	if *mode != "rpc" && *mode != "bridge" && *mode != "publisher" && *mode != "consumer" {
		fail("unknown_role")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	source, err := experiment.OpenDatabase(cfg.Source)
	if err != nil {
		fail("source_configuration_rejected")
	}
	defer source.Close()
	projection, err := experiment.OpenDatabase(cfg.Projection)
	if err != nil {
		fail("projection_configuration_rejected")
	}
	defer projection.Close()
	metrics := &experiment.Metrics{}
	metrics.Add("starts", 1)
	metricsServer := experiment.MetricsServer(cfg.Metrics[*mode], cfg.Identities, metrics)
	metricListener, err := net.Listen("tcp", metricsServer.Addr)
	if err != nil {
		fail("metrics_listener_failed")
	}
	metricErrors := make(chan error, 1)
	go func() { metricErrors <- metricsServer.Serve(metricListener) }()
	defer func() {
		shutdown, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = metricsServer.Shutdown(shutdown)
	}()
	runtimeErrors := make(chan error, 1)
	workerDone := make(chan struct{})
	if *mode == "rpc" {
		listener, err := net.Listen("tcp", cfg.RPCAddress)
		if err != nil {
			fail("rpc_listener_failed")
		}
		server := experiment.RPCServer(cfg, experiment.Source{DB: source}, metrics)
		go func() { runtimeErrors <- server.Serve(listener) }()
		defer func() {
			done := make(chan struct{})
			go func() { server.GracefulStop(); close(done) }()
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				server.Stop()
			}
		}()
	} else {
		go func() {
			defer close(workerDone)
			for ctx.Err() == nil {
				var workErr error
				if *mode == "bridge" {
					iteration, cancel := context.WithTimeout(ctx, 10*time.Second)
					workErr = (experiment.Bridge{Source: experiment.Source{DB: source}, Projection: experiment.Projection{DB: projection}, SourceName: cfg.SourceName, Metrics: metrics}).Once(iteration)
					cancel()
				} else {
					brokerConfig := cfg.Publisher
					if *mode == "consumer" {
						brokerConfig = cfg.Consumer
					}
					broker, connectErr := experiment.OpenBroker(brokerConfig)
					if connectErr != nil {
						workErr = connectErr
					} else {
						if *mode == "consumer" {
							consumer := experiment.Consumer{Projection: experiment.Projection{DB: projection}, Sender: broker, SourceName: cfg.SourceName, MaxAge: cfg.MaxAge(), RetryLimit: cfg.RetryLimit, Metrics: metrics}
							if os.Getenv("R5_ACCEPTANCE_FAULT") == "after_commit" && os.Getenv("R5_INTEGRATION") == "1" {
								consumer.Apply = func(call context.Context, envelope experiment.Envelope, report experiment.Report) (bool, error) {
									duplicate, applyErr := (experiment.Projection{DB: projection}).Apply(call, envelope, report)
									if applyErr == nil && !duplicate {
										os.Exit(42)
									}
									return duplicate, applyErr
								}
							}
							workErr = consumer.Run(ctx, broker)
						} else {
							iteration, cancel := context.WithTimeout(ctx, 5*time.Second)
							workErr = (experiment.Publisher{Projection: experiment.Projection{DB: projection}, Sender: broker, RetryLimit: cfg.RetryLimit, Metrics: metrics}).Once(iteration)
							cancel()
						}
						broker.Close()
					}
				}
				if workErr != nil {
					metrics.Add("worker_errors", 1)
					slog.Warn("r5 work deferred", "role", *mode, "error_code", "dependency_or_delivery_failed")
				}
				select {
				case <-ctx.Done():
				case <-time.After(time.Second):
				}
			}
			runtimeErrors <- nil
		}()
	}
	slog.Info("r5 role started", "role", *mode)
	select {
	case <-ctx.Done():
	case err := <-runtimeErrors:
		if err != nil {
			slog.Error("r5 runtime stopped", "role", *mode, "error_code", "runtime_stopped")
		}
		stop()
	case err := <-metricErrors:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("r5 metrics stopped", "error_code", "metrics_stopped")
		}
		stop()
	}
	stop()
	if *mode != "rpc" {
		select {
		case <-workerDone:
		case <-time.After(6 * time.Second):
			slog.Warn("r5 worker shutdown exceeded grace period", "role", *mode)
		}
	}
}
func fail(code string) { slog.Error("r5 startup failed", "error_code", code); os.Exit(1) }
