# Day 13 学习计划：GM 操作日志按时间范围筛选

## 你今天从任务几开始

你现在从：

```text
任务 0：确认 Day 12 操作日志查询接口已经存在
```

开始。

我已经检查到当前项目里有：

```text
GET /api/admin/operation-logs
AdminHandler.ListOperationLogs
```

并且：

```text
go test ./...
```

当前是通过的。

所以 Day 13 不需要新建接口，而是在 Day 12 的日志查询接口上增强两个参数：

```text
start_time
end_time
```

## 今日目标

今天给 GM 操作日志查询接口增加时间范围筛选：

```text
GET /api/admin/operation-logs?start_time=2026-06-16T00:00:00+08:00&end_time=2026-06-16T23:59:59+08:00
```

最终效果：

```text
GM 可以查询某个时间段内的操作日志。
```

例如：

- 查询今天的日志。
- 查询最近一次测试期间的日志。
- 查询某个封禁问题发生前后的日志。

## 为什么今天做时间筛选

Day 12 已经可以查询操作日志，并支持：

```text
action
admin_username
target_type
target_id
```

但真实后台查日志时，最常用的条件通常是：

```text
时间范围
```

比如：

```text
今天谁封禁了玩家？
昨晚 8 点到 10 点发生了什么操作？
某个玩家被封禁前后有哪些 GM 操作？
```

所以今天补：

```text
start_time
end_time
```

让日志查询更接近真实 GM 后台。

## 今日最终效果

查询全部日志：

```text
GET http://localhost:8080/api/admin/operation-logs?page=1&page_size=10
```

查询某一天日志：

```text
GET http://localhost:8080/api/admin/operation-logs?start_time=2026-06-16T00:00:00+08:00&end_time=2026-06-16T23:59:59+08:00
```

查询某个玩家在某个时间段的日志：

```text
GET http://localhost:8080/api/admin/operation-logs?target_type=player&target_id=1&start_time=2026-06-16T00:00:00+08:00&end_time=2026-06-16T23:59:59+08:00
```

如果时间格式错误：

```text
GET /api/admin/operation-logs?start_time=abc
```

返回：

```json
{
  "code": 40072,
  "message": "invalid start_time"
}
```

如果开始时间晚于结束时间：

```text
start_time=2026-06-16T23:59:59+08:00
end_time=2026-06-16T00:00:00+08:00
```

返回：

```json
{
  "code": 40074,
  "message": "start_time cannot be after end_time"
}
```

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

Day 12 已经注册了：

```text
GET /api/admin/operation-logs
```

今天只是给这个已有接口加筛选参数。

## 具体文件定位

打开：

```text
E:\game-realtime-gm\backend\internal\handler\admin.go
```

搜索：

```text
func (h *AdminHandler) ListOperationLogs
```

你会看到类似：

```go
func (h *AdminHandler) ListOperationLogs(c *gin.Context) {
	page := parsePositiveInt(c.DefaultQuery("page", "1"), 1)
	pageSize := parsePositiveInt(c.DefaultQuery("page_size", "10"), 10)
```

继续往下找这几行：

```go
	action := strings.TrimSpace(c.Query("action"))
	adminUsername := strings.TrimSpace(c.Query("admin_username"))
	targetType := strings.TrimSpace(c.Query("target_type"))
	targetIDText := strings.TrimSpace(c.Query("target_id"))
```

Day 13 的新参数就加在这里。

## 任务 0：确认 Day 12 状态

### 在哪里执行

PowerShell。

### 执行命令

```powershell
cd E:\game-realtime-gm\backend
go test ./...
```

成功结果：

```text
?   	game-realtime-gm/backend/internal/handler	[no test files]
?   	game-realtime-gm/backend/internal/router	[no test files]
```

再确认代码存在：

```powershell
Select-String -Path E:\game-realtime-gm\backend\internal\handler\admin.go -Pattern "ListOperationLogs"
Select-String -Path E:\game-realtime-gm\backend\internal\router\router.go -Pattern "operation-logs"
```

