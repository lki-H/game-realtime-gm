package social

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"game-realtime-gm/backend/internal/pve/store"
)

type Action struct {
	PlayerID  int64  `json:"player_id"`
	RequestID int64  `json:"request_id"`
	Accept    bool   `json:"accept"`
	Note      string `json:"note"`
	Body      string `json:"body"`
	MessageID int64  `json:"message_id"`
}

func (s *Service) Execute(ctx context.Context, transaction *sql.Tx, player int64, kind string, input Action) (any, error) {
	peer := input.PlayerID
	if kind == "friend_request" || kind == "friend_delete" || kind == "block" || kind == "unblock" || kind == "note" || kind == "message.send" || kind == "message.read" {
		if peer <= 0 {
			return nil, store.Invalid
		}
	}
	if peer == player {
		return nil, store.Invalid
	}
	switch kind {
	case "friend_request":
		var recent int
		if err := transaction.QueryRowContext(ctx, "SELECT COUNT(*) FROM pve_social_friend_requests WHERE requester_id=? AND updated_at>UTC_TIMESTAMP(3)-INTERVAL 1 MINUTE", player).Scan(&recent); err != nil {
			return nil, err
		}
		if recent >= 20 {
			return nil, store.Conflict
		}
		if err := store.LockPlayers(ctx, transaction, []int64{peer}); err != nil {
			return nil, err
		}
		blocked, err := store.Blocked(ctx, transaction, player, peer)
		if err != nil {
			return nil, err
		}
		if blocked {
			return nil, store.Forbidden
		}
		friends, err := store.Friends(ctx, transaction, player, peer)
		if err != nil {
			return nil, err
		}
		if friends {
			return nil, store.Conflict
		}
		if _, err := transaction.ExecContext(ctx, "INSERT INTO pve_social_friend_requests(requester_id,recipient_id,status) VALUES(?,?,'pending') ON DUPLICATE KEY UPDATE status='pending',updated_at=UTC_TIMESTAMP(3)", player, peer); err != nil {
			return nil, err
		}
		var id int64
		if err := transaction.QueryRowContext(ctx, "SELECT id FROM pve_social_friend_requests WHERE requester_id=? AND recipient_id=?", player, peer).Scan(&id); err != nil {
			return nil, err
		}
		value := map[string]any{"id": id, "requester_id": player, "recipient_id": peer, "status": "pending"}
		return value, store.Notify(ctx, transaction, []int64{peer}, "v2.social.friend_requested", value)
	case "friend_response", "friend_withdraw":
		var requester, recipient int64
		var status string
		err := transaction.QueryRowContext(ctx, "SELECT requester_id,recipient_id,status FROM pve_social_friend_requests WHERE id=? FOR UPDATE", input.RequestID).Scan(&requester, &recipient, &status)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, store.NotFound
		}
		if err != nil {
			return nil, err
		}
		if status != "pending" {
			return nil, store.Conflict
		}
		next := "rejected"
		if kind == "friend_withdraw" {
			if requester != player {
				return nil, store.Forbidden
			}
			next = "withdrawn"
		} else {
			if recipient != player {
				return nil, store.Forbidden
			}
			blocked, err := store.Blocked(ctx, transaction, requester, recipient)
			if err != nil {
				return nil, err
			}
			if blocked {
				return nil, store.Forbidden
			}
			if input.Accept {
				next = "accepted"
				low, high := canonical(requester, recipient)
				if _, err := transaction.ExecContext(ctx, "INSERT INTO pve_social_friendships(player_low_id,player_high_id,status) VALUES(?,?,'active') ON DUPLICATE KEY UPDATE status='active'", low, high); err != nil {
					return nil, err
				}
			}
		}
		_, err = transaction.ExecContext(ctx, "UPDATE pve_social_friend_requests SET status=?,updated_at=UTC_TIMESTAMP(3) WHERE id=?", next, input.RequestID)
		return map[string]any{"status": next}, err
	case "friend_delete":
		low, high := canonical(player, peer)
		_, err := transaction.ExecContext(ctx, "UPDATE pve_social_friendships SET status='removed' WHERE player_low_id=? AND player_high_id=?", low, high)
		return map[string]any{"removed": true}, err
	case "block":
		if peer <= 0 {
			return nil, store.Invalid
		}
		if _, err := transaction.ExecContext(ctx, "INSERT IGNORE INTO pve_social_blocks(blocker_id,blocked_id) VALUES(?,?)", player, peer); err != nil {
			return nil, err
		}
		low, high := canonical(player, peer)
		if _, err := transaction.ExecContext(ctx, "UPDATE pve_social_friendships SET status='removed' WHERE player_low_id=? AND player_high_id=?", low, high); err != nil {
			return nil, err
		}
		_, err := transaction.ExecContext(ctx, "UPDATE pve_social_friend_requests SET status='rejected' WHERE status='pending' AND ((requester_id=? AND recipient_id=?) OR (requester_id=? AND recipient_id=?))", player, peer, peer, player)
		return map[string]any{"blocked": true}, err
	case "unblock":
		_, err := transaction.ExecContext(ctx, "DELETE FROM pve_social_blocks WHERE blocker_id=? AND blocked_id=?", player, peer)
		return map[string]any{"blocked": false}, err
	case "note":
		friends, err := store.Friends(ctx, transaction, player, peer)
		if err != nil {
			return nil, err
		}
		if !friends {
			return nil, store.Forbidden
		}
		if len([]byte(input.Note)) > 128 {
			return nil, store.Invalid
		}
		_, err = transaction.ExecContext(ctx, "INSERT INTO pve_social_notes(owner_id,target_id,note) VALUES(?,?,?) ON DUPLICATE KEY UPDATE note=VALUES(note)", player, peer, strings.TrimSpace(input.Note))
		return map[string]any{"saved": true}, err
	case "message.send":
		body := strings.TrimSpace(input.Body)
		if len([]byte(body)) < 1 || len([]byte(body)) > 512 {
			return nil, store.Invalid
		}
		friends, err := store.Friends(ctx, transaction, player, peer)
		if err != nil {
			return nil, err
		}
		blocked, err := store.Blocked(ctx, transaction, player, peer)
		if err != nil {
			return nil, err
		}
		if !friends || blocked {
			return nil, store.Forbidden
		}
		var recent int
		if err := transaction.QueryRowContext(ctx, "SELECT COUNT(*) FROM pve_social_messages WHERE sender_id=? AND created_at>UTC_TIMESTAMP(3)-INTERVAL 1 MINUTE", player).Scan(&recent); err != nil {
			return nil, err
		}
		if recent >= 30 {
			return nil, store.Conflict
		}
		result, err := transaction.ExecContext(ctx, "INSERT INTO pve_social_messages(sender_id,recipient_id,body) VALUES(?,?,?)", player, peer, body)
		if err != nil {
			return nil, err
		}
		id, err := result.LastInsertId()
		if err != nil {
			return nil, err
		}
		value := map[string]any{"id": id, "sender_id": player, "recipient_id": peer, "body": body}
		return value, store.Notify(ctx, transaction, []int64{peer}, "v2.social.message.received", value)
	case "message.read":
		if input.MessageID < 0 {
			return nil, store.Invalid
		}
		var maximum int64
		if err := transaction.QueryRowContext(ctx, "SELECT COALESCE(MAX(id),0) FROM pve_social_messages WHERE sender_id=? AND recipient_id=?", peer, player).Scan(&maximum); err != nil {
			return nil, err
		}
		if input.MessageID > maximum {
			return nil, store.Invalid
		}
		_, err := transaction.ExecContext(ctx, "INSERT INTO pve_social_message_reads(player_id,peer_id,last_read_message_id) VALUES(?,?,?) ON DUPLICATE KEY UPDATE last_read_message_id=GREATEST(last_read_message_id,VALUES(last_read_message_id))", player, peer, input.MessageID)
		return map[string]any{"read_through": input.MessageID}, err
	default:
		return nil, store.NotFound
	}
}

