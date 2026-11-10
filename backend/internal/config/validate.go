package config

import (
	"errors"
	"net"
	"strconv"
	"strings"
)

// ValidateDatabase is also used by database-only maintenance tools. It does not
// require service identity or connect to storage.
func ValidateDatabase(cfg DatabaseConfig) error {
	if strings.TrimSpace(cfg.Host) == "" || strings.TrimSpace(cfg.User) == "" || strings.TrimSpace(cfg.Name) == "" || cfg.Password == "" {
		return errors.New("DB_HOST, DB_USER, DB_NAME and an explicit DB_PASSWORD are required")
	}
	if !validPort(cfg.Port) {
		return errors.New("DB_PORT must be between 1 and 65535")
	}
	if cfg.MaxOpenConns <= 0 || cfg.MaxIdleConns < 0 || cfg.MaxIdleConns > cfg.MaxOpenConns {
		return errors.New("invalid MySQL connection budget")
	}
	return nil
}

// ValidateService fails before opening a socket or database connection. Legacy
// storage can retain its explicitly supplied credentials; none are generated or
// rotated by configuration loading.
func ValidateService(cfg Config) error {
	if err := ValidateDatabase(cfg.Database); err != nil {
		return err
	}
	if cfg.GameplayMode != "v2" && cfg.GameplayMode != "legacy" {
		return errors.New("GAMEPLAY_MODE must be legacy or v2")
	}
	if net.ParseIP(cfg.AppHost) == nil || !validPort(cfg.AppPort) {
		return errors.New("APP_HOST must be an explicit IP address and APP_PORT a valid port")
	}
	if len(cfg.JWTSecret) < 32 || len(cfg.JWTSecret) > 4096 || strings.TrimSpace(cfg.JWTSecret) != cfg.JWTSecret {
		return errors.New("JWT_SECRET must be an explicitly supplied secret of 32 to 4096 bytes without surrounding whitespace")
	}
	if cfg.JWTSecret == cfg.Database.Password || (cfg.Redis.Password != "" && cfg.JWTSecret == cfg.Redis.Password) {
		return errors.New("JWT_SECRET must be independent of storage passwords")
	}
	if host, port, err := net.SplitHostPort(cfg.Redis.Addr); err != nil || host == "" || !validPort(port) || cfg.Redis.DB < 0 || cfg.Redis.DB > 15 {
		return errors.New("REDIS_ADDR and REDIS_DB are invalid")
	}
	if cfg.Redis.PoolSize <= 0 || cfg.Redis.MaxActiveConns < cfg.Redis.PoolSize || cfg.Redis.PoolTimeoutMS <= 0 {
		return errors.New("invalid Redis connection budget")
	}
	return nil
}

func validPort(value string) bool {
	port, err := strconv.Atoi(value)
	return err == nil && port >= 1 && port <= 65535
}
