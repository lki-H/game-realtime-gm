package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"game-realtime-gm/backend/internal/middleware"
	"game-realtime-gm/backend/internal/pve/run"
	"game-realtime-gm/backend/internal/pve/store"
	"github.com/gin-gonic/gin"
)

type repairRequest struct {
	OperationID      string `json:"operation_id"`
	ExpectedAttempts int    `json:"expected_attempts"`
	Reason           string `json:"reason"`
}

func (handler *AdminHandler) V2Retry(c *gin.Context) {
	adminID, ok := middleware.CurrentAdminID(c)
	if !ok || middleware.CurrentAdminRole(c) != "operator" {
		c.JSON(http.StatusForbidden, gin.H{"code": 40370, "message": "operator permission required"})
		return
	}
	var input repairRequest
	if c.ShouldBindJSON(&input) != nil || !store.ValidID(input.OperationID) || len(input.OperationID) > 120 || input.ExpectedAttempts < 1 || len(strings.TrimSpace(input.Reason)) < 1 || len(input.Reason) > 512 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40070, "message": "invalid repair request"})
		return
	}
	target := c.Param("operation_id")
	fingerprint := store.Hash(store.JSON(map[string]any{"admin_id": adminID, "target": target, "input": input}))
	repairID := "repair:" + input.OperationID
	err := store.Transaction(c.Request.Context(), handler.db, func(transaction *sql.Tx) error {
		var runID, kind, status string
		var attempts int
		if err := transaction.QueryRowContext(c.Request.Context(), "SELECT aggregate_id,operation_type,status,attempts FROM pve_pending_operations WHERE operation_id=? FOR UPDATE", target).Scan(&runID, &kind, &status, &attempts); err != nil {
			return err
		}
		if err := requireOperatorTx(c, transaction, adminID); err != nil {
			return err
		}
		var prior []byte
		err := transaction.QueryRowContext(c.Request.Context(), "SELECT payload FROM pve_pending_operations WHERE operation_id=?", repairID).Scan(&prior)
		if err == nil {
			var recorded struct {
				Fingerprint string `json:"fingerprint"`
			}
			if json.Unmarshal(prior, &recorded) != nil || recorded.Fingerprint != fingerprint {
				return store.Conflict
			}
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if (kind != "settlement" && kind != "task_completion") || status != "needs_repair" || attempts != input.ExpectedAttempts {
			return store.Conflict
		}
		state, err := run.Load(c.Request.Context(), transaction, runID)
		if err != nil {
			return err
		}
		if (kind == "settlement" && state.Status != "ending") || (kind == "task_completion" && state.Status != "running" && state.Status != "ending" && state.Status != "closed") {
			return store.Conflict
		}
		var missingAssets int
		if err := transaction.QueryRowContext(c.Request.Context(), "SELECT COUNT(*) FROM pve_run_participants p LEFT JOIN player_assets a ON a.player_id=p.player_id WHERE p.run_id=? AND a.player_id IS NULL", runID).Scan(&missingAssets); err != nil {
			return err
		}
		if missingAssets != 0 {
			return store.Conflict
		}
		if _, err := transaction.ExecContext(c.Request.Context(), "UPDATE pve_pending_operations SET status='pending',attempts=0,next_attempt_at=UTC_TIMESTAMP(3),last_error=NULL,updated_at=UTC_TIMESTAMP(3) WHERE operation_id=?", target); err != nil {
			return err
		}
		payload := store.JSON(map[string]any{"fingerprint": fingerprint, "admin_id": adminID, "target": target, "reason": input.Reason})
		if _, err := transaction.ExecContext(c.Request.Context(), "INSERT INTO pve_pending_operations(operation_id,operation_type,aggregate_id,payload,status,next_attempt_at) VALUES(?,'repair_audit',?,?,'done',?)", repairID, runID, payload, time.Now().UTC()); err != nil {
			return err
		}
		return handler.recordOperationTx(c, transaction, "admin.v2.settlement.retry", "pve_operation", nil, string(store.JSON(map[string]any{"repair_id": repairID, "target": target, "run_id": runID, "reason": input.Reason})))
	})
	if err != nil {
		status := http.StatusConflict
		if errorCode(err) == 50070 {
			status = http.StatusInternalServerError
		}
		if errorCode(err) == 40470 {
			status = http.StatusNotFound
		}
		if errors.Is(err, store.Forbidden) {
			status = http.StatusForbidden
		}
		c.JSON(status, gin.H{"code": errorCode(err), "message": errorText(err)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok", "data": gin.H{"operation_id": target, "repair_id": repairID}})
}