func (s *Service) Query(ctx context.Context, player int64, kind string, peer int64) (any, error) {
	query := ""
	args := []any{player}
	switch kind {
	case "friends":
		query = "SELECT p.id,p.nickname,n.note FROM pve_social_friendships f JOIN players p ON p.id=CASE WHEN f.player_low_id=? THEN f.player_high_id ELSE f.player_low_id END LEFT JOIN pve_social_notes n ON n.owner_id=? AND n.target_id=p.id WHERE f.status='active' AND (f.player_low_id=? OR f.player_high_id=?) ORDER BY p.id LIMIT 200"
		args = []any{player, player, player, player}
	case "requests":
		query = "SELECT id,requester_id,recipient_id,status FROM pve_social_friend_requests WHERE (recipient_id=? OR requester_id=?) ORDER BY id DESC LIMIT 50"
		args = []any{player, player}
	case "blocks":
		query = "SELECT blocked_id FROM pve_social_blocks WHERE blocker_id=? LIMIT 200"
	case "unread":
		query = "SELECT m.sender_id,COUNT(*) AS unread FROM pve_social_messages m LEFT JOIN pve_social_message_reads r ON r.player_id=m.recipient_id AND r.peer_id=m.sender_id WHERE m.recipient_id=? AND m.id>COALESCE(r.last_read_message_id,0) GROUP BY m.sender_id LIMIT 200"
	case "messages":
		if peer <= 0 || peer == player {
			return nil, store.Invalid
		}
		query = "SELECT id,sender_id,recipient_id,body,created_at FROM pve_social_messages WHERE ((sender_id=? AND recipient_id=?) OR (sender_id=? AND recipient_id=?)) AND created_at>? ORDER BY id DESC LIMIT 50"
		args = []any{player, peer, peer, player, time.Now().UTC().Add(-time.Duration(s.RetentionDays) * 24 * time.Hour)}
	default:
		return nil, store.NotFound
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return Rows(rows)
}
func Rows(rows *sql.Rows) ([]map[string]any, error) {
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	columnTypes, err := rows.ColumnTypes()
	if err != nil {
		return nil, err
	}
	result := []map[string]any{}
	for rows.Next() {
		values := make([]any, len(columns))
		pointers := make([]any, len(columns))
		for index := range values {
			pointers[index] = &values[index]
		}
		if err := rows.Scan(pointers...); err != nil {
			return nil, err
		}
		item := map[string]any{}
		for index, column := range columns {
			value := values[index]
			if data, ok := value.([]byte); ok {
				if columnTypes[index].DatabaseTypeName() == "JSON" {
					value = json.RawMessage(data)
				} else {
					value = string(data)
				}
			}
			if instant, ok := value.(time.Time); ok {
				value = instant.UTC().Format(time.RFC3339Nano)
			}
			item[column] = value
		}
		result = append(result, item)
	}
	return result, rows.Err()
}
