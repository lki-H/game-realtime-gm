package experiment

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"regexp"
	"strings"
	"time"
)

var identifier = regexp.MustCompile(`^[A-Za-z0-9_:-]{1,64}$`)

type DatabaseConfig struct {
	Address  string `json:"address"`
	Name     string `json:"name"`
	User     string `json:"user"`
	Password string `json:"password"`
}
type BrokerConfig struct {
	Address  string `json:"address"`
	VHost    string `json:"vhost"`
	User     string `json:"user"`
	Password string `json:"password"`
}
type Identity struct {
	Name      string   `json:"name"`
	TokenHash string   `json:"token_hash"`
	Scopes    []string `json:"scopes"`
	Players   []int64  `json:"players"`
}
type Config struct {
	Source        DatabaseConfig    `json:"source_db"`
	Projection    DatabaseConfig    `json:"projection_db"`
	Publisher     BrokerConfig      `json:"publisher"`
	Consumer      BrokerConfig      `json:"consumer"`
	SourceName    string            `json:"source_name"`
	RPCAddress    string            `json:"rpc_address"`
	Metrics       map[string]string `json:"metrics"`
	Identities    []Identity        `json:"identities"`
	RetryLimit    int               `json:"retry_limit"`
	RetryDelayMS  int               `json:"retry_delay_ms"`
	MaxAgeSeconds int               `json:"max_age_seconds"`
}

func LoadConfig(path string) (Config, error) {
	var cfg Config
	file, err := os.Open(path)
	if err != nil {
		return cfg, errors.New("configuration cannot be opened")
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, 65537))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&cfg) != nil {
		return cfg, errors.New("configuration is invalid")
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return cfg, errors.New("configuration has trailing data")
	}
	return cfg, cfg.Validate()
}
func Loopback(address string) bool {
	host, port, err := net.SplitHostPort(address)
	return err == nil && port != "" && net.ParseIP(host) != nil && net.ParseIP(host).IsLoopback()
}
func (cfg Config) Validate() error {
	if !identifier.MatchString(cfg.SourceName) || !Loopback(cfg.RPCAddress) || cfg.RetryLimit < 1 || cfg.RetryLimit > 10 || cfg.RetryDelayMS < 100 || cfg.RetryDelayMS > 30000 || cfg.MaxAgeSeconds < 60 || cfg.MaxAgeSeconds > 604800 {
		return errors.New("invalid experiment limits or listener")
	}
	if cfg.Source.Name != "game_realtime_v2_r5_restore" || cfg.Projection.Name != "game_realtime_r5_reports" {
		return errors.New("isolated experiment databases required")
	}
	if cfg.Source.User != "r5_reader" || cfg.Projection.User != "r5_projector" || cfg.Publisher.User != "r5_publisher" || cfg.Consumer.User != "r5_consumer" {
		return errors.New("least-privilege experiment accounts required")
	}
	for _, database := range []DatabaseConfig{cfg.Source, cfg.Projection} {
		if !Loopback(database.Address) || database.User == "" || len(database.Password) < 24 {
			return errors.New("invalid database configuration")
		}
	}
	for _, broker := range []BrokerConfig{cfg.Publisher, cfg.Consumer} {
		if !Loopback(broker.Address) || broker.VHost != "r5" || broker.User == "" || len(broker.Password) < 24 {
			return errors.New("isolated broker credentials required")
		}
	}
	for _, role := range []string{"rpc", "bridge", "publisher", "consumer"} {
		if !Loopback(cfg.Metrics[role]) {
			return errors.New("loopback metrics listeners required")
		}
	}
	if len(cfg.Identities) == 0 || len(cfg.Identities) > 20 {
		return errors.New("service identities required")
	}
	names := map[string]bool{}
	for _, identity := range cfg.Identities {
		if !identifier.MatchString(identity.Name) || names[identity.Name] || len(identity.TokenHash) != 64 || strings.Trim(identity.TokenHash, "0123456789abcdef") != "" || len(identity.Players) > 100 {
			return errors.New("invalid service identity")
		}
		names[identity.Name] = true
		for _, scope := range identity.Scopes {
			if scope != "leaderboard" && scope != "result" && scope != "metrics" {
				return errors.New("invalid service scope")
			}
		}
		for _, player := range identity.Players {
			if player <= 0 {
				return errors.New("invalid service player scope")
			}
		}
	}
	return nil
}
func (cfg Config) MaxAge() time.Duration { return time.Duration(cfg.MaxAgeSeconds) * time.Second }
