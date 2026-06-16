# Day 12 学习计划：GM 操作日志查询接口

## 你今天从任务几开始

你现在从：

```text
任务 0：确认 Day 11 已经编译通过
```

开始。

我已经检查到当前后端：

```text
go test ./...
```

是通过的，并且代码里已经有：

```text
BanPlayer
UnbanPlayer
admin.players.ban
admin.players.unban
```

所以 Day 12 可以正式做：

```text
GM 操作日志查询接口
```

## 今日目标

今天新增一个管理员接口：

```text
GET /api/admin/operation-logs
```

它用来查询：

```text
admin_operation_logs
```

也就是 Day 09 到 Day 11 已经写入的 GM 操作日志。

最终 GM 可以通过接口看到：

- 谁操作的。
- 操作了什么。
- 操作对象是什么。
- 操作对象 ID 是多少。
- 操作详情是什么。
- 操作 IP。
- 操作时间。

## 为什么今天做操作日志查询

前几天系统已经会写日志：

```text
admin.players.list
admin.players.detail
admin.players.ban
admin.players.unban
```

但如果只能在 Docker Desktop 里手动执行 SQL 查日志，不像一个真正的 GM 后台能力。

真实 GM 后台需要通过接口查询操作日志，后面前端页面才能展示：

```text
GM 操作日志列表
```

所以 Day 12 做一个查询接口。

## 今日最终效果

管理员请求：

```text
GET http://localhost:8080/api/admin/operation-logs?page=1&page_size=10
```

Headers：

```text
Authorization: Bearer 管理员token
```

