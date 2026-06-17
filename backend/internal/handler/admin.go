package handler

import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"game-realtime-gm/backend/internal/middleware"
	"game-realtime-gm/backend/internal/model"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type AdminHandler struct {
	db          *pgxpool.Pool
	redisClient *redis.Client
}

type banPlayerRequest struct {
	Reason string `json:"reason" binding:"required"`
}

type unbanPlayerRequest struct {
	Reason string `json:"reason" binding:"required"`
}

func NewAdminHandler(db *pgxpool.Pool, redisClient *redis.Client) *AdminHandler {
	return &AdminHandler{
		db:          db,
		redisClient: redisClient,
	}
}

func (h *AdminHandler) Me(c *gin.Context) {
	adminID, ok := middleware.CurrentAdminID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    40115,
			"message": "admin identity missing",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "ok",
		"data": gin.H{
			"id":       adminID,
			"username": middleware.CurrentAdminUsername(c),
			"role":     middleware.CurrentAdminRole(c),
		},
	})
}

func (h *AdminHandler) DashboardSummary(c *gin.Context) {
	location := time.FixedZone("Asia/Shanghai", 8*60*60)
	now := time.Now().In(location)
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, location)

	var totalPlayers int64
	if err := h.db.QueryRow(c.Request.Context(), `SELECT COUNT(*) FROM players`).Scan(&totalPlayers); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50091,
			"message": "count total players failed",
		})
		return
	}

	var normalPlayers int64
	if err := h.db.QueryRow(c.Request.Context(), `SELECT COUNT(*) FROM players WHERE status = $1`, "normal").Scan(&normalPlayers); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50092,
			"message": "count normal players failed",
		})
		return
	}

	var bannedPlayers int64
	if err := h.db.QueryRow(c.Request.Context(), `SELECT COUNT(*) FROM players WHERE status = $1`, "banned").Scan(&bannedPlayers); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50093,
			"message": "count banned players failed",
		})
		return
	}

	var todayNewPlayers int64
	if err := h.db.QueryRow(c.Request.Context(), `SELECT COUNT(*) FROM players WHERE created_at >= $1`, todayStart).Scan(&todayNewPlayers); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50094,
			"message": "count today new players failed",
		})
		return
	}

	onlinePlayers, err := h.countOnlinePlayers(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50095,
			"message": "count online players failed",
		})
		return
	}

	var todayAdminOperations int64
	if err := h.db.QueryRow(c.Request.Context(), `SELECT COUNT(*) FROM admin_operation_logs WHERE created_at >= $1`, todayStart).Scan(&todayAdminOperations); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50096,
			"message": "count today admin operations failed",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "ok",
		"data": gin.H{
			"total_players":          totalPlayers,
			"normal_players":         normalPlayers,
			"banned_players":         bannedPlayers,
			"today_new_players":      todayNewPlayers,
			"online_players":         onlinePlayers,
			"today_admin_operations": todayAdminOperations,
		},
	})
}

