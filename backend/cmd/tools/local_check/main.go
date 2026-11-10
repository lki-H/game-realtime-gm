// local_check performs bounded storage reads. It never initializes or reconciles
// schema and is safe to use for an explicitly configured existing installation.
package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"game-realtime-gm/backend/internal/cache"
	"game-realtime-gm/backend/internal/config"
	"game-realtime-gm/backend/internal/database"
)

func main() {
	mode := flag.String("mode", "schema", "schema or empty; both modes only read storage")
	expected := flag.String("expected-database", "", "explicit database identity")
	schemaRoot := flag.String("schema-root", "internal/database", "original schema directory")
	flag.Parse()
	if err := check(*mode, *expected, *schemaRoot); err != nil {
		// Do not expose driver errors, addresses, configuration, or credentials.
		fmt.Fprintln(os.Stderr, "Local storage check failed:", err)
		os.Exit(1)
	}
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"status": "PASS_READ_ONLY_STORAGE_CHECK", "mode": *mode, "schema_writes": false, "redis_writes": false})
}

func check(mode, expected, schemaRoot string) error {
	if (mode != "schema" && mode != "empty") || expected == "" {
		return errors.New("an explicit supported check mode and database name are required")
	}
	cfg := config.Load()
	if err := config.ValidateDatabase(cfg.Database); err != nil {
		return err
	}
	if cfg.Database.Name != expected {
		return errors.New("configured database identity differs from the expected database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	db, err := database.NewMySQLDB(ctx, cfg.Database)
	if err != nil {
		return errors.New("MySQL connection unavailable")
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return errors.New("read-only MySQL transaction unavailable")
	}
	defer tx.Rollback()
	var actual string
	if err := tx.QueryRowContext(ctx, "SELECT DATABASE()").Scan(&actual); err != nil || actual != expected {
		return errors.New("connected database identity mismatch")
	}
	var count int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE()").Scan(&count); err != nil {
		return errors.New("schema metadata could not be read")
	}
	if mode == "empty" {
		if count != 0 {
			return errors.New("initialization requires an entirely empty owned database")
		}
		return nil
	}
	for _, table := range []string{"players", "admins", "player_assets", "asset_ledger"} {
		if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name=?", table).Scan(&count); err != nil || count != 1 {
			return errors.New("required base schema is missing")
		}
	}
	if cfg.GameplayMode == "v2" {
		for _, migration := range []string{"day37_v2_pve_foundation", "day38_v2_product_model", "day39_v2_engineering", "day40_v2_retirement"} {
			data, err := os.ReadFile(filepath.Join(schemaRoot, "migrations", migration+".sql"))
			if err != nil {
				return errors.New("original migration source could not be read")
			}
			digest := sha256.Sum256(data)
			var checksum, status string
			if err := tx.QueryRowContext(ctx, "SELECT checksum,status FROM schema_migrations WHERE migration_id=?", migration).Scan(&checksum, &status); err != nil {
				return errors.New("required migration ledger entry is missing")
			}
			if checksum != hex.EncodeToString(digest[:]) || (status != "applied" && status != "reconciled") {
				return errors.New("migration ledger is dirty or its original SQL checksum differs")
			}
			if err := inspectTables(ctx, tx, string(data)); err != nil {
				return err
			}
		}
		if err := inspectFoundation(ctx, tx); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return errors.New("read-only transaction could not complete")
	}
	redis, err := cache.NewRedisClient(ctx, cfg.Redis)
	if err != nil {
		return errors.New("Redis authenticated PING unavailable")
	}
	defer redis.Close()
	return nil
}