两条都有输出，再继续。

## 任务 1：给 admin.go 增加 time import

### 为什么要加

今天要解析：

```text
2026-06-16T00:00:00+08:00
```

这种时间字符串。

Go 里用：

```go
time.Parse(time.RFC3339, value)
```

所以需要 import：

```go
"time"
```

### 修改位置

打开：

```text
E:\game-realtime-gm\backend\internal\handler\admin.go
```

文件开头应该类似：

```go
import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
```

改成：

```go
import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
```

如果你后面执行 `gofmt`，它会自动整理 import 顺序。

## 任务 2：读取 start_time 和 end_time 参数

### 修改位置

在 `ListOperationLogs` 方法里找到：

```go
	action := strings.TrimSpace(c.Query("action"))
	adminUsername := strings.TrimSpace(c.Query("admin_username"))
	targetType := strings.TrimSpace(c.Query("target_type"))
	targetIDText := strings.TrimSpace(c.Query("target_id"))
```

改成：

```go
	action := strings.TrimSpace(c.Query("action"))
	adminUsername := strings.TrimSpace(c.Query("admin_username"))
	targetType := strings.TrimSpace(c.Query("target_type"))
	targetIDText := strings.TrimSpace(c.Query("target_id"))
	startTimeText := strings.TrimSpace(c.Query("start_time"))
	endTimeText := strings.TrimSpace(c.Query("end_time"))
```

### 参数含义

`start_time`：

```text
开始时间。
```

`end_time`：

```text
结束时间。
```

今天要求使用 RFC3339 格式，例如：

```text
2026-06-16T00:00:00+08:00
2026-06-16T23:59:59+08:00
```

注意中间有：

```text
T
```

最后有：

```text
+08:00
```

表示中国时区。

## 任务 3：在 where 条件里加入时间筛选

### 修改位置

在 `ListOperationLogs` 方法里找到这一段：

```go
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
```

在这段后面、`whereSQL := ""` 前面，插入下面代码：

```go
	var startTime time.Time
	var endTime time.Time

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
```

### 插入后的结构应该像这样

```go
	if targetIDText != "" {
		...
	}

	var startTime time.Time
	var endTime time.Time

	if startTimeText != "" {
		...
	}

	if endTimeText != "" {
		...
	}

	if !startTime.IsZero() && !endTime.IsZero() && startTime.After(endTime) {
		...
	}

	whereSQL := ""
	if len(whereParts) > 0 {
		whereSQL = "WHERE " + strings.Join(whereParts, " AND ")
	}
```

## 任务 4：代码解释

### 为什么用 time.RFC3339

RFC3339 是接口里常用的时间格式。

例如：

```text
2026-06-16T00:00:00+08:00
```

它包含：

```text
日期
时间
时区
```

PostgreSQL 的 `TIMESTAMPTZ` 字段也能很好地处理带时区的时间。

### 为什么不要用简单日期

比如：

```text
2026-06-16
```

看起来更短，但含义不够明确：

```text
是 0 点？
是本地时区？
是 UTC？
结束时间包不包含当天？
```

初期学习阶段直接用完整时间，最不容易混乱。

### 为什么 start_time 用 >=

```sql
created_at >= start_time
```

表示包含开始时间。

### 为什么 end_time 用 <=

```sql
created_at <= end_time
```

表示包含结束时间。

今天为了直观，先用 `<=`。

后续如果做更严谨的时间区间，可以改成：

```text
左闭右开
created_at >= start_time AND created_at < end_time
```

### 为什么要检查 startTime.After(endTime)

如果用户传：

```text
start_time 比 end_time 更晚
```

这个查询没有业务意义。

所以直接返回：

```text
40074 start_time cannot be after end_time
```

### 是否会 SQL 注入

不会。

因为用户传入的时间字符串先经过：

```go
time.Parse
```

解析成 `time.Time`。

最终也不是拼进 SQL 字符串，而是放进：

```go
args
```

由 pgx 参数化传入。

拼进 SQL 的只有固定字段名：

