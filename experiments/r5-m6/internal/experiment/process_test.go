package experiment

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestIntegrationConsumerProcessAfterCommitRecovery(t *testing.T) {
	fixture := integration(t)
	binary := os.Getenv("R5_PROCESS_BINARY")
	if binary == "" {
		t.Fatal("set R5_PROCESS_BINARY to the built native role executable for real integration")
	}
	cfg, err := fixture.cfg.ForRole("consumer")
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	cfg.MetricsAddress = listener.Addr().String()
	listener.Close()
	path := filepath.Join(t.TempDir(), "consumer.json")
	data, _ := json.Marshal(cfg)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	start := func(fault string) (*exec.Cmd, <-chan error) {
		t.Helper()
		command := exec.Command(binary, "-mode", "consumer", "-config", path)
		for _, entry := range os.Environ() {
			if !strings.HasPrefix(entry, "R5_ACCEPTANCE_FAULT=") {
				command.Env = append(command.Env, entry)
			}
		}
		command.Env = append(command.Env, "R5_ACCEPTANCE_FAULT="+fault)
		log, err := os.OpenFile(filepath.Join(t.TempDir(), "role.log"), os.O_CREATE|os.O_WRONLY, 0600)
		if err != nil {
			t.Fatal(err)
		}
		command.Stdout, command.Stderr = log, log
		if err := command.Start(); err != nil {
			log.Close()
			t.Fatal("role process failed to start")
		}
		done := make(chan error, 1)
		go func() { done <- command.Wait(); log.Close() }()
		t.Cleanup(func() { _ = command.Process.Kill() })
		return command, done
	}
	envelope := fixture.envelope("actual_process_commit_crash")
	fixture.publish(t, envelope)
	first, done := start("after_commit")
	select {
	case err := <-done:
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() != 42 {
			t.Fatal("consumer did not exit at the guarded commit/ACK window")
		}
	case <-time.After(20 * time.Second):
		_ = first.Process.Kill()
		t.Fatal("consumer fault did not occur before deadline")
	}
	data, err = os.ReadFile(filepath.Join(filepath.Dir(os.Getenv("R5_CONFIG")), "clients.json"))
	if err != nil {
		t.Fatal(err)
	}
	var tokens map[string]string
	if json.Unmarshal(data, &tokens) != nil {
		t.Fatal("client identity fixture invalid")
	}
	second, secondDone := start("")
	defer func() {
		_ = second.Process.Kill()
		select {
		case <-secondDone:
		case <-time.After(5 * time.Second):
			t.Error("consumer cleanup exceeded deadline")
		}
	}()
	client := &http.Client{Timeout: time.Second}
	deadline := time.Now().Add(20 * time.Second)
	duplicate := false
	for time.Now().Before(deadline) {
		request, _ := http.NewRequest("GET", "http://"+cfg.MetricsAddress+"/metrics", nil)
		request.Header.Set("X-Service-ID", "r5-observer")
		request.Header.Set("Authorization", "Bearer "+tokens["r5-observer"])
		response, err := client.Do(request)
		if err == nil {
			body, _ := io.ReadAll(io.LimitReader(response.Body, 8192))
			response.Body.Close()
			if response.StatusCode == 200 && strings.Contains(string(body), "r5_duplicates_total 1\n") {
				duplicate = true
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !duplicate {
		t.Fatal("restarted process did not acknowledge the committed duplicate")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var count int
	var hash string
	if err := fixture.projection.DB.QueryRowContext(ctx, "SELECT COUNT(*),MIN(payload_hash) FROM r5_report_runs").Scan(&count, &hash); err != nil || count != 1 || hash != envelope.PayloadHash {
		t.Fatal("process crash recovery changed report content or count")
	}
	var status string
	if err := fixture.projection.DB.QueryRowContext(ctx, "SELECT status FROM r5_receipts WHERE message_id=?", envelope.MessageID).Scan(&status); err != nil || status != "applied" {
		t.Fatal("process crash lost committed receipt")
	}
}
