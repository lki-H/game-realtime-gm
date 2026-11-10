package experiment

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"time"
)

// RoleConfig is a private runtime format, independent of the V1 wire contracts.
// A role must not receive credentials for dependencies it does not use.
type RoleConfig struct {
	Role           string          `json:"role"`
	Source         *DatabaseConfig `json:"source_db,omitempty"`
	Projection     *DatabaseConfig `json:"projection_db,omitempty"`
	Publisher      *BrokerConfig   `json:"publisher,omitempty"`
	Consumer       *BrokerConfig   `json:"consumer,omitempty"`
	SourceName     string          `json:"source_name,omitempty"`
	RPCAddress     string          `json:"rpc_address,omitempty"`
	MetricsAddress string          `json:"metrics_address,omitempty"`
	Identities     []Identity      `json:"identities,omitempty"`
	RetryLimit     int             `json:"retry_limit,omitempty"`
	MaxAgeSeconds  int             `json:"max_age_seconds,omitempty"`
}

func readPrivateJSON(path string, target any) error {
	file, err := os.Open(path)
	if err != nil {
		return errors.New("configuration cannot be opened")
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 65537))
	if err != nil || len(data) > 65536 {
		return errors.New("configuration exceeds limit")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(target) != nil {
		return errors.New("configuration is invalid")
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return errors.New("configuration has trailing data")
	}
	return nil
}

func LoadRoleConfig(path, role string) (RoleConfig, error) {
	var cfg RoleConfig
	if err := readPrivateJSON(path, &cfg); err != nil {
		return cfg, err
	}
	return cfg, cfg.ValidateForRole(role)
}

func (cfg RoleConfig) ValidateForRole(role string) error {
	if cfg.Role != role {
		return errors.New("role configuration mismatch; prepare a new isolated runtime")
	}
	needSource, needProjection, needPublisher, needConsumer := false, false, false, false
	needSourceName, needRetry, needAge := false, false, false
	switch role {
	case "rpc":
		needSource = true
	case "bridge":
		needSource, needProjection, needSourceName = true, true, true
	case "publisher":
		needProjection, needPublisher, needRetry = true, true, true
	case "consumer":
		needProjection, needConsumer, needSourceName, needRetry, needAge = true, true, true, true, true
	case "ops":
		needProjection, needSourceName, needAge = true, true, true
	default:
		return errors.New("unknown experiment role")
	}
	if (cfg.Source != nil) != needSource || (cfg.Projection != nil) != needProjection || (cfg.Publisher != nil) != needPublisher || (cfg.Consumer != nil) != needConsumer {
		return errors.New("role dependencies must be exact")
	}
	for _, entry := range []struct {
		value      *DatabaseConfig
		name, user string
	}{
		{cfg.Source, "game_realtime_v2_r5_restore", "r5_reader"},
		{cfg.Projection, "game_realtime_r5_reports", "r5_projector"},
	} {
		if entry.value != nil && (entry.value.Name != entry.name || entry.value.User != entry.user || !Loopback(entry.value.Address) || len(entry.value.Password) < 24) {
			return errors.New("isolated role database required")
		}
	}
	for _, entry := range []struct {
		value *BrokerConfig
		user  string
	}{{cfg.Publisher, "r5_publisher"}, {cfg.Consumer, "r5_consumer"}} {
		if entry.value != nil && (entry.value.User != entry.user || entry.value.VHost != "r5" || !Loopback(entry.value.Address) || len(entry.value.Password) < 24) {
			return errors.New("isolated role broker required")
		}
	}
	if needSourceName != (cfg.SourceName != "") || (needSourceName && !identifier.MatchString(cfg.SourceName)) || needRetry != (cfg.RetryLimit != 0) || (needRetry && (cfg.RetryLimit < 1 || cfg.RetryLimit > 10)) || needAge != (cfg.MaxAgeSeconds != 0) || (needAge && (cfg.MaxAgeSeconds < 60 || cfg.MaxAgeSeconds > 604800)) {
		return errors.New("invalid role limits")
	}
	if role == "ops" {
		if cfg.RPCAddress != "" || cfg.MetricsAddress != "" || len(cfg.Identities) != 0 {
			return errors.New("ops configuration must contain projection credentials only")
		}
		return nil
	}
	if (role == "rpc") != (cfg.RPCAddress != "") || (role == "rpc" && !Loopback(cfg.RPCAddress)) || !Loopback(cfg.MetricsAddress) {
		return errors.New("loopback role listeners required")
	}
	if len(cfg.Identities) == 0 || len(cfg.Identities) > 20 {
		return errors.New("role identities required")
	}
	names := map[string]bool{}
	for _, identity := range cfg.Identities {
		if !identifier.MatchString(identity.Name) || names[identity.Name] || len(identity.TokenHash) != 64 || strings.Trim(identity.TokenHash, "0123456789abcdef") != "" || len(identity.Players) > 100 || len(identity.Scopes) == 0 {
			return errors.New("invalid role identity")
		}
		names[identity.Name] = true
		for _, scope := range identity.Scopes {
			if scope != "metrics" && (role != "rpc" || (scope != "result" && scope != "leaderboard")) {
				return errors.New("role scope rejected")
			}
		}
		if role != "rpc" && len(identity.Players) != 0 {
			return errors.New("worker player scope rejected")
		}
		for _, player := range identity.Players {
			if player <= 0 {
				return errors.New("invalid player scope")
			}
		}
	}
	return nil
}

func (cfg Config) ForRole(role string) (RoleConfig, error) {
	if err := cfg.Validate(); err != nil {
		return RoleConfig{}, err
	}
	result := RoleConfig{Role: role}
	if role == "rpc" || role == "bridge" {
		value := cfg.Source
		result.Source = &value
	}
	if role == "bridge" || role == "publisher" || role == "consumer" || role == "ops" {
		value := cfg.Projection
		result.Projection = &value
	}
	if role == "publisher" {
		value := cfg.Publisher
		result.Publisher = &value
		result.RetryLimit = cfg.RetryLimit
	}
	if role == "consumer" {
		value := cfg.Consumer
		result.Consumer = &value
		result.RetryLimit = cfg.RetryLimit
	}
	if role == "bridge" || role == "consumer" || role == "ops" {
		result.SourceName = cfg.SourceName
	}
	if role == "consumer" || role == "ops" {
		result.MaxAgeSeconds = cfg.MaxAgeSeconds
	}
	if role == "rpc" {
		result.RPCAddress = cfg.RPCAddress
		result.Identities = cfg.Identities
	}
	if role != "ops" {
		result.MetricsAddress = cfg.Metrics[role]
		if role != "rpc" {
			for _, identity := range cfg.Identities {
				if hasScope(identity, "metrics") {
					result.Identities = append(result.Identities, Identity{Name: identity.Name, TokenHash: identity.TokenHash, Scopes: []string{"metrics"}})
				}
			}
		}
	}
	return result, result.ValidateForRole(role)
}

func (cfg RoleConfig) MaxAge() time.Duration { return time.Duration(cfg.MaxAgeSeconds) * time.Second }

type QueryConfig struct {
	RPCAddress string `json:"rpc_address"`
}

func LoadQueryConfig(path string) (QueryConfig, error) {
	var cfg QueryConfig
	if err := readPrivateJSON(path, &cfg); err != nil {
		return cfg, err
	}
	if !Loopback(cfg.RPCAddress) {
		return cfg, errors.New("loopback RPC endpoint required")
	}
	return cfg, nil
}
