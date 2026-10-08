package database

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"
)

const V2FoundationMigration = "day37_v2_pve_foundation"
const V2ProductMigration = "day38_v2_product_model"
const V2EngineeringMigration = "day39_v2_engineering"
const V2RetirementMigration = "day40_v2_retirement"

type MigrationRecord struct {
	ID        string
	Checksum  string
	Status    string
	AppliedAt sql.NullTime
	ErrorText sql.NullString
}

func ApplyV2Foundation(ctx context.Context, db *sql.DB, path string) (string, error) {
	return applyNumberedMigration(ctx, db, path, V2FoundationMigration, inspectV2Foundation)
}
func ApplyV2Products(ctx context.Context, db *sql.DB, path string) (string, error) {
	return applyNumberedMigration(ctx, db, path, V2ProductMigration, inspectV2Tables)
}
func ApplyV2Engineering(ctx context.Context, db *sql.DB, path string) (string, error) {
	return applyNumberedMigration(ctx, db, path, V2EngineeringMigration, inspectV2Tables)
}
func ApplyV2Retirement(ctx context.Context, db *sql.DB, path string) (string, error) {
	return applyNumberedMigration(ctx, db, path, V2RetirementMigration, inspectV2Tables)
}
func applyNumberedMigration(ctx context.Context, db *sql.DB, path, migrationID string, inspect func(context.Context, *sql.Conn, string) (bool, bool, error)) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	checksum := fmt.Sprintf("%x", digest[:])
	connection, err := db.Conn(ctx)
	if err != nil {
		return "", err
	}
	defer connection.Close()
	var locked int
	if err := connection.QueryRowContext(ctx, "SELECT GET_LOCK(CONCAT('gm-gameplay:', DATABASE()), 0)").Scan(&locked); err != nil || locked != 1 {
		return "", fmt.Errorf("database gameplay or migration owner is active")
	}
	defer func() {
		release, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = connection.ExecContext(release, "SELECT RELEASE_LOCK(CONCAT('gm-gameplay:', DATABASE()))")
	}()
	if _, err := connection.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
        migration_id VARCHAR(128) NOT NULL PRIMARY KEY,
        checksum CHAR(64) NOT NULL,
        status VARCHAR(32) NOT NULL,
        started_at DATETIME(3) NOT NULL,
        applied_at DATETIME(3) NULL,
        error_text TEXT NULL
    ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci`); err != nil {
		return "", err
	}
	var record MigrationRecord
	err = connection.QueryRowContext(ctx, `SELECT migration_id, checksum, status, applied_at, error_text
        FROM schema_migrations WHERE migration_id = ?`, migrationID).
		Scan(&record.ID, &record.Checksum, &record.Status, &record.AppliedAt, &record.ErrorText)
	if err == nil {
		if record.Checksum != checksum {
			return "", fmt.Errorf("migration checksum changed: %s", migrationID)
		}
		if record.Status == "applied" || record.Status == "reconciled" {
			complete, _, err := inspect(ctx, connection, string(data))
			if err != nil {
				return "", err
			}
			if !complete {
				return "", fmt.Errorf("applied migration schema is incomplete: %s", record.ID)
			}
			return record.Status, nil
		}
		return "", fmt.Errorf("migration is not clean: %s status=%s", record.ID, record.Status)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	complete, partial, err := inspect(ctx, connection, string(data))
	if err != nil {
		return "", err
	}
	if complete {
		if _, err := connection.ExecContext(ctx, `INSERT INTO schema_migrations
            (migration_id, checksum, status, started_at, applied_at)
            VALUES (?, ?, 'reconciled', ?, ?)`, migrationID, checksum, time.Now().UTC(), time.Now().UTC()); err != nil {
			return "", err
		}
		return "reconciled", nil
	}
	if partial {
		return "", fmt.Errorf("partial V2 migration detected; restore or repair before applying %s", migrationID)
	}
	if _, err := connection.ExecContext(ctx, `INSERT INTO schema_migrations
        (migration_id, checksum, status, started_at)
        VALUES (?, ?, 'running', ?)`, migrationID, checksum, time.Now().UTC()); err != nil {
		return "", err
	}
	statements := splitSQL(string(data))
	for index, statement := range statements {
		if strings.TrimSpace(statement) == "" {
			continue
		}
		if _, err := connection.ExecContext(ctx, statement); err != nil {
			_, _ = connection.ExecContext(ctx, `UPDATE schema_migrations SET status='failed', error_text=? WHERE migration_id=?`, fmt.Sprintf("statement %d failed", index+1), migrationID)
			return "", err
		}
	}
	if _, err := connection.ExecContext(ctx, `UPDATE schema_migrations SET status='applied', applied_at=? WHERE migration_id=?`, time.Now().UTC(), migrationID); err != nil {
		return "", err
	}
	return "applied", nil
}

func inspectV2Foundation(ctx context.Context, connection *sql.Conn, script string) (bool, bool, error) {
	complete, partial, err := inspectV2Tables(ctx, connection, script)
	if err != nil {
		return false, false, err
	}
	var columns, nullable, uniqueGrant int
	if err := connection.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='asset_ledger' AND column_name IN ('pve_grant_id','pve_run_id')").Scan(&columns); err != nil {
		return false, false, err
	}
	if err := connection.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='asset_ledger' AND column_name IN ('mission_record_id','reward_record_id','mission_instance_id') AND is_nullable='YES'").Scan(&nullable); err != nil {
		return false, false, err
	}
	if err := connection.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.statistics WHERE table_schema=DATABASE() AND table_name='asset_ledger' AND index_name='uq_asset_ledger_pve_grant' AND column_name='pve_grant_id' AND non_unique=0").Scan(&uniqueGrant); err != nil {
		return false, false, err
	}
	return complete && columns == 2 && nullable == 3 && uniqueGrant == 1, partial || columns > 0 || nullable > 0, nil
}
func inspectV2Tables(ctx context.Context, connection *sql.Conn, script string) (bool, bool, error) {
	definitions := regexp.MustCompile(`(?is)CREATE TABLE IF NOT EXISTS (pve_[a-z_]+)\s*\((.*?)\)\s*ENGINE`).FindAllStringSubmatch(script, -1)
	if len(definitions) == 0 {
		return false, false, fmt.Errorf("migration contains no V2 table definitions")
	}
	complete, partial := true, false
	for _, definition := range definitions {
		rows, err := connection.QueryContext(ctx, "SELECT column_name,column_type,is_nullable FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name=?", definition[1])
		if err != nil {
			return false, false, err
		}
		type columnShape struct {
			kind     string
			nullable string
		}
		columns := map[string]columnShape{}
		for rows.Next() {
			var column string
			var shape columnShape
			if err := rows.Scan(&column, &shape.kind, &shape.nullable); err != nil {
				rows.Close()
				return false, false, err
			}
			columns[column] = shape
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return false, false, err
		}
		partial = partial || len(columns) > 0
		complete = complete && len(columns) > 0
		for _, column := range regexp.MustCompile(`(?m)^\s*([a-z][a-z0-9_]*)\s+([A-Z]+(?:\([0-9,]+\))?)([^\r\n]*)`).FindAllStringSubmatch(definition[2], -1) {
			expectedNullable := "YES"
			if strings.Contains(column[3], "NOT NULL") {
				expectedNullable = "NO"
			}
			shape, exists := columns[column[1]]
			complete = complete && exists && shape.kind == strings.ToLower(column[2]) && shape.nullable == expectedNullable
		}
		for _, index := range regexp.MustCompile(`(?i)\b(PRIMARY KEY|UNIQUE KEY\s+([a-z0-9_]+)|KEY\s+([a-z0-9_]+))\s*\(([^)]+)\)`).FindAllStringSubmatch(definition[2], -1) {
			name := "PRIMARY"
			if index[2] != "" {
				name = index[2]
			} else if index[3] != "" {
				name = index[3]
			}
			var actualColumns string
			var nonUnique int
			if err := connection.QueryRowContext(ctx, "SELECT COALESCE(GROUP_CONCAT(column_name ORDER BY seq_in_index),''),COALESCE(MAX(non_unique),0) FROM information_schema.statistics WHERE table_schema=DATABASE() AND table_name=? AND index_name=?", definition[1], name).Scan(&actualColumns, &nonUnique); err != nil {
				return false, false, err
			}
			expectedColumns := strings.ReplaceAll(index[4], " ", "")
			complete = complete && actualColumns == expectedColumns
			if name == "PRIMARY" || index[2] != "" {
				complete = complete && nonUnique == 0
			}
		}
	}
	return complete, partial, nil
}

func splitSQL(script string) []string {
	statements := make([]string, 0)
	start := 0
	inSingle, inDouble, inBacktick, inLineComment, inBlockComment := false, false, false, false, false
	for index := 0; index < len(script); index++ {
		character := script[index]
		if inLineComment {
			if character == '\n' {
				inLineComment = false
			}
			continue
		}
		if inBlockComment {
			if character == '*' && index+1 < len(script) && script[index+1] == '/' {
				inBlockComment = false
				index++
			}
			continue
		}
		if !inSingle && !inDouble && !inBacktick && character == '-' && index+2 < len(script) && script[index+1] == '-' && (script[index+2] == ' ' || script[index+2] == '\t') {
			inLineComment = true
			continue
		}
		if !inSingle && !inDouble && !inBacktick && character == '/' && index+1 < len(script) && script[index+1] == '*' {
			inBlockComment = true
			index++
			continue
		}
		switch character {
		case '\'':
			if !inDouble && !inBacktick && (index == 0 || script[index-1] != '\\') {
				inSingle = !inSingle
			}
		case '"':
			if !inSingle && !inBacktick && (index == 0 || script[index-1] != '\\') {
				inDouble = !inDouble
			}
		case '`':
			if !inSingle && !inDouble {
				inBacktick = !inBacktick
			}
		case ';':
			if !inSingle && !inDouble && !inBacktick {
				if strings.TrimSpace(script[start:index]) != "" {
					statements = append(statements, script[start:index])
				}
				start = index + 1
			}
		}
	}
	if strings.TrimSpace(script[start:]) != "" {
		statements = append(statements, script[start:])
	}
	return statements
}
