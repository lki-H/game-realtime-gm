package experiment

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"
)

type auditEvent struct {
	OperationID string             `json:"operation_id"`
	MessageID   string             `json:"message_id"`
	Phase       string             `json:"phase"`
	At          time.Time          `json:"at"`
	Actor       string             `json:"actor"`
	Request     *RetryRequest      `json:"request,omitempty"`
	State       DeliveryInspection `json:"state"`
}
type AuditLog struct {
	file       *os.File
	lock       *os.File
	lockPath   string
	actor      string
	latest     map[string]string
	operations map[string]string
	uncertain  bool
}

func insidePath(path, root string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

// OpenAudit accepts only a dedicated directory outside the repository. The
// JSONL and MySQL transaction are separate durability domains, not atomic audit.
func OpenAudit(directory, repository string) (*AuditLog, error) {
	if !filepath.IsAbs(directory) || !filepath.IsAbs(repository) {
		return nil, errors.New("absolute_audit_and_repository_paths_required")
	}
	root, err := filepath.EvalSymlinks(repository)
	if err != nil {
		return nil, errors.New("repository_path_unavailable")
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(directory))
	if err != nil {
		return nil, errors.New("audit_parent_directory_required")
	}
	directory = filepath.Join(parent, filepath.Base(directory))
	if insidePath(directory, root) || insidePath(root, directory) {
		return nil, errors.New("audit_must_be_separate_from_repository_and_its_ancestors")
	}
	if err := os.Mkdir(directory, 0700); err != nil && !os.IsExist(err) {
		return nil, errors.New("audit_directory_unavailable")
	}
	directory, err = filepath.EvalSymlinks(directory)
	if err != nil || insidePath(directory, root) || insidePath(root, directory) {
		return nil, errors.New("audit_path_rejected")
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, errors.New("audit_directory_unavailable")
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || (entry.Name() != "manifest.jsonl" && entry.Name() != "manifest.lock") {
			return nil, errors.New("dedicated_audit_directory_required")
		}
	}
	if err := secureAuditPath(directory, true); err != nil {
		return nil, errors.New("private_audit_acl_required")
	}
	lockPath := filepath.Join(directory, "manifest.lock")
	lock, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, errors.New("audit_locked_inspect_recorded_process_before_removing_stale_lock")
	}
	audit := &AuditLog{lock: lock, lockPath: lockPath, latest: map[string]string{}, operations: map[string]string{}}
	failed := true
	defer func() {
		if failed {
			audit.Close()
		}
	}()
	if err := secureAuditPath(lockPath, false); err != nil {
		return nil, errors.New("private_audit_acl_required")
	}
	if err := json.NewEncoder(lock).Encode(map[string]any{"pid": os.Getpid(), "started_at": time.Now().UTC()}); err != nil {
		return nil, errors.New("audit_lock_write_failed")
	}
	if err := lock.Sync(); err != nil {
		return nil, errors.New("audit_lock_sync_failed")
	}
	filePath := filepath.Join(directory, "manifest.jsonl")
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0600)
	if err != nil {
		return nil, errors.New("audit_manifest_unavailable")
	}
	audit.file = file
	if err := secureAuditPath(filePath, false); err != nil {
		return nil, errors.New("private_audit_acl_required")
	}
	info, err := file.Stat()
	if err != nil || info.Size() > 16<<20 {
		return nil, errors.New("audit_manifest_limit_or_stat_failed")
	}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4096), 16384)
	for scanner.Scan() {
		var event auditEvent
		if json.Unmarshal(scanner.Bytes(), &event) != nil || event.OperationID == "" || event.MessageID == "" || (event.Phase != "intent" && event.Phase != "committed" && event.Phase != "rejected" && event.Phase != "unknown") {
			return nil, errors.New("audit_manifest_invalid_manual_review_required")
		}
		if event.Phase == "intent" {
			if audit.operations[event.OperationID] != "" {
				return nil, errors.New("duplicate_audit_operation")
			}
			audit.operations[event.OperationID] = event.MessageID
		} else if audit.operations[event.OperationID] != event.MessageID {
			return nil, errors.New("audit_result_without_matching_intent")
		}
		audit.latest[event.MessageID] = event.Phase
	}
	if scanner.Err() != nil {
		return nil, errors.New("audit_manifest_read_failed")
	}
	if _, err := file.Seek(0, io.SeekEnd); err != nil {
		return nil, errors.New("audit_manifest_seek_failed")
	}
	actor, err := user.Current()
	if err != nil {
		return nil, errors.New("audit_actor_unavailable")
	}
	audit.actor = actor.Username
	failed = false
	return audit, nil
}

func (audit *AuditLog) append(event auditEvent) error {
	event.At, event.Actor = time.Now().UTC(), audit.actor
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	written, err := audit.file.Write(data)
	if err != nil {
		audit.uncertain = true
		return err
	}
	if written != len(data) {
		audit.uncertain = true
		return io.ErrShortWrite
	}
	if err := audit.file.Sync(); err != nil {
		audit.uncertain = true
		return err
	}
	audit.latest[event.MessageID] = event.Phase
	return nil
}
func (audit *AuditLog) Intent(request RetryRequest, before DeliveryInspection) (string, error) {
	if phase := audit.latest[request.MessageID]; phase == "intent" || phase == "unknown" {
		return "", errors.New("unresolved_retry_requires_manual_inspection")
	}
	data := make([]byte, 16)
	if _, err := rand.Read(data); err != nil {
		return "", errors.New("audit_random_source_unavailable")
	}
	id := hex.EncodeToString(data)
	if err := audit.append(auditEvent{OperationID: id, MessageID: request.MessageID, Phase: "intent", Request: &request, State: before}); err != nil {
		return "", err
	}
	audit.operations[id] = request.MessageID
	return id, nil
}
func (audit *AuditLog) Result(id, messageID, phase string, state DeliveryInspection) error {
	if audit.operations[id] != messageID || (phase != "committed" && phase != "rejected" && phase != "unknown") {
		return errors.New("audit_result_rejected")
	}
	return audit.append(auditEvent{OperationID: id, MessageID: messageID, Phase: phase, State: state})
}
func (audit *AuditLog) Close() error {
	var err error
	if audit.file != nil {
		err = audit.file.Close()
	}
	if audit.lock != nil {
		err = errors.Join(err, audit.lock.Close())
		// A write/Sync failure may leave a complete but unconfirmed result line.
		// Preserve the lock so a new invocation cannot trust that line by itself.
		if !audit.uncertain {
			err = errors.Join(err, os.Remove(audit.lockPath))
		}
		audit.lock = nil
	}
	return err
}
