package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type runConfig struct {
	HTTPBaseURL    string
	WebSocketURL   string
	UsernamePrefix string
	Password       string
	Clients        int
	SquadSize      int
	EchoRounds     int
	EnsureAccounts bool
	RequestTimeout time.Duration
	WarmupDuration time.Duration
	EchoInterval   time.Duration
	HoldDuration   time.Duration
}

func (config runConfig) Validate() error {
	if config.Clients < 2 || config.Clients > 200 {
		return fmt.Errorf("clients must be between 2 and 200")
	}
	if config.SquadSize < 2 || config.SquadSize > 4 {
		return fmt.Errorf("squad-size must be between 2 and 4")
	}
	if config.Clients%config.SquadSize != 0 {
		return fmt.Errorf("clients must be divisible by squad-size")
	}
	if config.EchoRounds < 1 || config.EchoRounds > 10000 {
		return fmt.Errorf("echo-rounds must be between 1 and 10000")
	}
	if len(config.Password) < 6 || len(config.Password) > 72 {
		return fmt.Errorf("password length must be between 6 and 72")
	}
	if strings.TrimSpace(config.UsernamePrefix) == "" || len(fmt.Sprintf("%s_%03d", config.UsernamePrefix, config.Clients)) > 64 {
		return fmt.Errorf("username-prefix produces an invalid username")
	}
	if config.RequestTimeout <= 0 || config.RequestTimeout > time.Minute {
		return fmt.Errorf("request-timeout must be greater than 0 and no more than 1 minute")
	}
	if config.WarmupDuration < 0 || config.EchoInterval < 0 || config.HoldDuration < 0 {
		return fmt.Errorf("duration flags cannot be negative")
	}
	if err := validateURL(config.HTTPBaseURL, "http", "https"); err != nil {
		return fmt.Errorf("invalid http-url: %w", err)
	}
	if err := validateURL(config.WebSocketURL, "ws", "wss"); err != nil {
		return fmt.Errorf("invalid ws-url: %w", err)
	}
	return nil
}

func validateURL(value string, allowedSchemes ...string) error {
	parsedURL, err := url.Parse(value)
	if err != nil {
		return err
	}
	if parsedURL.Host == "" {
		return fmt.Errorf("host is required")
	}
	for _, scheme := range allowedSchemes {
		if parsedURL.Scheme == scheme {
			return nil
		}
	}
	return fmt.Errorf("scheme %q is not allowed", parsedURL.Scheme)
}

type squadGroup struct {
	Clients []*botClient
	SquadID string
}

type runner struct {
	config       runConfig
	measurements *metrics
	httpClient   *http.Client
}

func newRunner(config runConfig, measurements *metrics) *runner {
	return &runner{
		config:       config,
		measurements: measurements,
		httpClient:   &http.Client{Timeout: config.RequestTimeout},
	}
}

func (runner *runner) Run(ctx context.Context) error {
	log.Printf("phase=prepare clients=%d ensure_accounts=%t", runner.config.Clients, runner.config.EnsureAccounts)
	accounts, err := runner.prepareAccounts(ctx)
	if err != nil {
		return err
	}

	log.Printf("phase=connect clients=%d", runner.config.Clients)
	clients, err := runner.connectClients(ctx, accounts)
	if err != nil {
		closeClients(clients)
		return err
	}
	defer closeClients(clients)

	log.Printf("phase=squad_setup squads=%d", runner.config.Clients/runner.config.SquadSize)
	groups, err := runner.setupSquads(ctx, clients)
	if err != nil {
		return err
	}

	if err := waitForDuration(ctx, runner.config.WarmupDuration, "warmup"); err != nil {
		return err
	}

	log.Printf("phase=echo clients=%d rounds_per_client=%d", runner.config.Clients, runner.config.EchoRounds)
	if err := runner.runEcho(ctx, clients); err != nil {
		return err
	}

	if err := waitForDuration(ctx, runner.config.HoldDuration, "hold"); err != nil {
		return err
	}

	log.Printf("phase=cleanup squads=%d", len(groups))
	return runner.cleanupSquads(ctx, groups)
}

func (runner *runner) prepareAccounts(ctx context.Context) ([]account, error) {
	accounts := make([]account, runner.config.Clients)
	workerLimit := runner.config.Clients
	if workerLimit > 8 {
		workerLimit = 8
	}
	semaphore := make(chan struct{}, workerLimit)
	errorsByIndex := make([]error, runner.config.Clients)
	var waitGroup sync.WaitGroup

	for index := 0; index < runner.config.Clients; index++ {
		waitGroup.Add(1)
		go func(accountIndex int) {
			defer waitGroup.Done()
			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-ctx.Done():
				errorsByIndex[accountIndex] = ctx.Err()
				return
			}

			preparedAccount, err := ensureAccount(ctx, runner.httpClient, runner.config, accountIndex, runner.measurements)
			accounts[accountIndex] = preparedAccount
			errorsByIndex[accountIndex] = err
		}(index)
	}
	waitGroup.Wait()
	return accounts, joinIndexedErrors("prepare account", errorsByIndex)
}

