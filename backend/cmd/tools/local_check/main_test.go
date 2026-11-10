package main

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

func TestReadOnlyCheckerRejectsUnknownModeAndMismatchedIdentityBeforeConnecting(t *testing.T) {
	if err := check("initialize", "game_realtime_local", "unused"); err == nil {
		t.Fatal("read-only tool accepted a writing mode")
	}
	t.Setenv("DB_HOST", "127.0.0.1")
	t.Setenv("DB_PORT", "33306")
	t.Setenv("DB_USER", "gm_local")
	t.Setenv("DB_NAME", "another_database")
	t.Setenv("DB_PASSWORD", "fixture-value-not-a-secret")
	if err := check("schema", "game_realtime_local", "unused"); err == nil || err.Error() != "configured database identity differs from the expected database" {
		t.Fatal("mismatched identity was not rejected before a connection")
	}
}

type schemaDriver struct{ defect string }
type schemaConnection struct{ defect string }
type schemaTransaction struct{}
type schemaRows struct {
	values []driver.Value
	done   bool
}

var schemaSequence atomic.Int64

func (fixture schemaDriver) Open(string) (driver.Conn, error) {
	return &schemaConnection{defect: fixture.defect}, nil
}
func (*schemaConnection) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("no prepared or writing statements in read-only fixture")
}
func (*schemaConnection) Close() error              { return nil }
func (*schemaConnection) Begin() (driver.Tx, error) { return schemaTransaction{}, nil }
func (schemaTransaction) Commit() error             { return nil }
func (schemaTransaction) Rollback() error           { return nil }
func (*schemaConnection) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	return schemaTransaction{}, nil
}
func (connection *schemaConnection) QueryContext(_ context.Context, query string, arguments []driver.NamedValue) (driver.Rows, error) {
	if !strings.HasPrefix(query, "SELECT ") {
		return nil, errors.New("checker attempted a non-SELECT statement")
	}
	name := func(index int) string { return arguments[index].Value.(string) }
	if strings.Contains(query, "information_schema.tables") {
		count := int64(1)
		if connection.defect == "missing_table" {
			count = 0
		}
		return &schemaRows{values: []driver.Value{count}}, nil
	}
	if strings.Contains(query, "information_schema.columns") {
		kind, nullable := "bigint", "YES"
		if name(0) == "pve_match_lanes" {
			kind, nullable = "varchar(128)", "NO"
		} else if name(0) == "pve_session_revocations" {
			nullable = "NO"
		} else if name(1) == "mission_instance_id" || name(1) == "pve_run_id" {
			kind = "varchar(64)"
		}
		if connection.defect == "missing_"+name(1) {
			return &schemaRows{done: true}, nil
		}
		if connection.defect == "nonnull_legacy_column" && name(1) == "mission_record_id" {
			nullable = "NO"
		}
		return &schemaRows{values: []driver.Value{kind, nullable}}, nil
	}
	if strings.Contains(query, "information_schema.statistics") {
		column := "id"
		if name(0) == "pve_session_revocations" {
			column = "player_id"
		} else if name(0) == "asset_ledger" {
			column = "pve_grant_id"
		}
		if connection.defect == "missing_"+name(0)+"_key" {
			column = ""
		}
		nonUnique := int64(0)
		if connection.defect == "nonunique_reward" && name(0) == "asset_ledger" {
			nonUnique = 1
		}
		return &schemaRows{values: []driver.Value{column, nonUnique}}, nil
	}
	return nil, errors.New("unexpected metadata read")
}
func (rows *schemaRows) Columns() []string {
	columns := make([]string, len(rows.values))
	for index := range columns {
		columns[index] = "fixture"
	}
	return columns
}
func (*schemaRows) Close() error { return nil }
func (rows *schemaRows) Next(values []driver.Value) error {
	if rows.done {
		return io.EOF
	}
	copy(values, rows.values)
	rows.done = true
	return nil
}

func metadataTransaction(t *testing.T, defect string) *sql.Tx {
	t.Helper()
	name := "local-schema-fixture-" + strconv.FormatInt(schemaSequence.Add(1), 10)
	sql.Register(name, schemaDriver{defect: defect})
	db, err := sql.Open(name, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	tx, err := db.BeginTx(context.Background(), &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback() })
	return tx
}

func TestReadOnlyCheckerRejectsMissingInlineTablesAndFoundationConstraints(t *testing.T) {
	if err := inspectFoundation(context.Background(), metadataTransaction(t, "")); err != nil {
		t.Fatal("complete foundation was rejected", err)
	}
	for _, defect := range []string{"missing_pve_grant_id", "missing_pve_run_id", "missing_id", "missing_player_id", "nonnull_legacy_column", "missing_pve_match_lanes_key", "missing_pve_session_revocations_key", "missing_asset_ledger_key", "nonunique_reward"} {
		t.Run(defect, func(t *testing.T) {
			if err := inspectFoundation(context.Background(), metadataTransaction(t, defect)); err == nil {
				t.Fatal("invalid foundation constraint accepted")
			}
		})
	}
	const inlineTable = "CREATE TABLE IF NOT EXISTS pve_match_lanes (id VARCHAR(128) NOT NULL PRIMARY KEY) ENGINE=InnoDB;"
	if err := inspectTables(context.Background(), metadataTransaction(t, "missing_table"), inlineTable); err == nil {
		t.Fatal("inline missing table was hidden by a valid migration ledger")
	}
}
