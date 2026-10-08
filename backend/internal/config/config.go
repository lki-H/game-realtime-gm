package config

import (
	"os"
	"strconv"
)

type Config struct {
	AppPort      string
	GameplayMode string
	Pprof        PprofConfig
	Database     DatabaseConfig
	Redis        RedisConfig
	JWTSecret    string
	PVE          PVEConfig
	HTTP         HTTPConfig
}

type HTTPConfig struct {
	MaxBodyBytes              int64
	AuthRateLimit             int
	WebSocketRateLimit        int
	WebSocketCommandRateLimit int
	WebSocketMaxConnections   int
	WebSocketMaxPerIP         int
}

type DatabaseConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	Name     string
	UTC      bool
}

type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

type PprofConfig struct {
	Enabled bool
	Addr    string
}

type PVEConfig struct {
	RegroupTTLSeconds    int
	MaxProposalRounds    int
	MessageRetentionDays int
	RulesPath            string
	TestEventsEnabled    bool
	TestEventsAddr       string
	TestEventsToken      string
	ArchiveEnabled       bool
	ArchiveDays          int
	ArchiveBatchSize     int
	MetricsEnabled       bool
	MetricsAddr          string
	MetricsToken         string
}

func Load() Config {
	mode := getEnv("GAMEPLAY_MODE", "v2")
	return Config{
		AppPort:      getEnv("APP_PORT", "8080"),
		GameplayMode: mode,
		Pprof: PprofConfig{
			Enabled: getEnvBool("PPROF_ENABLED", false),
			Addr:    getEnv("PPROF_ADDR", "127.0.0.1:6060"),
		},
		Database: DatabaseConfig{
			UTC:      mode == "v2",
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnv("DB_PORT", "3306"),
			User:     getEnv("DB_USER", "game"),
			Password: getEnv("DB_PASSWORD", "game123456"),
			Name:     getEnv("DB_NAME", "game_realtime"),
		},
		Redis: RedisConfig{
			Addr:     getEnv("REDIS_ADDR", "localhost:6379"),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getEnvInt("REDIS_DB", 0),
		},
		JWTSecret: getEnv("JWT_SECRET", "game-realtime-dev-secret"),
		PVE: PVEConfig{
			RegroupTTLSeconds:    getEnvInt("PVE_REGROUP_TTL_SECONDS", 120),
			MaxProposalRounds:    getEnvInt("PVE_MAX_PROPOSAL_ROUNDS", 8),
			MessageRetentionDays: getEnvInt("PVE_MESSAGE_RETENTION_DAYS", 90),
			RulesPath:            getEnv("PVE_RULES_PATH", ""),
			TestEventsEnabled:    getEnvBool("PVE_TEST_EVENTS_ENABLED", false),
			TestEventsAddr:       getEnv("PVE_TEST_EVENTS_ADDR", "127.0.0.1:8090"),
			TestEventsToken:      getEnv("PVE_TEST_EVENTS_TOKEN", ""),
			ArchiveEnabled:       getEnvBool("PVE_ARCHIVE_ENABLED", false),
			ArchiveDays:          getEnvInt("PVE_ARCHIVE_RETENTION_DAYS", 90),
			ArchiveBatchSize:     getEnvInt("PVE_ARCHIVE_BATCH_SIZE", 100),
			MetricsEnabled:       getEnvBool("PVE_METRICS_ENABLED", false),
			MetricsAddr:          getEnv("PVE_METRICS_ADDR", "127.0.0.1:8091"),
			MetricsToken:         getEnv("PVE_METRICS_TOKEN", ""),
		},
		HTTP: HTTPConfig{MaxBodyBytes: int64(getEnvInt("HTTP_MAX_BODY_BYTES", 1<<20)), AuthRateLimit: getEnvInt("AUTH_RATE_LIMIT_PER_MINUTE", 20), WebSocketRateLimit: getEnvInt("WS_RATE_LIMIT_PER_MINUTE", 30), WebSocketCommandRateLimit: getEnvInt("WS_COMMAND_RATE_LIMIT_PER_MINUTE", 120), WebSocketMaxConnections: getEnvInt("WS_MAX_CONNECTIONS", 256), WebSocketMaxPerIP: getEnvInt("WS_MAX_CONNECTIONS_PER_IP", 64)},
	}
}

func getEnv(key string, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

func getEnvInt(key string, defaultValue int) int {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}

	parsedValue, err := strconv.Atoi(value)
	if err != nil {
		return defaultValue
	}
	return parsedValue
}

func getEnvBool(key string, defaultValue bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}

	parsedValue, err := strconv.ParseBool(value)
	if err != nil {
		return defaultValue
	}
	return parsedValue
}
