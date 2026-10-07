package database

import (
	"context"
	"database/sql"
	"time"

	"game-realtime-gm/backend/internal/config"

	"github.com/go-sql-driver/mysql"
)

func NewMySQLDB(ctx context.Context, cfg config.DatabaseConfig) (*sql.DB, error) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return nil, err
	}
	driverConfig := mysql.NewConfig()
	driverConfig.User = cfg.User
	driverConfig.Passwd = cfg.Password
	driverConfig.Net = "tcp"
	driverConfig.Addr = cfg.Host + ":" + cfg.Port
	driverConfig.DBName = cfg.Name
	driverConfig.ParseTime = true
	driverConfig.Loc = location
	driverConfig.Collation = "utf8mb4_0900_ai_ci"
	driverConfig.Params = map[string]string{"charset": "utf8mb4"}
	if cfg.UTC {
		driverConfig.Loc = time.UTC
		driverConfig.Params["time_zone"] = "'+00:00'"
	}
	driverConfig.Timeout = 5 * time.Second
	driverConfig.ReadTimeout = 10 * time.Second
	driverConfig.WriteTimeout = 10 * time.Second
	dsn := driverConfig.FormatDSN()

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(5 * time.Minute)

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}