```text
created_at >= $1
created_at <= $2
```

## 任务 5：完整 ListOperationLogs 对照版

如果你担心插错位置，可以只替换 `ListOperationLogs` 这一个方法。

### 修改文件

```text
E:\game-realtime-gm\backend\internal\handler\admin.go
```

### 替换范围

从：

```go
func (h *AdminHandler) ListOperationLogs(c *gin.Context) {
```

开始。

一直替换到这个方法自己的最后一个：

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

### 在哪里执行

PowerShell。

### 执行命令

```powershell
cd E:\game-realtime-gm\backend
gofmt -w .\internal\handler\admin.go
go test ./...
```

### 成功结果

看到：

```text
?   	game-realtime-gm/backend/internal/handler	[no test files]
?   	game-realtime-gm/backend/internal/router	[no test files]
```

### 常见错误

#### 1. `undefined: time`

原因：

你用了：

```go
time.Parse
```

但没有在 import 里加：

```go
"time"
```

#### 2. `imported and not used: "time"`

原因：

你加了 import，但还没有成功把时间解析代码加进方法里。

解决：

确认 `ListOperationLogs` 里有：

```go
time.Parse(time.RFC3339, ...)
```

#### 3. `syntax error`

原因：

大概率是你把时间筛选代码插进了 `if targetIDText != ""` 的大括号里面，或者替换方法时少复制了最后的 `}`。

解决：

确认结构是：

```go
if targetIDText != "" {
	...
}

var startTime time.Time
...

whereSQL := ""
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

如果 8080 被占用：

```text
listen tcp :8080: bind: Only one usage of each socket address
```

说明旧服务还在运行。

找到旧 PowerShell 或 GoLand 运行窗口，按：

```text
Ctrl + C
```

再重新启动。

## 任务 8：准备时间参数

### 推荐格式

今天使用：

```text
2026-06-16T00:00:00+08:00
```

注意：

如果你直接把 `+08:00` 放到 URL 里，部分客户端可能把 `+` 当成空格。

所以在 Apifox 里推荐用 Query 参数表单填：

```text
start_time = 2026-06-16T00:00:00+08:00
end_time   = 2026-06-16T23:59:59+08:00
```

如果你手写完整 URL，建议把 `+` 编码成：

```text
%2B
```

例如：

```text
GET http://localhost:8080/api/admin/operation-logs?start_time=2026-06-16T00:00:00%2B08:00&end_time=2026-06-16T23:59:59%2B08:00
```

## 任务 9：用 Apifox 查询今天的日志

### 第 1 步：登录管理员

请求：

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

复制：

```text
data.token
```

### 第 2 步：查询今天的日志

Apifox 请求：

```text
GET http://localhost:8080/api/admin/operation-logs
```

Headers：

```text
Authorization: Bearer 管理员token
```

Query 参数：

```text
page = 1
page_size = 10
start_time = 2026-06-16T00:00:00+08:00
end_time = 2026-06-16T23:59:59+08:00
```

预期：

```json
{
  "code": 0,
  "message": "ok"
}
```

如果今天有操作日志，`items` 里会有数据。

如果没有，`items` 是空数组，这不一定是错误。

## 任务 10：测试错误时间格式

请求：

```text
GET http://localhost:8080/api/admin/operation-logs?start_time=abc
```

Headers：

```text
Authorization: Bearer 管理员token
```

预期：

```json
{
  "code": 40072,
  "message": "invalid start_time"
}
```

再测试：

```text
GET http://localhost:8080/api/admin/operation-logs?end_time=abc
```

预期：

```json
{
  "code": 40073,
  "message": "invalid end_time"
}
```

## 任务 11：测试开始时间晚于结束时间

请求：

```text
GET http://localhost:8080/api/admin/operation-logs?start_time=2026-06-16T23:59:59%2B08:00&end_time=2026-06-16T00:00:00%2B08:00
```

预期：

```json
{
  "code": 40074,
  "message": "start_time cannot be after end_time"
}
```

## 任务 12：组合筛选

查询今天某个玩家的封禁日志：

```text
GET /api/admin/operation-logs?action=admin.players.ban&target_type=player&target_id=1&start_time=2026-06-16T00:00:00%2B08:00&end_time=2026-06-16T23:59:59%2B08:00
```

预期只返回同时满足这些条件的日志：

```text
action = admin.players.ban
target_type = player
target_id = 1
created_at 在指定时间范围内
```

## 任务 13：用 PostgreSQL 对照验证

Docker Desktop 进入 PostgreSQL：

```bash
psql -U game -d game_realtime
```

执行：

```sql
SELECT id, admin_username, action, target_type, target_id, detail, created_at
FROM admin_operation_logs
WHERE created_at >= '2026-06-16T00:00:00+08:00'
  AND created_at <= '2026-06-16T23:59:59+08:00'