func (h *AdminHandler) countOnlinePlayers(c *gin.Context) (int64, error) {
	var cursor uint64
	var total int64

	for {
		keys, nextCursor, err := h.redisClient.Scan(c.Request.Context(), cursor, "online:player:*", 100).Result()
		if err != nil {
			return 0, err
		}

		total += int64(len(keys))
		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	return total, nil
}

func (h *AdminHandler) recordOperation(c *gin.Context, action string, targetType string, targetID *int64, detail string) {
	adminID, ok := middleware.CurrentAdminID(c)
	if !ok {
		return
	}

	var targetValue any
	if targetID != nil {
		targetValue = *targetID
	}

	_, err := h.db.Exec(
		c.Request.Context(),
		`INSERT INTO admin_operation_logs (
            admin_id,
            admin_username,
            admin_role,
            action,
            target_type,
            target_id,
            detail,
            ip,
            user_agent
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		adminID,
		middleware.CurrentAdminUsername(c),
		middleware.CurrentAdminRole(c),
		action,
		targetType,
		targetValue,
		detail,
		c.ClientIP(),
		c.GetHeader("User-Agent"),
	)
	if err != nil {
		log.Printf("record admin operation failed: action=%s target_type=%s target_id=%v err=%v", action, targetType, targetValue, err)
	}
}

func (h *AdminHandler) recordOperationTx(c *gin.Context, tx pgx.Tx, action string, targetType string, targetID *int64, detail string) error {
	adminID, ok := middleware.CurrentAdminID(c)
	if !ok {
		return nil
	}

	var targetValue any
	if targetID != nil {
		targetValue = *targetID
	}

	_, err := tx.Exec(
		c.Request.Context(),
		`INSERT INTO admin_operation_logs (
            admin_id,
            admin_username,
            admin_role,
            action,
            target_type,
            target_id,
            detail,
            ip,
            user_agent
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		adminID,
		middleware.CurrentAdminUsername(c),
		middleware.CurrentAdminRole(c),
		action,
		targetType,
		targetValue,
		detail,
		c.ClientIP(),
		c.GetHeader("User-Agent"),
	)
	return err
}

func (h *AdminHandler) ListPlayers(c *gin.Context) {
	page := parsePositiveInt(c.DefaultQuery("page", "1"), 1)
	pageSize := parsePositiveInt(c.DefaultQuery("page_size", "10"), 10)
	if pageSize > 50 {
		pageSize = 50
	}

	keyword := strings.TrimSpace(c.Query("keyword"))
	offset := (page - 1) * pageSize

	whereSQL := ""
	args := []any{}
	if keyword != "" {
		whereSQL = "WHERE username ILIKE $1 OR nickname ILIKE $1"
		args = append(args, "%"+keyword+"%")
	}

	var total int64
	countSQL := `SELECT COUNT(*) FROM players ` + whereSQL
	if err := h.db.QueryRow(c.Request.Context(), countSQL, args...).Scan(&total); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50041,
			"message": "count players failed",
		})
		return
	}

	listArgs := append(args, int32(pageSize), int32(offset))
	limitIndex := len(args) + 1
	offsetIndex := len(args) + 2

	rows, err := h.db.Query(
		c.Request.Context(),
		`SELECT id, username, nickname, created_at, updated_at, status, banned_reason, banned_at, banned_by_admin_id
         FROM players
         `+whereSQL+`
         ORDER BY id DESC
         LIMIT $`+strconv.Itoa(limitIndex)+` OFFSET $`+strconv.Itoa(offsetIndex),
		listArgs...,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50042,
			"message": "query player list failed",
		})
		return
	}
	defer rows.Close()

	players := make([]model.Player, 0)
	for rows.Next() {
		var player model.Player
		if err := rows.Scan(
			&player.ID,
			&player.Username,
			&player.Nickname,
			&player.CreatedAt,
			&player.UpdatedAt,
			&player.Status,
			&player.BannedReason,
			&player.BannedAt,
			&player.BannedByAdminID,
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    50043,
				"message": "scan player failed",
			})
			return
		}
		players = append(players, player)
	}

	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50044,
			"message": "read player rows failed",
		})
		return
	}
	detail := "page=" + strconv.Itoa(page) + ",page_size=" + strconv.Itoa(pageSize)
	if keyword != "" {
		detail += ",keyword=" + keyword
	}
	h.recordOperation(c, "admin.players.list", "player", nil, detail)

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "ok",
		"data": gin.H{
			"items":     players,
			"page":      page,
			"page_size": pageSize,
			"total":     total,
		},
	})
}

func (h *AdminHandler) GetPlayerByID(c *gin.Context) {
	playerID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || playerID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40041,
			"message": "invalid player id",
		})
		return
	}

	var player model.Player
	err = h.db.QueryRow(
		c.Request.Context(),
		`SELECT id, username, nickname, created_at, updated_at, status, banned_reason, banned_at, banned_by_admin_id
         FROM players
         WHERE id = $1`,
		playerID,
	).Scan(
		&player.ID,
		&player.Username,
		&player.Nickname,
		&player.CreatedAt,
		&player.UpdatedAt,
		&player.Status,
		&player.BannedReason,
		&player.BannedAt,
		&player.BannedByAdminID,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    40441,
			"message": "player not found",
		})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50045,
			"message": "query player failed",
		})
		return
	}

	h.recordOperation(c, "admin.players.detail", "player", &playerID, "query player detail")

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "ok",
		"data":    player,
	})
}

func (h *AdminHandler) BanPlayer(c *gin.Context) {
	playerID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || playerID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40051,
			"message": "invalid player id",
		})
		return
	}

	var req banPlayerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40052,
			"message": "invalid request",
		})
		return
	}

	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40053,
			"message": "ban reason cannot be empty",
		})
		return
	}
	if len([]rune(reason)) > 200 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40054,
			"message": "ban reason is too long",
		})
		return
	}

	adminID, ok := middleware.CurrentAdminID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    40116,
			"message": "admin identity missing",
		})
		return
	}

	tx, err := h.db.Begin(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50051,
			"message": "begin transaction failed",
		})
		return
	}
	defer tx.Rollback(c.Request.Context())

	var currentStatus string
	err = tx.QueryRow(
		c.Request.Context(),
		`SELECT status FROM players WHERE id = $1`,
		playerID,
	).Scan(&currentStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    40451,
			"message": "player not found",
		})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50052,
			"message": "query player status failed",
		})
		return
	}

	if currentStatus == "banned" {
		c.JSON(http.StatusConflict, gin.H{
			"code":    40951,
			"message": "player already banned",
		})
		return
	}

	var player model.Player
	err = tx.QueryRow(
		c.Request.Context(),
		`UPDATE players
         SET status = 'banned',
             banned_reason = $1,
             banned_at = NOW(),
             banned_by_admin_id = $2,
             updated_at = NOW()
         WHERE id = $3
         RETURNING id, username, nickname, created_at, updated_at, status, banned_reason, banned_at, banned_by_admin_id`,
		reason,
		adminID,
		playerID,
	).Scan(
		&player.ID,
		&player.Username,
		&player.Nickname,
		&player.CreatedAt,
		&player.UpdatedAt,
		&player.Status,
		&player.BannedReason,
		&player.BannedAt,
		&player.BannedByAdminID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50053,
			"message": "ban player failed",
		})
		return
	}

	if err := h.recordOperationTx(c, tx, "admin.players.ban", "player", &playerID, "reason="+reason); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50054,
			"message": "record ban operation failed",
		})
		return
	}

	if err := tx.Commit(c.Request.Context()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50055,
			"message": "commit transaction failed",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "ban player success",
		"data":    player,
	})
}

