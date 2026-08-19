package database

import (
	"context"
	"database/sql"
	"net/url"
	"time"

	"game-realtime-gm/backend/internal/config"

	_ "github.com/go-sql-driver/mysql"
)

func NewMySQLDB(ctx context.Context, cfg config.DatabaseConfig) (*sql.DB, error) {
	dsn := cfg.User + ":" + cfg.Password + "@tcp(" + cfg.Host + ":" + cfg.Port + ")/" + cfg.Name +
		"?parseTime=true&charset=utf8mb4&collation=utf8mb4_0900_ai_ci&loc=" + url.QueryEscape("Asia/Shanghai")

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}
