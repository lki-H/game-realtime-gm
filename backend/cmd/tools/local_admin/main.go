// local_admin creates a new operator only inside the launcher-owned database.
// Credentials are read from a bounded file, never accepted on the command line.
package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"

	"game-realtime-gm/backend/internal/config"
	"game-realtime-gm/backend/internal/database"

	"github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/bcrypt"
)

var usernamePattern = regexp.MustCompile(`^[a-zA-Z0-9_][a-zA-Z0-9_.-]{2,63}$`)

func main() {
	username := flag.String("username", "", "new administrator username")
	passwordFile := flag.String("password-file", "", "external protected password file")
	flag.Parse()
	if err := create(*username, *passwordFile); err != nil {
		fmt.Fprintln(os.Stderr, "Administrator creation failed:", err)
		os.Exit(1)
	}
	fmt.Println("Created a new operator; existing accounts were not changed.")
}

func password(path string) ([]byte, error) {
	if path == "" {
		return nil, errors.New("an external password file is required")
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 1024 {
		return nil, errors.New("password file must be a bounded regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.New("password file could not be opened")
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 1025))
	if err != nil || len(data) > 1024 {
		return nil, errors.New("password file exceeds its read limit")
	}
	value := []byte(strings.TrimSuffix(strings.TrimSuffix(string(data), "\n"), "\r"))
	if len(value) < 12 || len(value) > 72 || strings.ContainsAny(string(value), "\r\n\x00") {
		return nil, errors.New("administrator password must contain 12 to 72 bytes without newlines")
	}
	return value, nil
}

func create(username, passwordFile string) error {
	if !usernamePattern.MatchString(username) {
		return errors.New("administrator username must contain 3 to 64 permitted characters")
	}
	cfg := config.Load()
	if cfg.Database.Name != "game_realtime_local" || cfg.Database.Host != "127.0.0.1" || os.Getenv("GM_LOCAL_OWNED_ADMIN") != "1" {
		return errors.New("administrator bootstrap is restricted to the launcher-owned local database")
	}
	if err := config.ValidateDatabase(cfg.Database); err != nil {
		return err
	}
	value, err := password(passwordFile)
	if err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword(value, bcrypt.DefaultCost)
	for index := range value {
		value[index] = 0
	}
	if err != nil {
		return errors.New("administrator password could not be hashed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	db, err := database.NewMySQLDB(ctx, cfg.Database)
	if err != nil {
		return errors.New("owned database connection unavailable")
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return errors.New("administrator transaction unavailable")
	}
	defer tx.Rollback()
	var existing int64
	err = tx.QueryRowContext(ctx, "SELECT id FROM admins WHERE username=? LIMIT 1 FOR UPDATE", username).Scan(&existing)
	if err == nil {
		return errors.New("administrator already exists; no account was overwritten")
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return errors.New("administrator identity could not be checked")
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO admins(username,password_hash,display_name,role) VALUES(?,?,?,'operator')", username, string(hash), username); err != nil {
		var duplicate *mysql.MySQLError
		if errors.As(err, &duplicate) && duplicate.Number == 1062 {
			return errors.New("administrator already exists; no account was overwritten")
		}
		return errors.New("administrator insert failed")
	}
	if err := tx.Commit(); err != nil {
		return errors.New("administrator transaction could not commit")
	}
	return nil
}
