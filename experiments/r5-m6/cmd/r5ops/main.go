package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"game-realtime-gm/experiments/r5-m6/internal/experiment"
)

func repositoryRoot() string {
	directory, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		if _, err := os.Stat(filepath.Join(directory, ".git")); err == nil {
			return directory
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return ""
		}
		directory = parent
	}
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
func run() error {
	mode := flag.String("mode", "inspect", "inspect or retry; retry defaults to dry run")
	config := flag.String("config", "", "private ops.json; reporting database only")
	limit := flag.Int("limit", 20, "inspection rows, between 1 and 100")
	message := flag.String("message", "", "one exact delivery message ID")
	hash := flag.String("expect-hash", "", "expected payload SHA256")
	fingerprint := flag.String("expect-fingerprint", "", "expected complete envelope SHA256")
	reason := flag.String("reason", "", "non-sensitive operator reason")
	execute := flag.Bool("execute", false, "execute one needs_repair retry; attempts and message remain unchanged")
	auditDirectory := flag.String("audit-dir", "", "dedicated absolute directory outside the repository")
	repository := flag.String("repository", repositoryRoot(), "absolute repository root for audit containment")
	flag.Parse()
	if *mode != "inspect" && *mode != "retry" {
		return failure("unknown_operation")
	}
	if *mode == "inspect" && (*execute || *message != "" || *hash != "" || *fingerprint != "" || *reason != "" || *auditDirectory != "") {
		return failure("inspection_rejects_mutation_options")
	}
	cfg, err := experiment.LoadRoleConfig(*config, "ops")
	if err != nil {
		return failure("ops_configuration_rejected")
	}
	database, err := experiment.OpenDatabase(*cfg.Projection)
	if err != nil {
		return failure("projection_configuration_rejected")
	}
	defer database.Close()
	projection := experiment.Projection{DB: database}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if *mode == "inspect" {
		result, err := projection.Inspect(ctx, *limit, cfg.MaxAge())
		if err != nil {
			return failure("inspection_failed")
		}
		if json.NewEncoder(os.Stdout).Encode(result) != nil {
			return failure("output_failed")
		}
		return nil
	}
	var audit *experiment.AuditLog
	if *execute {
		audit, err = experiment.OpenAudit(*auditDirectory, *repository)
		if err != nil {
			return failure("private_audit_unavailable_or_unresolved_review_manifest_and_lock")
		}
		defer audit.Close()
	}
	result, err := projection.ManualRetry(ctx, cfg, experiment.RetryRequest{MessageID: *message, ExpectedHash: *hash, ExpectedFingerprint: *fingerprint, Reason: *reason, Execute: *execute}, audit)
	if err != nil {
		// Fixed error categories avoid leaking dependency errors or credentials.
		if err == experiment.ErrCommitUnknown || err == experiment.ErrAuditUnknown {
			return failure(err.Error())
		}
		return failure("retry_rejected_inspect_state_time_fingerprint_and_audit")
	}
	if json.NewEncoder(os.Stdout).Encode(result) != nil {
		return failure("output_failed_inspect_before_further_action")
	}
	return nil
}
func failure(code string) error { return errors.New(code) }
