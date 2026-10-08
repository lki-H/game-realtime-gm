package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"game-realtime-gm/backend/internal/middleware"
	"game-realtime-gm/backend/internal/pve/control"
	"game-realtime-gm/backend/internal/pve/matchmaking"
	"game-realtime-gm/backend/internal/pve/store"
	"github.com/gin-gonic/gin"
)

type v2ControlRequest struct {
	OperationID     string `json:"operation_id"`
	AdmissionState  string `json:"admission_state"`
	ExpectedVersion int64  `json:"expected_version"`
	Reason          string `json:"reason"`
}

func (handler *AdminHandler) V2ControlState(c *gin.Context) {
	snapshot, err := control.Observe(c.Request.Context(), handler.db)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": 50371, "message": "control state unavailable"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok", "data": snapshot})
}

func (handler *AdminHandler) V2Control(c *gin.Context) {
	adminID, ok := middleware.CurrentAdminID(c)
	if !ok || middleware.CurrentAdminRole(c) != "operator" {
		c.JSON(http.StatusForbidden, gin.H{"code": 40370, "message": "operator permission required"})
		return
	}
	if handler.V2App == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": 50371, "message": "v2 control unavailable"})
		return
	}
	var input v2ControlRequest
	if c.ShouldBindJSON(&input) != nil || !store.ValidID(input.OperationID) || len(input.OperationID) > 120 || input.ExpectedVersion < 1 || strings.TrimSpace(input.Reason) == "" || len(input.Reason) > 512 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40070, "message": "invalid control request"})
		return
	}
	if input.AdmissionState != "open" && input.AdmissionState != "draining" && input.AdmissionState != "closed" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40070, "message": "invalid admission state"})
		return
	}
	fingerprint := store.Hash(store.JSON(map[string]any{"admin_id": adminID, "input": input}))
	var snapshot control.Snapshot
	err := store.Transaction(c.Request.Context(), handler.db, func(transaction *sql.Tx) error {
		if err := matchmaking.Lane(c.Request.Context(), transaction); err != nil {
			return err
		}
		state, err := control.ReadTx(c.Request.Context(), transaction, "exclusive")
		if err != nil {
			return err
		}
		if err := requireOperatorTx(c, transaction, adminID); err != nil {
			return err
		}
		var prior []byte
		var priorFingerprint string
		err = transaction.QueryRowContext(c.Request.Context(), "SELECT fingerprint,result FROM pve_control_operations WHERE operation_id=?", input.OperationID).Scan(&priorFingerprint, &prior)
		if err == nil {
			if priorFingerprint != fingerprint {
				return store.Conflict
			}
			return json.Unmarshal(prior, &snapshot)
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if state.Version != input.ExpectedVersion {
			return store.Conflict
		}
		if input.AdmissionState == "draining" {
			if err := handler.V2App.Match.DrainTx(c.Request.Context(), transaction); err != nil {
				return err
			}
		}
		if input.AdmissionState == "closed" {
			if state.AdmissionState != "draining" {
				return store.Conflict
			}
			candidate := control.Snapshot{State: state}
			if err := control.ReadCountsTx(c.Request.Context(), transaction, &candidate); err != nil {
				return err
			}
			if candidate.QueuedTickets+candidate.ProposedTickets+candidate.PendingProposals+candidate.ActiveRuns+candidate.EndingRuns+candidate.PendingOperations+candidate.NeedsRepair+candidate.ActivityLocks+candidate.PendingOutbox != 0 {
				return store.Conflict
			}
		}
		if _, err := transaction.ExecContext(c.Request.Context(), "UPDATE pve_service_control SET admission_state=?,version=version+1,reason=?,updated_at=UTC_TIMESTAMP(3) WHERE id='gameplay'", input.AdmissionState, strings.TrimSpace(input.Reason)); err != nil {
			return err
		}
		snapshot.State, err = control.ReadTx(c.Request.Context(), transaction, "")
		if err != nil {
			return err
		}
		if err := control.ReadCountsTx(c.Request.Context(), transaction, &snapshot); err != nil {
			return err
		}
		snapshot.ReadyToStop = snapshot.AdmissionState == "draining" && snapshot.QueuedTickets+snapshot.ProposedTickets+snapshot.PendingProposals+snapshot.ActiveRuns+snapshot.EndingRuns+snapshot.PendingOperations+snapshot.NeedsRepair+snapshot.ActivityLocks+snapshot.PendingOutbox == 0
		snapshot.ObservedAt = time.Now().UTC()
		payload := store.JSON(snapshot)
		if _, err := transaction.ExecContext(c.Request.Context(), "INSERT INTO pve_control_operations(operation_id,admin_id,fingerprint,result) VALUES(?,?,?,?)", input.OperationID, adminID, fingerprint, payload); err != nil {
			return err
		}
		return handler.recordOperationTx(c, transaction, "admin.v2.control."+input.AdmissionState, "pve_service_control", nil, string(store.JSON(map[string]any{"operation_id": input.OperationID, "reason": strings.TrimSpace(input.Reason)})))
	})
	if err != nil {
		status := http.StatusConflict
		if errorCode(err) == 50070 || errorCode(err) == 50371 {
			status = http.StatusServiceUnavailable
		}
		if errors.Is(err, store.Forbidden) {
			status = http.StatusForbidden
		}
		c.JSON(status, gin.H{"code": errorCode(err), "message": errorText(err)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok", "data": snapshot})
}