成功响应类似：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "items": [
      {
        "id": 4,
        "admin_id": 1,
        "admin_username": "admin",
        "admin_role": "super_admin",
        "action": "admin.players.unban",
        "target_type": "player",
        "target_id": 1,
        "detail": "reason=申诉通过",
        "ip": "::1",
        "user_agent": "Apifox/...",
        "created_at": "2026-06-15T..."
      }
    ],
    "page": 1,
    "page_size": 10,
    "total": 4
  }
}
```

还支持筛选：

```text
GET /api/admin/operation-logs?action=admin.players.ban
GET /api/admin/operation-logs?admin_username=admin
GET /api/admin/operation-logs?target_type=player&target_id=1
```

## 今日会学到什么

今天会学到：

- 如何查询审计日志表。
- 如何给后台列表接口做分页。
- 如何做可选筛选条件。
- 如何动态拼 SQL 但仍然使用参数化查询。
- 为什么不要把密码、token、Authorization header 返回给前端。
- 为什么操作日志查询接口必须挂在管理员鉴权下。

## 今日文件范围

今天建议修改两个文件：

```text
E:\game-realtime-gm\backend\internal\handler\admin.go
E:\game-realtime-gm\backend\internal\router\router.go
```

不需要改：

```text
schema.sql
model/admin_operation_log.go
```

原因：

Day 09 已经建好了：

```text
admin_operation_logs
```

Day 09 也已经有了：

```text
model.AdminOperationLog
```

今天只是新增查询接口。

## 具体文件定位速查

### 1. handler 文件

文件：

```text
E:\game-realtime-gm\backend\internal\handler\admin.go
```

搜索：

```text
func (h *AdminHandler) UnbanPlayer
```

建议把今天新增的：

```text
ListOperationLogs
```

放在 `UnbanPlayer` 方法后面，也就是 `admin.go` 文件底部。

### 2. router 文件

文件：

```text
E:\game-realtime-gm\backend\internal\router\router.go
```

搜索：

```text
adminProtected.POST("/players/:id/unban", adminHandler.UnbanPlayer)
```

在这行下面新增：

```go
adminProtected.GET("/operation-logs", adminHandler.ListOperationLogs)
```

### 3. 判断是否已经添加过

PowerShell 执行：

```powershell
Select-String -Path E:\game-realtime-gm\backend\internal\handler\admin.go -Pattern "ListOperationLogs"
Select-String -Path E:\game-realtime-gm\backend\internal\router\router.go -Pattern "operation-logs"
```

如果没有输出，说明还没加。

## 任务 0：确认 Day 11 状态

### 在哪里执行

PowerShell。

### 执行命令

```powershell
cd E:\game-realtime-gm\backend
go test ./...
```

成功结果类似：

```text
?   	game-realtime-gm/backend/internal/handler	[no test files]
?   	game-realtime-gm/backend/internal/router	[no test files]
```

如果失败，先不要做 Day 12。

把完整错误发给我，我先告诉你原因和解决方法。

## 任务 1：理解今天的查询参数

今天接口：

```text
GET /api/admin/operation-logs
```

支持这些 query 参数：

```text
page
page_size
action
admin_username
target_type
target_id
```

### page

页码。

默认：

```text
1
```

### page_size

每页数量。

默认：

```text
10
```

最大：

```text
50
```

### action

按操作类型筛选。

例如：

```text
admin.players.ban
admin.players.unban
admin.players.list
admin.players.detail
```

### admin_username

按管理员用户名筛选。

例如：

```text
admin
```

### target_type

按操作对象类型筛选。

目前主要是：

```text
player
```

### target_id

按操作对象 ID 筛选。

例如查看某个玩家所有相关操作：

```text
target_type=player&target_id=1
```

## 任务 2：在 admin.go 新增 ListOperationLogs 方法

### 修改文件

```text
E:\game-realtime-gm\backend\internal\handler\admin.go
```

### 具体位置

搜索：

```text
func (h *AdminHandler) UnbanPlayer
```

一直到文件底部。

在最后一个 `}` 后面新增下面方法。

### 新增代码

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

## 任务 3：代码逐段理解

### 1. 分页参数

```go
page := parsePositiveInt(c.DefaultQuery("page", "1"), 1)
pageSize := parsePositiveInt(c.DefaultQuery("page_size", "10"), 10)
```

复用之前的分页工具函数。

如果用户不传，就默认：

```text
page = 1
page_size = 10
```

```go
if pageSize > 50 {
	pageSize = 50
}
```

限制最多查 50 条，避免一次返回太多日志。

### 2. 可选筛选条件

```go
action := strings.TrimSpace(c.Query("action"))
adminUsername := strings.TrimSpace(c.Query("admin_username"))
targetType := strings.TrimSpace(c.Query("target_type"))
targetIDText := strings.TrimSpace(c.Query("target_id"))
```

这几个参数都不是必填。

不传参数时，查询全部日志。

传了参数时，按条件筛选。

### 3. 动态 WHERE

```go
whereParts := make([]string, 0)
args := make([]any, 0)
```

`whereParts` 放 SQL 条件。

`args` 放参数值。

例如传了：

```text
action=admin.players.ban
target_id=1
```

最后会拼成类似：

```sql
WHERE action = $1 AND target_id = $2
```

参数值放在：

```text
args
```

里，不直接拼进 SQL。

### 4. 为什么这样不会 SQL 注入

这里虽然动态拼了 SQL 条件，但拼进去的是我们自己写死的字段名：

```text
action
admin_username
target_type
target_id
```

用户输入的值没有直接拼进 SQL。

用户输入都放进：

```go
args
```

再由 pgx 参数化处理。

这和直接写：

```go
"WHERE action = '" + action + "'"
```

完全不同。

今天不要写这种字符串拼接用户输入的 SQL。

### 5. target_id 单独校验

```go
targetID, err := strconv.ParseInt(targetIDText, 10, 64)
```

因为 `target_id` 是数字。

如果用户传：

```text
target_id=abc
```

返回：

```json
{
  "code": 40071,
  "message": "invalid target id"
}
```

### 6. countSQL

```go
SELECT COUNT(*) FROM admin_operation_logs
```

查询总数。

分页接口需要返回：

```text
total
```

前端以后才能知道总共有多少条日志。

### 7. listSQL

```go
ORDER BY id DESC
```

让最新操作排在最前面。

```go
LIMIT ... OFFSET ...
```

实现分页。

### 8. 返回数据

```go
logs := make([]model.AdminOperationLog, 0)
```

使用 Day 09 已经定义好的模型：

```text
model.AdminOperationLog
```

## 任务 4：注册操作日志查询路由

### 修改文件

```text
E:\game-realtime-gm\backend\internal\router\router.go
```

### 具体位置

搜索：

```text
adminProtected.POST("/players/:id/unban", adminHandler.UnbanPlayer)
```

在它下面新增：

```go
adminProtected.GET("/operation-logs", adminHandler.ListOperationLogs)
```

### 修改后应该是

```go
adminProtected.GET("/me", adminHandler.Me)
adminProtected.GET("/players", adminHandler.ListPlayers)
adminProtected.GET("/players/:id", adminHandler.GetPlayerByID)
adminProtected.POST("/players/:id/ban", adminHandler.BanPlayer)
adminProtected.POST("/players/:id/unban", adminHandler.UnbanPlayer)
adminProtected.GET("/operation-logs", adminHandler.ListOperationLogs)
```

### 为什么放在 adminProtected 下面

操作日志属于后台审计数据。

不能让普通玩家查看。

所以必须挂在：

```text
middleware.AdminAuth
```

之后。

也就是：

```text
adminProtected
```

下面。

## 任务 5：格式化并编译

### 在哪里执行

PowerShell。

### 执行命令

```powershell
cd E:\game-realtime-gm\backend
gofmt -w .\internal\handler\admin.go .\internal\router\router.go
go test ./...
```

### 成功结果

看到：

```text
?   	game-realtime-gm/backend/internal/handler	[no test files]
?   	game-realtime-gm/backend/internal/router	[no test files]
```

### 常见错误

#### 1. `undefined: model.AdminOperationLog`

原因：

`model/admin_operation_log.go` 不存在，或者结构体名字写错。

检查文件：

```text
E:\game-realtime-gm\backend\internal\model\admin_operation_log.go
```

里面应该有：

```go
type AdminOperationLog struct
```

#### 2. `undefined: ListOperationLogs`

原因：

`router.go` 注册了：

```go
adminHandler.ListOperationLogs
```

但 `admin.go` 里没有新增方法。

#### 3. `syntax error: unexpected name`

原因：

大概率是方法粘贴到了另一个函数内部。

检查 `ListOperationLogs` 是否放在 `admin.go` 文件底部，且前一个方法已经用 `}` 正确结束。

#### 4. `imported and not used`

原因：

你加了 import 但代码没用。

当前 Day 12 不需要新增 import，因为 `admin.go` 已经有：

```go
net/http
strconv
strings
model
```

这些都已经在使用。

## 任务 6：启动后端

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

找到旧的 PowerShell 或 GoLand 运行窗口，按：

```text
Ctrl + C
```

再重新启动。

## 任务 7：用 Apifox 查询全部操作日志

### 第 1 步：登录管理员

Apifox 请求：

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

复制响应中的：

```text
data.token
```

### 第 2 步：查询操作日志

Apifox 请求：

```text
GET http://localhost:8080/api/admin/operation-logs?page=1&page_size=10
```

Headers：

```text
Authorization: Bearer 管理员token
```

成功响应应该包含：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "items": [],
    "page": 1,
    "page_size": 10,
    "total": 0
  }
}
```

