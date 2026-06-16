# Day 14 学习计划：GM 操作日志快捷时间范围

## 你今天从任务几开始

你现在从：

```text
任务 0：确认 Day 13 时间范围筛选已经存在
```

开始。

我已经检查到当前项目里：

```text
AdminHandler.ListOperationLogs
```

已经有：

```text
start_time
end_time
time.Parse(time.RFC3339, ...)
```

并且：

```text
go test ./...
```

当前是通过的。

所以 Day 14 不新增接口，而是在 Day 13 的基础上加一个更方便的参数：

```text
range
```

## 今日目标

给 GM 操作日志查询接口增加快捷时间范围：

```text
GET /api/admin/operation-logs?range=today
GET /api/admin/operation-logs?range=last_7_days
GET /api/admin/operation-logs?range=last_30_days
```

这样以后前端 GM 后台可以做三个快捷按钮：

```text
今天
最近 7 天
最近 30 天
```

不用每次手动拼：

```text
start_time=2026-06-16T00:00:00%2B08:00
end_time=2026-06-16T23:59:59%2B08:00
```

## 今日最终效果

查询今天的日志：

```text
GET http://localhost:8080/api/admin/operation-logs?range=today
```

查询最近 7 天：

```text
GET http://localhost:8080/api/admin/operation-logs?range=last_7_days
```

查询最近 30 天：

```text
GET http://localhost:8080/api/admin/operation-logs?range=last_30_days
```

如果传了不支持的 range：

```text
GET /api/admin/operation-logs?range=yesterday
```

返回：

```json
{
  "code": 40075,
  "message": "invalid range"
}
```

如果同时传：

```text
range=today
start_time=2026-06-16T00:00:00+08:00
```

今天的规则是：

```text
手动 start_time / end_time 优先，range 自动忽略。
```

原因是手动时间更精确。

## 今日会学到什么

今天会学到：

- 如何设计快捷筛选参数。
- 如何处理 `range` 和 `start_time/end_time` 的优先级。
- 如何用 Go 计算今天开始时间。
- 如何用 Go 计算最近 7 天和最近 30 天。
- 为什么后端要明确使用业务时区。
- 为什么无效 range 不能静默忽略。

## 今日文件范围

今天只修改一个文件：

```text
E:\game-realtime-gm\backend\internal\handler\admin.go
```

不需要改：

```text
router.go
schema.sql
model/admin_operation_log.go
```

原因：

Day 12 已经有接口：

```text
GET /api/admin/operation-logs
```

Day 13 已经有时间范围字段：

```text
created_at
```

Day 14 只是增强查询参数。

## 具体文件定位

打开：

```text
E:\game-realtime-gm\backend\internal\handler\admin.go
```

搜索：

```text
func (h *AdminHandler) ListOperationLogs
```

在这个方法里继续搜索：

```text
startTimeText := strings.TrimSpace(c.Query("start_time"))
```

你现在应该能看到：

```go
	startTimeText := strings.TrimSpace(c.Query("start_time"))
	endTimeText := strings.TrimSpace(c.Query("end_time"))
```

Day 14 要在这里新增：

```go
	rangeText := strings.TrimSpace(c.Query("range"))
```

再继续搜索：

```text
var startTime time.Time
```

Day 14 的快捷时间范围逻辑要放在：

```text
var startTime time.Time
var endTime time.Time
```

后面，手动解析 `start_time` 之前。

## 任务 0：确认 Day 13 状态

### 在哪里执行

PowerShell。

### 执行命令

```powershell
cd E:\game-realtime-gm\backend
go test ./...
```

再执行：

```powershell
Select-String -Path E:\game-realtime-gm\backend\internal\handler\admin.go -Pattern "start_time"
Select-String -Path E:\game-realtime-gm\backend\internal\handler\admin.go -Pattern "time.RFC3339"
```

如果都有输出，说明 Day 13 代码存在。

## 任务 1：读取 range 参数

### 修改文件

```text
E:\game-realtime-gm\backend\internal\handler\admin.go
```

### 修改位置

在 `ListOperationLogs` 里找到：

```go
	startTimeText := strings.TrimSpace(c.Query("start_time"))
	endTimeText := strings.TrimSpace(c.Query("end_time"))
```