func (h *AdminHandler) UnbanPlayer(c *gin.Context) {
	playerID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || playerID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40061,
			"message": "invalid player id",
		})
		return
	}

	var req unbanPlayerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40062,
			"message": "invalid request",
		})
		return
	}

	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40063,
			"message": "unban reason cannot be empty",
		})
		return
	}
	if len([]rune(reason)) > 200 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40064,
			"message": "unban reason is too long",
		})
		return
	}

	tx, err := h.db.Begin(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50061,
			"message": "begin transaction failed",
		})
		return
	}
	defer tx.Rollback(c.Request.Context())

	var currentStatus string
	err = tx.QueryRow(
		c.Request.Context(),
		`SELECT status FROM players WHERE id = $1`,
		playerID,
	).Scan(&currentStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    40461,
			"message": "player not found",
		})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50062,
			"message": "query player status failed",
		})
		return
	}

	if currentStatus != "banned" {
		c.JSON(http.StatusConflict, gin.H{
			"code":    40961,
			"message": "player is not banned",
		})
		return
	}

	var player model.Player
	err = tx.QueryRow(
		c.Request.Context(),
		`UPDATE players
         SET status = 'normal',
             banned_reason = '',
             banned_at = NULL,
             banned_by_admin_id = NULL,
             updated_at = NOW()
         WHERE id = $1
         RETURNING id, username, nickname, created_at, updated_at, status, banned_reason, banned_at, banned_by_admin_id`,
		playerID,
	).Scan(
		&player.ID,
		&player.Username,
		&player.Nickname,
		&player.CreatedAt,
		&player.UpdatedAt,
		&player.Status,
		&player.BannedReason,
		&player.BannedAt,
		&player.BannedByAdminID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50063,
			"message": "unban player failed",
		})
		return
	}

	if err := h.recordOperationTx(c, tx, "admin.players.unban", "player", &playerID, "reason="+reason); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50064,
			"message": "record unban operation failed",
		})
		return
	}

	if err := tx.Commit(c.Request.Context()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50065,
			"message": "commit transaction failed",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "unban player success",
		"data":    player,
	})
}