func (runner *runner) connectClients(ctx context.Context, accounts []account) ([]*botClient, error) {
	clients := make([]*botClient, len(accounts))
	errorsByIndex := make([]error, len(accounts))
	var waitGroup sync.WaitGroup

	for index := range accounts {
		waitGroup.Add(1)
		go func(clientIndex int) {
			defer waitGroup.Done()
			client, err := connectBot(ctx, runner.config, accounts[clientIndex], runner.measurements)
			clients[clientIndex] = client
			errorsByIndex[clientIndex] = err
		}(index)
	}
	waitGroup.Wait()
	return clients, joinIndexedErrors("connect client", errorsByIndex)
}

func (runner *runner) setupSquads(ctx context.Context, clients []*botClient) ([]squadGroup, error) {
	groupCount := len(clients) / runner.config.SquadSize
	groups := make([]squadGroup, groupCount)
	errorsByIndex := make([]error, groupCount)
	var waitGroup sync.WaitGroup

	for groupIndex := 0; groupIndex < groupCount; groupIndex++ {
		start := groupIndex * runner.config.SquadSize
		end := start + runner.config.SquadSize
		groups[groupIndex].Clients = clients[start:end]

		waitGroup.Add(1)
		go func(currentGroup int) {
			defer waitGroup.Done()
			errorsByIndex[currentGroup] = runner.setupSquad(ctx, &groups[currentGroup])
		}(groupIndex)
	}
	waitGroup.Wait()
	return groups, joinIndexedErrors("setup squad", errorsByIndex)
}

func (runner *runner) setupSquad(ctx context.Context, group *squadGroup) error {
	leader := group.Clients[0]
	response, duration, err := leader.Request(ctx, "squad.create", nil)
	if err != nil {
		runner.measurements.RecordFailure("squad_create", err)
		return err
	}

	var created squadData
	if err := json.Unmarshal(response.Data, &created); err != nil || created.Squad == nil || strings.TrimSpace(created.Squad.ID) == "" {
		if err == nil {
			err = fmt.Errorf("squad.create response missing squad id")
		}
		err = withCategory("squad_create_decode", err)
		runner.measurements.RecordFailure("squad_create", err)
		return err
	}
	runner.measurements.RecordSuccess("squad_create", duration)
	group.SquadID = created.Squad.ID

	for memberIndex := 1; memberIndex < len(group.Clients); memberIndex++ {
		member := group.Clients[memberIndex]
		_, duration, err := member.Request(ctx, "squad.join", map[string]any{"squad_id": group.SquadID})
		if err != nil {
			runner.measurements.RecordFailure("squad_join", err)
			return err
		}
		runner.measurements.RecordSuccess("squad_join", duration)

		_, duration, err = member.Request(ctx, "squad.ready", map[string]any{"ready": true})
		if err != nil {
			runner.measurements.RecordFailure("squad_ready", err)
			return err
		}
		runner.measurements.RecordSuccess("squad_ready", duration)
	}
	return nil
}

func (runner *runner) runEcho(ctx context.Context, clients []*botClient) error {
	errorsByIndex := make([]error, len(clients))
	var waitGroup sync.WaitGroup
	for index, client := range clients {
		waitGroup.Add(1)
		go func(clientIndex int, currentClient *botClient) {
			defer waitGroup.Done()
			for round := 1; round <= runner.config.EchoRounds; round++ {
				_, duration, err := currentClient.Request(ctx, "debug.echo", map[string]any{"round": round})
				if err != nil {
					runner.measurements.RecordFailure("debug_echo", err)
					errorsByIndex[clientIndex] = err
					return
				}
				runner.measurements.RecordSuccess("debug_echo", duration)
				if runner.config.EchoInterval > 0 {
					if err := waitForTimer(ctx, runner.config.EchoInterval); err != nil {
						errorsByIndex[clientIndex] = err
						return
					}
				}
			}
		}(index, client)
	}
	waitGroup.Wait()
	return joinIndexedErrors("echo client", errorsByIndex)
}

func (runner *runner) cleanupSquads(ctx context.Context, groups []squadGroup) error {
	errorsByIndex := make([]error, len(groups))
	var waitGroup sync.WaitGroup
	for groupIndex := range groups {
		waitGroup.Add(1)
		go func(currentGroup int) {
			defer waitGroup.Done()
			group := groups[currentGroup]
			for memberIndex := len(group.Clients) - 1; memberIndex >= 0; memberIndex-- {
				_, duration, err := group.Clients[memberIndex].Request(ctx, "squad.leave", nil)
				if err != nil {
					runner.measurements.RecordFailure("squad_leave", err)
					errorsByIndex[currentGroup] = err
					return
				}
				runner.measurements.RecordSuccess("squad_leave", duration)
			}
		}(groupIndex)
	}
	waitGroup.Wait()
	return joinIndexedErrors("cleanup squad", errorsByIndex)
}

func closeClients(clients []*botClient) {
	for _, client := range clients {
		if client != nil {
			client.Close()
		}
	}
}

func joinIndexedErrors(action string, errorsByIndex []error) error {
	joined := make([]error, 0)
	for index, err := range errorsByIndex {
		if err != nil {
			joined = append(joined, fmt.Errorf("%s %d: %w", action, index+1, err))
		}
	}
	return errors.Join(joined...)
}

func waitForDuration(ctx context.Context, duration time.Duration, phase string) error {
	if duration <= 0 {
		return nil
	}
	log.Printf("phase=%s duration=%s", phase, duration)
	return waitForTimer(ctx, duration)
}

func waitForTimer(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
