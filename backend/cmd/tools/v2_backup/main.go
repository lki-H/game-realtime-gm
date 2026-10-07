package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

var identifier = regexp.MustCompile(`^[A-Za-z0-9_]+$`)
var containerIdentifier = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]*$`)

func main() {
	action := flag.String("action", "backup", "backup or restore")
	container := flag.String("container", "gm-r1-test-mysql-1", "MySQL container")
	database := flag.String("database", "", "source database for backup; required new isolated database for restore/check")
	file := flag.String("file", "", "backup file")
	flag.Parse()
	if os.Getenv("V2_BACKUP_CONFIRM") != "I_UNDERSTAND_V2_BACKUP" {
		fatal(errors.New("set V2_BACKUP_CONFIRM=I_UNDERSTAND_V2_BACKUP"))
	}
	if !containerIdentifier.MatchString(*container) || (*database != "" && !identifier.MatchString(*database)) || (*action != "backup" && *database == "") {
		fatal(errors.New("container and database must be simple identifiers"))
	}
	if *file == "" && *action != "check" {
		fatal(errors.New("-file is required"))
	}
	switch *action {
	case "backup":
		backup(*container, *database, *file)
	case "restore":
		restore(*container, *database, *file)
	case "check":
		check(*container, *database)
	default:
		fatal(errors.New("-action must be backup or restore"))
	}
}

func backup(container, database, file string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	source := `"$MYSQL_DATABASE"`
	if database != "" {
		source = database
	}
	command := exec.CommandContext(ctx, "docker", "exec", container, "sh", "-lc", `MYSQL_PWD="$MYSQL_PASSWORD" mysqldump -u"$MYSQL_USER" --single-transaction --routines --triggers --no-tablespaces --set-gtid-purged=OFF `+source)
	output, err := command.Output()
	if err != nil {
		fatal(err)
	}
	if len(output) == 0 {
		fatal(errors.New("backup output is empty"))
	}
	backupFile, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		fatal(err)
	}
	if _, err := backupFile.Write(output); err != nil {
		backupFile.Close()
		fatal(err)
	}
	if err := backupFile.Close(); err != nil {
		fatal(err)
	}
	fmt.Printf("backup written: %s (%d bytes) sha256=%x\n", file, len(output), sha256.Sum256(output))
}

func restore(container, database, file string) {
	if !strings.HasPrefix(database, "game_realtime_v2_") || !strings.HasSuffix(database, "_restore") {
		fatal(errors.New("restore requires a new game_realtime_v2_*_restore database"))
	}
	input, err := os.Open(file)
	if err != nil {
		fatal(err)
	}
	defer input.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	create := exec.CommandContext(ctx, "docker", "exec", container, "sh", "-lc", fmt.Sprintf(`MYSQL_PWD="$MYSQL_ROOT_PASSWORD" mysql -uroot -e "CREATE DATABASE %s; GRANT ALL ON %s.* TO '$MYSQL_USER'@'%%'"`, database, database))
	create.Stdout, create.Stderr = os.Stdout, os.Stderr
	if err := create.Run(); err != nil {
		fatal(err)
	}
	restore := exec.CommandContext(ctx, "docker", "exec", "-i", container, "sh", "-lc", fmt.Sprintf(`MYSQL_PWD="$MYSQL_PASSWORD" mysql -u"$MYSQL_USER" %s`, database))
	restore.Stdin, restore.Stdout, restore.Stderr = input, os.Stdout, os.Stderr
	if err := restore.Run(); err != nil {
		fatal(err)
	}
	fmt.Printf("backup restored: %s\n", database)
}

func check(container, database string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, "docker", "exec", container, "sh", "-lc", fmt.Sprintf(`MYSQL_PWD="$MYSQL_PASSWORD" mysql -N -B -u"$MYSQL_USER" %s -e "SELECT COUNT(*) FROM players; SELECT COUNT(*) FROM pve_runs; SELECT COUNT(*) FROM pve_reward_grants; SELECT COUNT(*) FROM asset_ledger WHERE pve_run_id IS NOT NULL"`, database))
	output, err := command.Output()
	if err != nil {
		fatal(err)
	}
	fmt.Printf("restore_counts:\n%s", output)
}

func fatal(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