改成：

```go
	startTimeText := strings.TrimSpace(c.Query("start_time"))
	endTimeText := strings.TrimSpace(c.Query("end_time"))
	rangeText := strings.TrimSpace(c.Query("range"))
```

### 参数解释

`range` 是快捷时间范围。

今天支持：

```text
today
last_7_days
last_30_days
```

## 任务 2：新增快捷时间范围逻辑

### 修改位置

在 `ListOperationLogs` 里找到：

```go
	var startTime time.Time
	var endTime time.Time
```

在它下面、下面这段代码前面：

```go
	if startTimeText != "" {
```

插入：

```go
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
```

### 插入后的结构应该是

```go
	var startTime time.Time
	var endTime time.Time

	if rangeText != "" && startTimeText == "" && endTimeText == "" {
		...
	}

	if startTimeText != "" {
		...
	}

	if endTimeText != "" {
		...
	}
```

## 任务 3：理解优先级规则

今天的规则是：

```text
如果传了 start_time 或 end_time，就使用手动时间。
只有 start_time 和 end_time 都没传时，range 才生效。
```

也就是：

```go
if rangeText != "" && startTimeText == "" && endTimeText == "" {
```

### 为什么这样设计

`range=today` 是快捷选择。

`start_time/end_time` 是精确选择。

如果用户同时传：

```text
range=today
start_time=2026-06-15T00:00:00+08:00
end_time=2026-06-15T23:59:59+08:00
```

我们应该尊重更精确的手动时间。

否则用户会困惑：

```text
我明明传了 start_time，为什么没生效？
```

### 为什么无效 range 要返回 400

如果用户只传：

```text
range=yesterday
```

后端不能静默忽略。

否则用户以为筛选了昨天，实际却查了全部日志。

这会影响后台审计判断。

所以返回：

```text
40075 invalid range
```

## 任务 4：关于时区

今天用：

```go
location := time.FixedZone("Asia/Shanghai", 8*60*60)
```

表示：

```text
中国时区 UTC+8
```

为什么不用服务器本地时区？

因为你现在是 Windows + Docker + PostgreSQL，日志里可能看到 UTC：

```text
2026-06-16 06:00:00+00
```

而你日常理解的是中国时间：

```text
2026-06-16 14:00:00+08:00
```

所以快捷范围明确按：

```text
Asia/Shanghai
```

来算。

注意：`time.FixedZone` 只是一个固定 UTC+8 偏移，不依赖系统是否安装时区数据库。对中国时间来说够用。

## 任务 5：完整 ListOperationLogs 对照版

如果你担心插错，可以只替换：

```text
ListOperationLogs
```

这一个方法。

### 替换范围

文件：

```text
E:\game-realtime-gm\backend\internal\handler\admin.go
```

从：

```go
func (h *AdminHandler) ListOperationLogs(c *gin.Context) {
```

开始，替换到这个方法自己的最后一个：

```go
}
```

不要替换整个 `admin.go`。

### 完整方法

```go
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
```

## 任务 6：格式化并编译

PowerShell 执行：

```powershell
cd E:\game-realtime-gm\backend
gofmt -w .\internal\handler\admin.go
go test ./...
```

成功看到：

```text
?   	game-realtime-gm/backend/internal/handler	[no test files]
?   	game-realtime-gm/backend/internal/router	[no test files]
```

## 任务 7：启动后端

PowerShell 执行：

```powershell
cd E:\game-realtime-gm\backend
go run .\cmd\server
```

成功看到：

```text
database connected
redis connected
server listening on :8080
```

如果 8080 被占用，先停掉旧后端。

## 任务 8：用 Apifox 测试 today

先登录管理员：

```text
POST http://localhost:8080/api/admin/login
```

Body：

```json
{
  "username": "admin",
  "password": "admin123456"
}
```

复制管理员 token。

请求：

```text
GET http://localhost:8080/api/admin/operation-logs?range=today&page=1&page_size=10
```

Headers：

```text
Authorization: Bearer 管理员token
```

预期：

```json
{
  "code": 0,
  "message": "ok"
}
```

如果今天没有日志，`items` 可能是空数组。

## 任务 9：测试 last_7_days 和 last_30_days

请求最近 7 天：

