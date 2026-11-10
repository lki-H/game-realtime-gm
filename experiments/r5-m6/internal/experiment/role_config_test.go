package experiment

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func aggregateTestConfig() Config {
	return Config{Source: DatabaseConfig{Address: "127.0.0.1:23306", Name: "game_realtime_v2_r5_restore", User: "r5_reader", Password: strings.Repeat("s", 32)}, Projection: DatabaseConfig{Address: "127.0.0.1:23306", Name: "game_realtime_r5_reports", User: "r5_projector", Password: strings.Repeat("p", 32)}, Publisher: BrokerConfig{Address: "127.0.0.1:25672", VHost: "r5", User: "r5_publisher", Password: strings.Repeat("b", 32)}, Consumer: BrokerConfig{Address: "127.0.0.1:25672", VHost: "r5", User: "r5_consumer", Password: strings.Repeat("c", 32)}, SourceName: "test_source", RPCAddress: "127.0.0.1:18090", Metrics: map[string]string{"rpc": "127.0.0.1:18091", "bridge": "127.0.0.1:18092", "publisher": "127.0.0.1:18093", "consumer": "127.0.0.1:18094"}, Identities: testIdentities(), RetryLimit: 3, RetryDelayMS: 500, MaxAgeSeconds: 86400}
}
func TestRoleConfigurationContainsOnlyRequiredDependencies(t *testing.T) {
	base := aggregateTestConfig()
	for _, role := range []string{"rpc", "bridge", "publisher", "consumer", "ops"} {
		t.Run(role, func(t *testing.T) {
			cfg, err := base.ForRole(role)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), role+".json")
			data, err := json.Marshal(cfg)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			loaded, err := LoadRoleConfig(path, role)
			if err != nil || loaded.Role != role {
				t.Fatal("role configuration could not load")
			}
			if _, err := LoadRoleConfig(path, "wrong_role"); err == nil {
				t.Fatal("role mismatch accepted")
			}
			if role == "rpc" {
				if strings.Contains(string(data), "projection_db") || strings.Contains(string(data), "publisher") || strings.Contains(string(data), "consumer") {
					t.Fatal("RPC received unrelated credentials")
				}
				value := base.Projection
				cfg.Projection = &value
				if cfg.ValidateForRole(role) == nil {
					t.Fatal("RPC accepted projection credentials")
				}
			} else if role != "ops" {
				cfg.Identities = base.Identities
				if cfg.ValidateForRole(role) == nil {
					t.Fatal("worker accepted query/player scope")
				}
			}
		})
	}
	path := filepath.Join(t.TempDir(), "aggregate.json")
	data, _ := json.Marshal(base)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(path); err != nil {
		t.Fatal("acceptance aggregate compatibility lost")
	}
	if _, err := LoadRoleConfig(path, "rpc"); err == nil {
		t.Fatal("runtime accepted aggregate credentials")
	}
}
func TestQueryConfigRejectsCredentialsAndOversizedFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "query.json")
	for _, data := range []string{`{"rpc_address":"127.0.0.1:18090","source_db":{}}`, `{"rpc_address":"0.0.0.0:18090"}`, `{"rpc_address":"127.0.0.1:18090"}` + strings.Repeat(" ", 65537)} {
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadQueryConfig(path); err == nil {
			t.Fatal("invalid query configuration accepted")
		}
	}
	if err := os.WriteFile(path, []byte(`{"rpc_address":"127.0.0.1:18090"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadQueryConfig(path); err != nil {
		t.Fatal(err)
	}
}
