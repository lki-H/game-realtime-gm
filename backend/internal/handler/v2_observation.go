package handler

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"game-realtime-gm/backend/internal/pve/social"
	"github.com/gin-gonic/gin"
)

func V2Observation(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		page, pageErr := strconv.Atoi(c.DefaultQuery("page", "1"))
		pageSize, sizeErr := strconv.Atoi(c.DefaultQuery("page_size", "20"))
		if pageErr != nil || sizeErr != nil || page < 1 || page > 10000 || pageSize < 1 || pageSize > 50 {
			c.JSON(400, gin.H{"code": 40070, "message": "invalid observation pagination"})
			return
		}
		queries := map[string]string{
			"parties":   "SELECT id,owner_id,status,roster_version,plan_version,join_policy,plan FROM pve_parties ORDER BY updated_at DESC LIMIT 50",
			"proposals": "SELECT id,revision,status,run_id,deadline_at FROM pve_match_proposals ORDER BY created_at DESC LIMIT 50",
			"runs":      "SELECT id,operation_name,difficulty,status,end_reason,started_at,ended_at,reinforcement_used FROM pve_runs ORDER BY created_at DESC LIMIT 50",
			"tasks":     "SELECT run_id,player_id,task_key,compatibility_status,status FROM pve_player_task_attempts ORDER BY created_at DESC LIMIT 50",
			"pending":   "SELECT operation_id,operation_type,aggregate_id,status,attempts,next_attempt_at,last_error FROM pve_pending_operations ORDER BY updated_at DESC LIMIT 50",
		}
		query, exists := queries[c.Param("entity")]
		if !exists {
			c.JSON(404, gin.H{"code": 40470, "message": "unknown observation"})
			return
		}
		tables := map[string]string{"parties": "pve_parties", "proposals": "pve_match_proposals", "runs": "pve_runs", "tasks": "pve_player_task_attempts", "pending": "pve_pending_operations"}
		var total int64
		if err := db.QueryRowContext(c.Request.Context(), "SELECT COUNT(*) FROM "+tables[c.Param("entity")]).Scan(&total); err != nil {
			c.JSON(500, gin.H{"code": 50070, "message": "count observation failed"})
			return
		}
		query = strings.TrimSuffix(query, " LIMIT 50") + ", " + map[string]string{"parties": "id", "proposals": "id", "runs": "id", "tasks": "id", "pending": "operation_id"}[c.Param("entity")] + " LIMIT ? OFFSET ?"
		rows, err := db.QueryContext(c.Request.Context(), query, pageSize, (page-1)*pageSize)
		if err != nil {
			c.JSON(500, gin.H{"code": 50070, "message": "query observation failed"})
			return
		}
		defer rows.Close()
		items, err := social.Rows(rows)
		if err != nil {
			c.JSON(500, gin.H{"code": 50070, "message": "read observation failed"})
			return
		}
		c.JSON(200, gin.H{"code": 0, "message": "ok", "data": items, "pagination": gin.H{"page": page, "page_size": pageSize, "total": total}, "observed_at": time.Now().UTC()})
	}
}
func V2Metrics(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		queries := []struct{ name, query string }{
			{"pve_queued_tickets", "SELECT COUNT(*) FROM pve_match_tickets WHERE status='queued'"},
			{"pve_outbox_pending", "SELECT COUNT(*) FROM pve_outbox_records WHERE status='pending'"},
			{"pve_pending_oldest_seconds", "SELECT COALESCE(MAX(TIMESTAMPDIFF(SECOND,updated_at,UTC_TIMESTAMP(3))),0) FROM pve_pending_operations WHERE status IN ('pending','retryable_failed')"},
			{"pve_event_gaps", "SELECT COUNT(*) FROM pve_runs WHERE status='running' AND JSON_EXTRACT(state,'$.gap_deadline') IS NOT NULL"},
			{"pve_queue_wait_seconds", "SELECT COALESCE(AVG(TIMESTAMPDIFF(MICROSECOND,queue_priority_since,UTC_TIMESTAMP(3)))/1000000,0) FROM pve_match_tickets WHERE status='queued'"},
			{"pve_proposal_rejected_total", "SELECT COUNT(*) FROM pve_match_proposals WHERE status='rejected'"},
			{"pve_loading_failed_total", "SELECT COUNT(*) FROM pve_runs WHERE end_reason='loading_failed'"},
			{"pve_event_gap_total", "SELECT COUNT(*) FROM pve_runs WHERE end_reason='event_gap'"},
			{"pve_reinforcement_reserved", "SELECT COALESCE(SUM(reinforcement_reserved),0) FROM pve_runs WHERE status='running'"},
			{"pve_worker_retries_total", "SELECT COALESCE(SUM(GREATEST(attempts-1,0)),0) FROM pve_pending_operations"},
			{"pve_needs_repair", "SELECT COUNT(*) FROM pve_pending_operations WHERE status='needs_repair'"},
			{"pve_settlement_seconds", "SELECT COALESCE(AVG(TIMESTAMPDIFF(MICROSECOND,r.ended_at,p.settled_at))/1000000,0) FROM pve_participant_results p JOIN pve_runs r ON r.id=p.run_id WHERE p.result_status='settled'"},
		}
		var body strings.Builder
		for _, metric := range queries {
			var value float64
			if err := db.QueryRowContext(c.Request.Context(), metric.query).Scan(&value); err != nil {
				c.Status(503)
				return
			}
			kind := "gauge"
			if strings.HasSuffix(metric.name, "_total") {
				kind = "counter"
			}
			fmt.Fprintf(&body, "# TYPE %s %s\n%s %g\n", metric.name, kind, metric.name, value)
		}
		c.Data(http.StatusOK, "text/plain; version=0.0.4; charset=utf-8", []byte(body.String()))
	}
}
