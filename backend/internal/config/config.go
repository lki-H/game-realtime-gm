package config

import (
	"os"
	"strconv"
)

type Config struct {
	AppPort   string
	Pprof     PprofConfig
	Database  DatabaseConfig
	Redis     RedisConfig
	JWTSecret string
}

type DatabaseConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	Name     string
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

func Load() Config {
	return Config{
		AppPort: getEnv("APP_PORT", "8080"),
		Pprof: PprofConfig{
			Enabled: getEnvBool("PPROF_ENABLED", false),
			Addr:    getEnv("PPROF_ADDR", "127.0.0.1:6060"),
		},
		Database: DatabaseConfig{
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
