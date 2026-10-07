package v2_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"game-realtime-gm/backend/internal/database"
	"github.com/go-sql-driver/mysql"
)

func TestMigrationLegacyUpgradeAndInterruptedDDL(t *testing.T) {
	if os.Getenv("PVE_INTEGRATION") != "1" {
		t.Skip("set PVE_INTEGRATION=1 for isolated migration tests")
	}
	password := os.Getenv("PVE_TEST_PASSWORD")
	if password == "" {
		t.Fatal("temporary test password required")
	}
	address := "127.0.0.1:23306"
	if os.Getenv("PVE_TEST_NETWORK") == "compose" {
		address = "gm-v2-test-mysql-1:3306"
	}
	configuration := mysql.Config{User: "pve_test", Passwd: password, Net: "tcp", Addr: address, DBName: "game_realtime_v2_migration_test", ParseTime: true, Loc: time.UTC, MultiStatements: true}
	db, err := sql.Open("mysql", configuration.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	var name string
	if err := db.QueryRowContext(ctx, "SELECT DATABASE()").Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != "game_realtime_v2_migration_test" {
		t.Fatal("unsafe migration database")
	}
	baseSchema, err := os.ReadFile("../../internal/database/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	migrationPath := "../../internal/database/migrations/day37_v2_pve_foundation.sql"
	reset := func(t *testing.T) {
		t.Helper()
		rows, err := db.QueryContext(ctx, "SHOW TABLES")
		if err != nil {
			t.Fatal(err)
		}
		tables := []string{}
		for rows.Next() {
			var table string
			if err := rows.Scan(&table); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			tables = append(tables, table)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		rows.Close()
		for _, table := range tables {
			if strings.ContainsAny(table, "`;") {
				t.Fatal("unsafe isolated table")
			}
			if _, err := db.ExecContext(ctx, "DROP TABLE `"+table+"`"); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := db.ExecContext(ctx, string(baseSchema)); err != nil {
			t.Fatal(err)
		}
	}
	t.Run("legacy_rows_balances_and_times_survive", func(t *testing.T) {
		reset(t)
		statements := []string{
			"INSERT INTO players(id,username,password_hash,nickname,banned_reason,created_at) VALUES(1,'migration_fixture','synthetic','legacy','','2026-08-23 10:48:12.123')",
			"INSERT INTO player_assets(player_id,soft_currency) VALUES(1,100)",
			"INSERT INTO mission_records(id,mission_instance_id,mission_id,squad_id,submitted_by_player_id,nonce,idempotency_key,completion_seconds,score) VALUES(1,'legacy_instance','mission_demo','squad',1,'nonce','idem',10,50)",
			"INSERT INTO reward_records(id,mission_record_id,mission_instance_id,player_id,reward_type,amount) VALUES(1,1,'legacy_instance',1,'soft_currency',100)",
			"INSERT INTO asset_ledger(player_id,mission_record_id,reward_record_id,mission_instance_id,asset_type,delta,balance_before,balance_after,reason) VALUES(1,1,1,'legacy_instance','soft_currency',100,0,100,'legacy_reward')",
		}
		for _, statement := range statements {
			if _, err := db.ExecContext(ctx, statement); err != nil {
				t.Fatal(err)
			}
		}
		status, err := database.ApplyV2Foundation(ctx, db, migrationPath)
		if err != nil || status != "applied" {
			t.Fatalf("first status=%s err=%v", status, err)
		}
		status, err = database.ApplyV2Foundation(ctx, db, migrationPath)
		if err != nil || status != "applied" {
			t.Fatalf("repeat status=%s err=%v", status, err)
		}
		var balance int64
		var created string
		if err := db.QueryRowContext(ctx, "SELECT a.soft_currency,DATE_FORMAT(p.created_at,'%Y-%m-%d %H:%i:%s.%f') FROM players p JOIN player_assets a ON a.player_id=p.id WHERE p.id=1").Scan(&balance, &created); err != nil {
			t.Fatal(err)
		}
		if balance != 100 || created != "2026-08-23 10:48:12.123000" {
			t.Fatalf("legacy changed balance=%d time=%s", balance, created)
		}
		var count int
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM asset_ledger l JOIN reward_records r ON r.id=l.reward_record_id JOIN mission_records m ON m.id=l.mission_record_id WHERE l.pve_grant_id IS NULL AND l.pve_run_id IS NULL AND l.balance_after=l.balance_before+l.delta AND r.amount=l.delta").Scan(&count); err != nil || count != 1 {
			t.Fatalf("legacy ledger count=%d err=%v", count, err)
		}
		if _, err := db.ExecContext(ctx, "ALTER TABLE pve_reward_grants DROP INDEX uq_pve_reward_source"); err != nil {
			t.Fatal(err)
		}
		if _, err := database.ApplyV2Foundation(ctx, db, migrationPath); err == nil {
			t.Fatal("schema drift ignored after migration applied")
		}
	})
	t.Run("partial_schema_is_rejected", func(t *testing.T) {
		reset(t)
		if _, err := db.ExecContext(ctx, "ALTER TABLE asset_ledger ADD pve_grant_id BIGINT NULL"); err != nil {
			t.Fatal(err)
		}
		if _, err := database.ApplyV2Foundation(ctx, db, migrationPath); err == nil {
			t.Fatal("partial ledger accepted")
		}
	})
	t.Run("failed_DDL_stays_dirty", func(t *testing.T) {
		reset(t)
		migration, err := os.ReadFile(migrationPath)
		if err != nil {
			t.Fatal(err)
		}
		broken := filepath.Join(t.TempDir(), "broken.sql")
		if err := os.WriteFile(broken, append(migration, []byte("\nTHIS IS NOT SQL;\n")...), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := database.ApplyV2Foundation(ctx, db, broken); err == nil {
			t.Fatal("broken DDL accepted")
		}
		var status string
		if err := db.QueryRowContext(ctx, "SELECT status FROM schema_migrations").Scan(&status); err != nil || status != "failed" {
			t.Fatalf("dirty status=%s err=%v", status, err)
		}
		if _, err := database.ApplyV2Foundation(ctx, db, broken); err == nil {
			t.Fatal("dirty migration rerun")
		}
	})
	reset(t)
}
