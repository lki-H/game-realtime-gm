package store

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
)

var Invalid = errors.New("invalid request")
var Conflict = errors.New("state or version conflict")
var Forbidden = errors.New("permission denied")
var NotFound = errors.New("resource not found")
var Archived = errors.New("operation receipt archived; recover current activity")

func ID(prefix string) string { return prefix + "_" + rand.Text() }
func JSON(value any) []byte {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var normalized any
	if err := decoder.Decode(&normalized); err != nil {
		panic(err)
	}
	canonical, err := json.Marshal(normalized)
	if err != nil {
		panic(err)
	}
	return canonical
}
func Hash(value []byte) string { digest := sha256.Sum256(value); return hex.EncodeToString(digest[:]) }
func ValidID(value string) bool {
	return len(value) >= 8 && len(value) <= 128 && !strings.ContainsAny(value, " \t\n\r")
}

func Transaction(ctx context.Context, db *sql.DB, execute func(*sql.Tx) error) error {
	for attempt := 0; attempt < 3; attempt++ {
		transaction, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
		if err != nil {
			return err
		}
		err = func() error {
			defer transaction.Rollback()
			if err := execute(transaction); err != nil {
				return err
			}
			return transaction.Commit()
		}()
		if err == nil {
			return nil
		}
		_ = transaction.Rollback()
		var mysqlError *mysql.MySQLError
		if !errors.As(err, &mysqlError) || (mysqlError.Number != 1213 && mysqlError.Number != 1205) {
			if errors.As(err, &mysqlError) && mysqlError.Number == 1062 {
				return Conflict
			}
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(attempt+1) * 10 * time.Millisecond):
		}
	}
	return Conflict
}

func LockPlayers(ctx context.Context, transaction *sql.Tx, players []int64) error {
	ordered := append([]int64(nil), players...)
	sort.Slice(ordered, func(first, second int) bool { return ordered[first] < ordered[second] })
	for _, player := range ordered {
		var status string
		if player <= 0 {
			return Invalid
		}
		err := transaction.QueryRowContext(ctx, "SELECT status FROM players WHERE id=? FOR UPDATE", player).Scan(&status)
		if errors.Is(err, sql.ErrNoRows) {
			return NotFound
		}
		if err != nil {
			return err
		}
		if status != "normal" {
			return Forbidden
		}
	}
	return nil
}

func Command(ctx context.Context, db *sql.DB, player int64, operation, action string, input any, execute func(*sql.Tx) (any, error)) (json.RawMessage, error) {
	if !ValidID(operation) {
		return nil, Invalid
	}
	fingerprint := Hash(JSON(input))
	var output json.RawMessage
	err := Transaction(ctx, db, func(transaction *sql.Tx) error {
		if err := LockPlayers(ctx, transaction, []int64{player}); err != nil {
			return err
		}
		var existingAction, existingHash string
		var result []byte
		err := transaction.QueryRowContext(ctx, "SELECT action,fingerprint,result FROM pve_command_results WHERE player_id=? AND operation_id=?", player, operation).Scan(&existingAction, &existingHash, &result)
		if err == nil {
			if existingAction != action || existingHash != fingerprint {
				return Conflict
			}
			var receipt struct {
				Archived bool `json:"_pve_archived"`
			}
			if json.Unmarshal(result, &receipt) == nil && receipt.Archived {
				return Archived
			}
			output = JSON(json.RawMessage(result))
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		value, err := execute(transaction)
		if err != nil {
			return err
		}
		output = JSON(value)
		_, err = transaction.ExecContext(ctx, "INSERT INTO pve_command_results(player_id,operation_id,action,fingerprint,result) VALUES(?,?,?,?,?)", player, operation, action, fingerprint, output)
		return err
	})
	return output, err
}

func Blocked(ctx context.Context, transaction *sql.Tx, first, second int64) (bool, error) {
	var count int
	err := transaction.QueryRowContext(ctx, "SELECT COUNT(*) FROM pve_social_blocks WHERE (blocker_id=? AND blocked_id=?) OR (blocker_id=? AND blocked_id=?)", first, second, second, first).Scan(&count)
	return count > 0, err
}
func Friends(ctx context.Context, transaction *sql.Tx, first, second int64) (bool, error) {
	if first > second {
		first, second = second, first
	}
	var count int
	err := transaction.QueryRowContext(ctx, "SELECT COUNT(*) FROM pve_social_friendships WHERE player_low_id=? AND player_high_id=? AND status='active'", first, second).Scan(&count)
	return count > 0, err
}
func Notify(ctx context.Context, transaction *sql.Tx, players []int64, event string, value any) error {
	payload := JSON(map[string]any{"players": players, "type": event, "data": value})
	_, err := transaction.ExecContext(ctx, "INSERT INTO pve_outbox_records(operation_id,event_type,aggregate_id,payload) VALUES(?,?,?,?)", ID("notify"), event, "notification", payload)
	return err
}
func RequireIdle(ctx context.Context, transaction *sql.Tx, player int64) error {
	var count int
	err := transaction.QueryRowContext(ctx, "SELECT COUNT(*) FROM pve_player_activity_locks WHERE player_id=?", player).Scan(&count)
	if err != nil {
		return err
	}
	if count > 0 {
		return Conflict
	}
	return nil
}

func RequireNoRecruitment(ctx context.Context, transaction *sql.Tx, player int64) error {
	var reserved int
	if err := transaction.QueryRowContext(ctx, "SELECT COUNT(*) FROM pve_recruitment_preferences WHERE player_id=? AND expires_at>UTC_TIMESTAMP(3)", player).Scan(&reserved); err != nil {
		return err
	}
	if reserved > 0 {
		return Conflict
	}
	return nil
}