如果你之前已经查询、封禁、解封过玩家，`items` 里应该有日志。

## 任务 8：测试 action 筛选

请求：

```text
GET http://localhost:8080/api/admin/operation-logs?action=admin.players.ban
```

Headers：

```text
Authorization: Bearer 管理员token
```

预期：

只返回：

```text
action = admin.players.ban
```

的日志。

再试：

```text
GET http://localhost:8080/api/admin/operation-logs?action=admin.players.unban
```

预期只返回解封日志。

## 任务 9：测试 target_id 筛选

假设玩家 ID 是 `1`。

请求：

```text
GET http://localhost:8080/api/admin/operation-logs?target_type=player&target_id=1
```

预期返回和玩家 1 相关的日志，例如：

```text
admin.players.detail
admin.players.ban
admin.players.unban
```

如果你请求：

```text
GET http://localhost:8080/api/admin/operation-logs?target_id=abc
```

预期：

```json
{
  "code": 40071,
  "message": "invalid target id"
}
```

## 任务 10：测试分页上限

请求：

```text
GET http://localhost:8080/api/admin/operation-logs?page=1&page_size=999
```

预期响应里：

```json
"page_size": 50
```

说明最大分页限制生效。

## 任务 11：测试权限隔离

### 不带 token

请求：

```text
GET http://localhost:8080/api/admin/operation-logs
```

不带 Authorization。

预期：

```json
{
  "code": 40112,
  "message": "authorization header missing"
}
```

### 用玩家 token

先登录普通玩家：

```text
POST http://localhost:8080/api/login
```

复制玩家 token。

再请求：

```text
GET http://localhost:8080/api/admin/operation-logs
```

Headers：

```text
Authorization: Bearer 玩家token
```