ORDER BY id DESC
LIMIT 10;
```

你在数据库里看到的结果，应该能在接口里查到。

如果 `psql` 底部显示：

```text
(END)
```

按：

```text
q
```

退出分页界面。

## 常见问题

### 1. 明明有日志，接口查不到

优先检查时间范围。

PostgreSQL 里的 `created_at` 可能显示：

```text
2026-06-16 06:xx:xx+00
```

这是 UTC。

中国时间是 UTC+8。

例如：

```text
2026-06-16 06:00:00+00
= 2026-06-16 14:00:00+08:00
```

所以你用 `+08:00` 查询是可以的，PostgreSQL 会换算。

### 2. start_time 带 `+08:00` 后返回 invalid start_time

原因：

URL 里的 `+` 可能被解析成空格。

解决：

在 Apifox 的 Query 表格里填参数，不要直接拼 URL。

或者把 `+` 写成：

```text
%2B
```

### 3. page_size 仍然最大 50 吗

是。

Day 13 没有改变分页逻辑。

### 4. 权限规则有变化吗

没有。

这个接口仍然在：

```text
adminProtected
```

下面。

只有管理员 token 能访问。

## 今日验收清单

- [ ] `admin.go` 已 import `time`。
- [ ] `ListOperationLogs` 已读取 `start_time` 和 `end_time`。
- [ ] `start_time` 格式错误返回 40072。
- [ ] `end_time` 格式错误返回 40073。
- [ ] `start_time > end_time` 返回 40074。
- [ ] 时间范围筛选能正常返回数据。
- [ ] 时间范围可以和 `action`、`target_id` 组合筛选。
- [ ] `go test ./...` 通过。
- [ ] 管理员 token 可以查询。
- [ ] 玩家 token 和无 token 仍然不能查询。
- [ ] 接口响应不包含密码、password_hash、JWT token、Authorization header。

## 今日不要做

今天不要做：

- 前端日期选择器。
- 最近 7 天快捷按钮。
- 日志导出 Excel。
- 日志删除。
- 日志归档。
- 管理员角色权限细分。

原因：

今天只把后端接口的时间范围筛选做扎实。

## 请求链路复盘

```text
Apifox
  ↓
GET /api/admin/operation-logs?start_time=...&end_time=...
  ↓
router.go
  ↓
middleware.AdminAuth
  ↓
AdminHandler.ListOperationLogs
  ↓
time.Parse(time.RFC3339, start_time)
  ↓
time.Parse(time.RFC3339, end_time)
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
在 GM 操作日志查询接口的基础上，我继续增加了时间范围筛选。接口支持 start_time 和 end_time 两个 RFC3339 格式参数，例如 2026-06-16T00:00:00+08:00。后端使用 time.Parse 校验格式，并检查开始时间不能晚于结束时间。SQL 条件使用 created_at >= $n 和 created_at <= $n，参数通过 pgx 传入，避免 SQL 注入。这个功能让 GM 能按时间段审计封禁、解封、查询玩家等操作。
```

## 明日预告

Day 14 建议做：

```text
GM 操作日志快捷时间范围
```

例如支持：

```text
range=today
range=last_7_days
range=last_30_days
```

这样前端以后可以更方便地做快捷筛选。