```text
GET http://localhost:8080/api/admin/operation-logs?range=last_7_days&page=1&page_size=10
```

请求最近 30 天：

```text
GET http://localhost:8080/api/admin/operation-logs?range=last_30_days&page=1&page_size=10
```

都应该返回：

```json
{
  "code": 0,
  "message": "ok"
}
```

## 任务 10：测试无效 range

请求：

```text
GET http://localhost:8080/api/admin/operation-logs?range=yesterday
```

预期：

```json
{
  "code": 40075,
  "message": "invalid range"
}
```

## 任务 11：测试手动时间优先

请求：

```text
GET http://localhost:8080/api/admin/operation-logs?range=today&start_time=2026-06-16T00:00:00%2B08:00&end_time=2026-06-16T23:59:59%2B08:00
```

预期：

```text
不会返回 invalid range。
使用 start_time/end_time 作为筛选条件。
```

如果你把 `range` 改成无效值：

```text
range=abc
```

但同时传了 `start_time/end_time`，按今天规则仍然会使用手动时间，不报 40075。

这是因为：

```text
手动时间优先。
```

## 任务 12：组合筛选

查询最近 7 天某个玩家的封禁日志：

```text
GET /api/admin/operation-logs?range=last_7_days&action=admin.players.ban&target_type=player&target_id=1
```

预期只返回同时满足：

```text
最近 7 天
action = admin.players.ban
target_type = player
target_id = 1
```

的日志。

## 常见问题

### 1. range=today 返回空列表

不一定是错误。

可能今天没有日志。

你可以先触发一个日志：

```text
GET /api/admin/players?page=1&page_size=10
```

这个会写：

```text
admin.players.list
```

然后再查：

```text
GET /api/admin/operation-logs?range=today
```

### 2. range=yesterday 为什么不支持

今天只做三个明确值：

```text
today
last_7_days
last_30_days
```

后续要加 yesterday 可以再单独扩展。

### 3. 为什么 last_7_days 是从现在往前 7 天

今天的 `last_7_days` 是：

```text
当前时间 - 7 天
到 当前时间
```

不是最近 7 个自然日。

如果以后前端需要“最近 7 个自然日”，可以再改规则。

### 4. 权限有变化吗

没有。

仍然只有管理员 token 能访问：

```text
GET /api/admin/operation-logs
```

## 今日验收清单

- [ ] `ListOperationLogs` 已读取 `range` 参数。
- [ ] `range=today` 能正常查询。
- [ ] `range=last_7_days` 能正常查询。
- [ ] `range=last_30_days` 能正常查询。
- [ ] `range=yesterday` 返回 40075。
- [ ] 手动 `start_time/end_time` 优先于 `range`。
- [ ] `range` 可以和 `action`、`target_id` 组合筛选。
- [ ] `go test ./...` 通过。
- [ ] 管理员 token 可以查询。
- [ ] 玩家 token 和无 token 仍然不能查询。

## 请求链路复盘

```text
Apifox
  ↓
GET /api/admin/operation-logs?range=today
  ↓
router.go
  ↓
middleware.AdminAuth
  ↓
AdminHandler.ListOperationLogs
  ↓
如果没有 start_time/end_time，就解析 range
  ↓
计算 startTime/endTime
  ↓
拼接 created_at >= $n / created_at <= $n
  ↓
SELECT COUNT(*) FROM admin_operation_logs
  ↓
SELECT ... FROM admin_operation_logs
  ↓
JSON response
```

## 面试怎么讲这一部分

可以这样说：

```text
在操作日志支持 start_time/end_time 精确时间筛选后，我继续增加了快捷时间范围参数 range，支持 today、last_7_days 和 last_30_days。后端会在没有传 start_time/end_time 时使用 range 自动计算时间区间，并按 Asia/Shanghai 的 UTC+8 业务时区生成 startTime 和 endTime。手动时间参数优先于 range，避免用户传入精确时间时被快捷条件覆盖。SQL 条件仍然通过 pgx 参数化传入，保证安全性。
```

## 明日预告

Day 15 建议做：

```text
GM 操作日志详情接口
```

例如：

```text
GET /api/admin/operation-logs/:id
```

用于查看单条日志的完整信息。