func (h *AdminHandler) ListOperationLogs(c *gin.Context) {
	page := parsePositiveInt(c.DefaultQuery("page", "1"), 1)
	pageSize := parsePositiveInt(c.DefaultQuery("page_size", "10"), 10)
	if pageSize > 50 {
		pageSize = 50
	}
	offset := (page - 1) * pageSize

	action := strings.TrimSpace(c.Query("action"))
	adminUsername := strings.TrimSpace(c.Query("admin_username"))
	targetType := strings.TrimSpace(c.Query("target_type"))
	targetIDText := strings.TrimSpace(c.Query("target_id"))
	startTimeText := strings.TrimSpace(c.Query("start_time"))
	endTimeText := strings.TrimSpace(c.Query("end_time"))
	rangeText := strings.TrimSpace(c.Query("range"))

	whereParts := make([]string, 0)
	args := make([]any, 0)

	if action != "" {
		args = append(args, action)
		whereParts = append(whereParts, "action = $"+strconv.Itoa(len(args)))
	}
	if adminUsername != "" {
		args = append(args, adminUsername)
		whereParts = append(whereParts, "admin_username = $"+strconv.Itoa(len(args)))
	}
	if targetType != "" {
		args = append(args, targetType)
		whereParts = append(whereParts, "target_type = $"+strconv.Itoa(len(args)))
	}
	if targetIDText != "" {
		targetID, err := strconv.ParseInt(targetIDText, 10, 64)
		if err != nil || targetID <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    40071,
				"message": "invalid target id",
			})
			return
		}
		args = append(args, targetID)
		whereParts = append(whereParts, "target_id = $"+strconv.Itoa(len(args)))
	}

	var startTime time.Time
	var endTime time.Time

	if rangeText != "" && startTimeText == "" && endTimeText == "" {
		now := time.Now()
		location := time.FixedZone("Asia/Shanghai", 8*60*60)
		nowInLocation := now.In(location)

		switch rangeText {
		case "today":
			startTime = time.Date(nowInLocation.Year(), nowInLocation.Month(), nowInLocation.Day(), 0, 0, 0, 0, location)
			endTime = nowInLocation
		case "last_7_days":
			endTime = nowInLocation
			startTime = nowInLocation.AddDate(0, 0, -7)
		case "last_30_days":
			endTime = nowInLocation
			startTime = nowInLocation.AddDate(0, 0, -30)
		default:
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    40075,
				"message": "invalid range",
			})
			return
		}

		args = append(args, startTime)
		whereParts = append(whereParts, "created_at >= $"+strconv.Itoa(len(args)))

		args = append(args, endTime)
		whereParts = append(whereParts, "created_at <= $"+strconv.Itoa(len(args)))
	}

	if startTimeText != "" {
		parsedStartTime, err := time.Parse(time.RFC3339, startTimeText)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    40072,
				"message": "invalid start_time",
			})
			return
		}
		startTime = parsedStartTime
		args = append(args, startTime)
		whereParts = append(whereParts, "created_at >= $"+strconv.Itoa(len(args)))
	}

	if endTimeText != "" {
		parsedEndTime, err := time.Parse(time.RFC3339, endTimeText)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    40073,
				"message": "invalid end_time",
			})
			return
		}
		endTime = parsedEndTime
		args = append(args, endTime)
		whereParts = append(whereParts, "created_at <= $"+strconv.Itoa(len(args)))
	}

	if !startTime.IsZero() && !endTime.IsZero() && startTime.After(endTime) {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40074,
			"message": "start_time cannot be after end_time",
		})
		return
	}

	whereSQL := ""
	if len(whereParts) > 0 {
		whereSQL = "WHERE " + strings.Join(whereParts, " AND ")
	}

	var total int64
	countSQL := `SELECT COUNT(*) FROM admin_operation_logs ` + whereSQL
	if err := h.db.QueryRow(c.Request.Context(), countSQL, args...).Scan(&total); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50071,
			"message": "count operation logs failed",
		})
		return
	}

	listArgs := append(args, int32(pageSize), int32(offset))
	limitIndex := len(args) + 1
	offsetIndex := len(args) + 2

	rows, err := h.db.Query(
		c.Request.Context(),
		`SELECT id, admin_id, admin_username, admin_role, action, target_type, target_id, detail, ip, user_agent, created_at
         FROM admin_operation_logs
         `+whereSQL+`
         ORDER BY id DESC
         LIMIT $`+strconv.Itoa(limitIndex)+` OFFSET $`+strconv.Itoa(offsetIndex),
		listArgs...,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50072,
			"message": "query operation logs failed",
		})
		return
	}
	defer rows.Close()

	logs := make([]model.AdminOperationLog, 0)
	for rows.Next() {
		var operationLog model.AdminOperationLog
		if err := rows.Scan(
			&operationLog.ID,
			&operationLog.AdminID,
			&operationLog.AdminUsername,
			&operationLog.AdminRole,
			&operationLog.Action,
			&operationLog.TargetType,
			&operationLog.TargetID,
			&operationLog.Detail,
			&operationLog.IP,
			&operationLog.UserAgent,
			&operationLog.CreatedAt,
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    50073,
				"message": "scan operation log failed",
			})
			return
		}
		logs = append(logs, operationLog)
	}

	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50074,
			"message": "read operation log rows failed",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "ok",
		"data": gin.H{
			"items":     logs,
			"page":      page,
			"page_size": pageSize,
			"total":     total,
		},
	})
}

func (h *AdminHandler) GetOperationLogByID(c *gin.Context) {
	logID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || logID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40081,
			"message": "invalid operation log id",
		})
		return
	}

	var operationLog model.AdminOperationLog
	err = h.db.QueryRow(
		c.Request.Context(),
		`SELECT id, admin_id, admin_username, admin_role, action, target_type, target_id, detail, ip, user_agent, created_at
         FROM admin_operation_logs
         WHERE id = $1`,
		logID,
	).Scan(
		&operationLog.ID,
		&operationLog.AdminID,
		&operationLog.AdminUsername,
		&operationLog.AdminRole,
		&operationLog.Action,
		&operationLog.TargetType,
		&operationLog.TargetID,
		&operationLog.Detail,
		&operationLog.IP,
		&operationLog.UserAgent,
		&operationLog.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    40481,
			"message": "operation log not found",
		})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50081,
			"message": "query operation log failed",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "ok",
		"data":    operationLog,
	})
}
