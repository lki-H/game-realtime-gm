package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"game-realtime-gm/experiments/r5-m6/internal/experiment"
)

type adminConfig struct {
	Root       experiment.DatabaseConfig `json:"root_db"`
	Broker     experiment.BrokerConfig   `json:"broker"`
	Management string                    `json:"management_address"`
}

func randomSecret() string {
	data := make([]byte, 32)
	if _, err := rand.Read(data); err != nil {
		panic("random source unavailable")
	}
	return hex.EncodeToString(data)
}
func main() {
	adminPath := flag.String("admin", "", "private isolated admin configuration")
	output := flag.String("output", "", "new private runtime configuration")
	flag.Parse()
	if err := prepare(*adminPath, *output); err != nil {
		fmt.Fprintln(os.Stderr, "R5 isolated provisioning failed; inspect configuration and prerequisites")
		os.Exit(1)
	}
	fmt.Println("R5 isolated views, least-privilege accounts, durable queues and private service identities prepared")
}
func writePrivate(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.Write(append(data, '\n'))
	return err
}
func prepare(adminPath, output string) error {
	if _, err := os.Stat(output); err == nil {
		return errors.New("output already exists")
	}
	data, err := os.ReadFile(adminPath)
	if err != nil {
		return err
	}
	var admin adminConfig
	if err := json.Unmarshal(data, &admin); err != nil {
		return err
	}
	if admin.Root.Name != "game_realtime_v2_r5_restore" || admin.Root.User != "root" || !experiment.Loopback(admin.Root.Address) || !experiment.Loopback(admin.Management) || !experiment.Loopback(admin.Broker.Address) || admin.Broker.VHost != "r5" {
		return errors.New("isolated provisioning required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	root, err := experiment.OpenDatabase(admin.Root)
	if err != nil {
		return err
	}
	defer root.Close()
	if err := root.PingContext(ctx); err != nil {
		return err
	}
	if _, err := root.ExecContext(ctx, "CREATE DATABASE game_realtime_r5_reports"); err != nil {
		return err
	}
	views, err := os.ReadFile("schema/002_source_views.sql")
	if err != nil {
		return err
	}
	for _, statement := range strings.Split(string(views), ";") {
		if strings.TrimSpace(statement) != "" {
			if _, err := root.ExecContext(ctx, statement); err != nil {
				return err
			}
		}
	}
	reportAdmin := admin.Root
	reportAdmin.Name = "game_realtime_r5_reports"
	reports, err := experiment.OpenDatabase(reportAdmin)
	if err != nil {
		return err
	}
	defer reports.Close()
	schema, err := os.ReadFile("schema/001_reporting.sql")
	if err != nil {
		return err
	}
	for _, statement := range strings.Split(string(schema), ";") {
		if strings.TrimSpace(statement) != "" {
			if _, err := reports.ExecContext(ctx, statement); err != nil {
				return err
			}
		}
	}
	readerPassword, writerPassword := randomSecret(), randomSecret()
	if _, err := root.ExecContext(ctx, "CREATE USER 'r5_reader'@'%' IDENTIFIED BY '"+readerPassword+"'"); err != nil {
		return err
	}
	if _, err := root.ExecContext(ctx, "CREATE USER 'r5_projector'@'%' IDENTIFIED BY '"+writerPassword+"'"); err != nil {
		return err
	}
	for _, view := range []string{"r5_runs", "r5_results", "r5_outbox", "r5_leaderboard"} {
		if _, err := root.ExecContext(ctx, "GRANT SELECT ON game_realtime_v2_r5_restore."+view+" TO 'r5_reader'@'%'"); err != nil {
			return err
		}
	}
	if _, err := root.ExecContext(ctx, "GRANT SELECT,INSERT,UPDATE,DELETE ON game_realtime_r5_reports.* TO 'r5_projector'@'%'"); err != nil {
		return err
	}
	players := []int64{}
	rows, err := root.QueryContext(ctx, "SELECT DISTINCT player_id FROM r5_results ORDER BY player_id LIMIT 100")
	if err != nil {
		return err
	}
	for rows.Next() {
		var player int64
		if err := rows.Scan(&player); err != nil {
			rows.Close()
			return err
		}
		players = append(players, player)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if len(players) == 0 {
		return errors.New("settled fixture required")
	}
	readerToken, observerToken, publisherPassword, consumerPassword := randomSecret(), randomSecret(), randomSecret(), randomSecret()
	cfg := experiment.Config{
		Source:     experiment.DatabaseConfig{Address: admin.Root.Address, Name: admin.Root.Name, User: "r5_reader", Password: readerPassword},
		Projection: experiment.DatabaseConfig{Address: admin.Root.Address, Name: "game_realtime_r5_reports", User: "r5_projector", Password: writerPassword},
		Publisher:  experiment.BrokerConfig{Address: admin.Broker.Address, VHost: "r5", User: "r5_publisher", Password: publisherPassword},
		Consumer:   experiment.BrokerConfig{Address: admin.Broker.Address, VHost: "r5", User: "r5_consumer", Password: consumerPassword},
		SourceName: "pve_r5_fixture_v1", RPCAddress: "127.0.0.1:18090", Metrics: map[string]string{"rpc": "127.0.0.1:18091", "bridge": "127.0.0.1:18092", "publisher": "127.0.0.1:18093", "consumer": "127.0.0.1:18094"},
		Identities: []experiment.Identity{{Name: "r5-report-client", TokenHash: experiment.Hash([]byte(readerToken)), Scopes: []string{"leaderboard", "result", "metrics"}, Players: players}, {Name: "r5-observer", TokenHash: experiment.Hash([]byte(observerToken)), Scopes: []string{"metrics"}}},
		RetryLimit: 3, RetryDelayMS: 500, MaxAgeSeconds: 86400,
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{Proxy: nil}}
	put := func(path string, value any) error {
		body, _ := json.Marshal(value)
		request, err := http.NewRequestWithContext(ctx, "PUT", "http://"+admin.Management+"/api/"+path, bytes.NewReader(body))
		if err != nil {
			return err
		}
		request.Header.Set("Content-Type", "application/json")
		request.SetBasicAuth(admin.Broker.User, admin.Broker.Password)
		response, err := client.Do(request)
		if err != nil {
			return errors.New("broker management unavailable")
		}
		response.Body.Close()
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return errors.New("broker provisioning rejected")
		}
		return nil
	}
	for _, step := range []struct {
		path  string
		value any
	}{
		{"vhosts/r5", map[string]any{}},
		{"permissions/r5/" + admin.Broker.User, map[string]string{"configure": ".*", "write": ".*", "read": ".*"}},
		{"users/r5_publisher", map[string]string{"password": publisherPassword, "tags": ""}},
		{"users/r5_consumer", map[string]string{"password": consumerPassword, "tags": ""}},
		{"permissions/r5/r5_publisher", map[string]string{"configure": "^$", "write": "^r5\\.events$", "read": "^$"}},
		{"permissions/r5/r5_consumer", map[string]string{"configure": "^$", "write": "^r5\\.(retry|dead)$", "read": "^r5\\.reports$"}},
	} {
		if err := put(step.path, step.value); err != nil {
			return err
		}
	}
	broker, err := experiment.OpenBroker(admin.Broker)
	if err != nil {
		return err
	}
	defer broker.Close()
	if err := experiment.DeclareTopology(broker.Channel, cfg.RetryDelayMS); err != nil {
		return err
	}
	if err := writePrivate(output, cfg); err != nil {
		return err
	}
	return writePrivate(filepath.Join(filepath.Dir(output), "clients.json"), map[string]string{"r5-report-client": readerToken, "r5-observer": observerToken})
}