预期：

```json
{
  "code": 40311,
  "message": "admin permission required"
}
```

### 用管理员 token

Headers：

```text
Authorization: Bearer 管理员token
```

预期成功。

这说明操作日志查询接口只允许管理员访问。

## 任务 12：用 PostgreSQL 对照验证

Docker Desktop 进入 PostgreSQL：

```bash
psql -U game -d game_realtime
```

执行：

```sql
SELECT id, admin_username, action, target_type, target_id, detail, created_at
FROM admin_operation_logs
ORDER BY id DESC
LIMIT 10;
```

你在数据库里看到的内容，应该能在：

```text
GET /api/admin/operation-logs?page=1&page_size=10
```

接口响应里看到。

如果 `psql` 底部出现：

```text
(END)
```

按：

```text
q
```

退出分页界面。

## 常见问题

### 1. 返回 404

原因：

路由没有注册。

检查：

```text
E:\game-realtime-gm\backend\internal\router\router.go
```

是否有：

```go
adminProtected.GET("/operation-logs", adminHandler.ListOperationLogs)
```

### 2. 返回 401

原因：

没带管理员 token。

Headers 要写：

```text
Authorization: Bearer 管理员token
```

### 3. 返回 403

原因：

你带的是玩家 token，不是管理员 token。

重新请求：

```text
POST /api/admin/login
```

拿管理员 token。

### 4. 返回空列表

可能不是错误。

原因可能是：

- 你还没有触发过任何 GM 操作日志。
- 筛选条件太严格。
- `admin_operation_logs` 表确实没有数据。

可以在 PostgreSQL 执行：

```sql
SELECT COUNT(*) FROM admin_operation_logs;
```

确认表里有没有日志。

### 5. target_id 筛选报错

如果你传：

```text
target_id=abc
```

会返回：

```text
invalid target id
```

这是正确行为。

`target_id` 必须是正整数。

## 今日验收清单

- [ ] `go test ./...` 通过。
- [ ] `admin.go` 已新增 `ListOperationLogs`。
- [ ] `router.go` 已注册 `GET /api/admin/operation-logs`。
- [ ] 管理员 token 可以查询操作日志列表。
- [ ] 不带 token 查询返回 401。
- [ ] 玩家 token 查询返回 403。
- [ ] `page_size=999` 会被限制为 50。
- [ ] `action=admin.players.ban` 可以筛选封禁日志。
- [ ] `target_type=player&target_id=1` 可以筛选指定玩家日志。
- [ ] `target_id=abc` 返回 40071。
- [ ] 接口响应不包含密码、password_hash、JWT token、Authorization header。

## 今日不要做

今天不要做：

- 操作日志删除。
- 操作日志修改。
- 导出 Excel。
- 前端日志页面。
- 高级时间范围筛选。
- 管理员角色权限细分。
- 日志脱敏策略配置。

原因：

今天只完成：

```text
管理员查询操作日志列表
```

这个基础闭环。

## 请求链路复盘

```text
Apifox
  ↓
GET /api/admin/operation-logs
  ↓
router.go
  ↓
middleware.AdminAuth
  ↓
AdminHandler.ListOperationLogs
  ↓
解析 page/page_size/action/target_id 等参数
  ↓
SELECT COUNT(*) FROM admin_operation_logs
  ↓
SELECT ... FROM admin_operation_logs ORDER BY id DESC LIMIT/OFFSET
  ↓
JSON response
```

## 面试怎么讲这一部分

可以这样说：

```text
在 GM 查询、封禁、解封功能都能写入操作日志后，我实现了 GM 操作日志查询接口。接口为 GET /api/admin/operation-logs，挂在 AdminAuth 管理员鉴权后面。它支持 page、page_size 分页，也支持按 action、admin_username、target_type、target_id 筛选。实现时动态拼接的是固定字段条件，用户输入仍然通过 pgx 参数化传入，避免 SQL 注入。接口返回 admin_operation_logs 中的审计信息，但不返回密码、JWT token 或 Authorization header 等敏感内容。
```

## 明日预告

Day 13 建议做：

```text
GM 操作日志按时间范围筛选
```

原因：

Day 12 已经能查询操作日志。

真实后台查日志时经常需要：

```text
今天
最近 7 天
某个时间段
```

所以 Day 13 可以补：

```text
start_time
end_time
```

两个查询参数。