// Check the numbered source's table/column/index shape using SELECT metadata
// queries only. In particular, an applied ledger cannot hide a dropped table.
func inspectTables(ctx context.Context, tx *sql.Tx, script string) error {
	definitions := regexp.MustCompile(`(?is)CREATE TABLE IF NOT EXISTS (pve_[a-z_]+)\s*\((.*?)\)\s*ENGINE`).FindAllStringSubmatch(script, -1)
	if len(definitions) == 0 {
		return errors.New("original migration has no recognizable table definitions")
	}
	for _, definition := range definitions {
		var tableCount int
		if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name=?", definition[1]).Scan(&tableCount); err != nil || tableCount != 1 {
			return errors.New("numbered schema table is missing")
		}
		for _, column := range regexp.MustCompile(`(?m)^\s*([a-z][a-z0-9_]*)\s+([A-Z]+(?:\([0-9,]+\))?)([^\r\n]*)`).FindAllStringSubmatch(definition[2], -1) {
			nullable := "YES"
			if strings.Contains(column[3], "NOT NULL") {
				nullable = "NO"
			}
			var actualType, actualNullable string
			if err := tx.QueryRowContext(ctx, "SELECT column_type,is_nullable FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name=? AND column_name=?", definition[1], column[1]).Scan(&actualType, &actualNullable); err != nil || actualType != strings.ToLower(column[2]) || actualNullable != nullable {
				return errors.New("numbered schema table or column shape is incomplete")
			}
		}
		for _, index := range regexp.MustCompile(`(?i)\b(PRIMARY KEY|UNIQUE KEY\s+([a-z0-9_]+)|KEY\s+([a-z0-9_]+))\s*\(([^)]+)\)`).FindAllStringSubmatch(definition[2], -1) {
			name := "PRIMARY"
			if index[2] != "" {
				name = index[2]
			} else if index[3] != "" {
				name = index[3]
			}
			var columns string
			var nonUnique int
			if err := tx.QueryRowContext(ctx, "SELECT COALESCE(GROUP_CONCAT(column_name ORDER BY seq_in_index),''),COALESCE(MAX(non_unique),0) FROM information_schema.statistics WHERE table_schema=DATABASE() AND table_name=? AND index_name=?", definition[1], name).Scan(&columns, &nonUnique); err != nil || columns != strings.ReplaceAll(index[4], " ", "") || ((name == "PRIMARY" || index[2] != "") && nonUnique != 0) {
				return errors.New("numbered schema index shape is incomplete")
			}
		}
	}
	return nil
}

// These are the inline primary keys and ALTER TABLE changes in the fixed
// day37 foundation. Keep them explicit rather than extending a SQL parser.
func inspectFoundation(ctx context.Context, tx *sql.Tx) error {
	columns := []struct{ table, name, kind, nullable string }{
		{"pve_match_lanes", "id", "varchar(128)", "NO"},
		{"pve_session_revocations", "player_id", "bigint", "NO"},
		{"asset_ledger", "mission_record_id", "bigint", "YES"},
		{"asset_ledger", "reward_record_id", "bigint", "YES"},
		{"asset_ledger", "mission_instance_id", "varchar(64)", "YES"},
		{"asset_ledger", "pve_grant_id", "bigint", "YES"},
		{"asset_ledger", "pve_run_id", "varchar(64)", "YES"},
	}
	for _, column := range columns {
		var kind, nullable string
		if err := tx.QueryRowContext(ctx, "SELECT column_type,is_nullable FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name=? AND column_name=?", column.table, column.name).Scan(&kind, &nullable); err != nil || kind != column.kind || nullable != column.nullable {
			return errors.New("foundation inline key or asset ledger column shape is incomplete")
		}
	}
	for _, index := range []struct{ table, name, column string }{{"pve_match_lanes", "PRIMARY", "id"}, {"pve_session_revocations", "PRIMARY", "player_id"}, {"asset_ledger", "uq_asset_ledger_pve_grant", "pve_grant_id"}} {
		var columns string
		var nonUnique int
		if err := tx.QueryRowContext(ctx, "SELECT COALESCE(GROUP_CONCAT(column_name ORDER BY seq_in_index),''),COALESCE(MAX(non_unique),0) FROM information_schema.statistics WHERE table_schema=DATABASE() AND table_name=? AND index_name=?", index.table, index.name).Scan(&columns, &nonUnique); err != nil || columns != index.column || nonUnique != 0 {
			return errors.New("foundation inline primary key or reward uniqueness index is incomplete")
		}
	}
	return nil
}
