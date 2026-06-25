# Day 18 学习计划：Dashboard 最近 GM 操作日志接口

## 你今天从任务几开始

你现在从：

```text
任务 0：确认 Day 17 操作日志 Action 选项接口已经完成
```

开始。

Day 18 不改数据库表，不改封禁/解封逻辑，不做前端页面。

今天只做一个给 GM 后台首页使用的“最近操作动态”接口：

```text
GET /api/admin/dashboard/recent-operation-logs
```

## 今日目标

新增 dashboard 最近 GM 操作日志接口。

它返回最近几条 GM 操作日志，例如：

```text
管理员 admin 查询了玩家列表
管理员 admin 查看了玩家详情
管理员 admin 封禁了某个玩家
管理员 admin 解封了某个玩家
```

前端 dashboard 首页以后可以展示：

```text
左侧：统计数字
右侧：最近 GM 操作
```

## 为什么今天做这个

Day 16 已经做了：

```text
GET /api/admin/dashboard/summary
```

它适合展示统计数字。

Day 17 已经做了：

```text
GET /api/admin/operation-log-actions
```

它适合给日志筛选框提供选项。

但一个真实后台首页通常不只有数字，还会有最近动态。

所以 Day 18 做：

```text
dashboard 最近 GM 操作日志接口
```

这能让 GM 后台首页更完整。

## 今日最终效果

请求：

```http
GET /api/admin/dashboard/recent-operation-logs
Authorization: Bearer 管理员token
```

返回：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "items": [
      {
        "id": 12,
        "admin_id": 1,
        "admin_username": "admin",
        "admin_role": "super_admin",
        "action": "admin.players.list",
        "target_type": "player",
        "detail": "page=1,page_size=10",
        "ip": "::1",
        "user_agent": "Apifox/xxx",
        "created_at": "2026-06-18T09:00:00Z"
      }
    ],
    "limit": 10
  }
}
```

也支持指定数量：

```text
GET /api/admin/dashboard/recent-operation-logs?limit=5
```

限制规则：

```text
limit 默认 10
limit 最大 20
limit 传 0、负数、非数字时使用默认值 10
```

## 今日会学到什么

今天会学到：

- dashboard 首页为什么需要“最近动态”。
- 如何复用已有 `model.AdminOperationLog`。
- 如何设计轻量列表接口。
- 为什么 dashboard 最近日志不需要返回分页 total。
- 如何给 `limit` 做默认值和最大值限制。
- 为什么这个接口不应该写入新的操作日志。
- 如何避免“查询操作日志又制造操作日志”的循环。

## 今日文件范围

今天主要修改两个 Go 文件：

```text
E:\game-realtime-gm\backend\internal\handler\admin.go
E:\game-realtime-gm\backend\internal\router\router.go
```

同步更新两个文档：

```text
E:\game-realtime-gm\README.md
E:\game-realtime-gm\docs\api-overview.md
```

不需要改：

```text
E:\game-realtime-gm\backend\internal\database\schema.sql
E:\game-realtime-gm\backend\internal\model\admin_operation_log.go
```

原因：

```text
admin_operation_logs 表已经有足够字段。
model.AdminOperationLog 已经能表示一条日志。
```

## 今日设计

### 接口路径

```text
GET /api/admin/dashboard/recent-operation-logs
```

为什么放在 dashboard 下：

```text
这个接口服务于后台首页，不是完整日志管理页。
```

完整日志管理页继续使用：

```text
GET /api/admin/operation-logs
```

### 和已有日志列表接口的区别

| 接口 | 用途 | 是否分页 | 是否筛选 |
| --- | --- | --- | --- |
| `/api/admin/operation-logs` | 日志管理页 | 是 | 是 |
| `/api/admin/dashboard/recent-operation-logs` | 后台首页最近动态 | 否，只 limit | 否 |

Day 18 的接口只负责：

```text
拿最近 N 条
```

不要把它做成另一个完整日志列表。

### 为什么不记录操作日志

这个接口本身就是查操作日志。

如果每次查询它都写入一条：

```text
admin.dashboard.recent_operation_logs
```

就会出现：

```text
查询最近日志 -> 写入一条新日志 -> 最近日志又变化
```

这会污染审计数据。

所以 Day 18 不记录这个接口本身的操作日志。

## 当前项目已有基础

当前已经有：

```text
model.AdminOperationLog
AdminHandler.ListOperationLogs
AdminHandler.GetOperationLogByID
```

Day 18 可以复用：

```text
model.AdminOperationLog
```

不用新增 model。

## 任务 0：确认 Day 17 状态

### 在哪里执行

PowerShell。

### 执行命令

```powershell
cd E:\game-realtime-gm\backend
go test ./...
```

再检查 Day 17 方法和路由：

```powershell
Select-String -Path E:\game-realtime-gm\backend\internal\handler\admin.go -Pattern "ListOperationLogActions" -SimpleMatch
Select-String -Path E:\game-realtime-gm\backend\internal\router\router.go -Pattern '"/operation-log-actions"' -SimpleMatch
```

如果都能看到输出，再继续 Day 18。

## 任务 1：新增 RecentOperationLogs 方法

### 修改文件

```text
E:\game-realtime-gm\backend\internal\handler\admin.go
```

### 操作类型

局部新增。

### 修改位置

搜索：

```text
func (h *AdminHandler) DashboardSummary
```

建议把新方法放在 `DashboardSummary` 后面、`countOnlinePlayers` 前面。

原因：

```text
DashboardSummary 和 RecentOperationLogs 都是 dashboard 接口。
放在一起方便阅读。
```

### 新增代码

在 `DashboardSummary` 方法结束后的 `}` 后面新增：

```go
func (h *AdminHandler) RecentOperationLogs(c *gin.Context) {
	limit := parsePositiveInt(c.DefaultQuery("limit", "10"), 10)
	if limit > 20 {
		limit = 20
	}

	rows, err := h.db.Query(
		c.Request.Context(),
		`SELECT id, admin_id, admin_username, admin_role, action, target_type, target_id, detail, ip, user_agent, created_at
         FROM admin_operation_logs
         ORDER BY id DESC
         LIMIT $1`,
		int32(limit),
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50101,
			"message": "query recent operation logs failed",
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
				"code":    50102,
				"message": "scan recent operation log failed",
			})
			return
		}
		logs = append(logs, operationLog)
	}

	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50103,
			"message": "read recent operation log rows failed",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "ok",
		"data": gin.H{
			"items": logs,
			"limit": limit,
		},
	})
}
```

### 代码分析

```go
limit := parsePositiveInt(c.DefaultQuery("limit", "10"), 10)
```

表示从 query string 读取 `limit`。

如果用户没有传：

```text
默认 10
```

如果用户传错：

```text
也使用 10
```

```go
if limit > 20 {
	limit = 20
}
```

表示最多返回 20 条。

原因：

```text
dashboard 首页只是展示最近动态，不应该一次返回太多日志。
```

SQL：

```sql
ORDER BY id DESC
LIMIT $1
```

表示按最新日志排在前面，并限制返回数量。

## 任务 2：注册 dashboard 最近日志路由

### 修改文件

```text
E:\game-realtime-gm\backend\internal\router\router.go
```

### 操作类型

局部新增。

### 修改位置

搜索：

```text
adminProtected.GET("/dashboard/summary", adminHandler.DashboardSummary)
```

当前应该能看到：

```go
adminProtected.GET("/me", adminHandler.Me)
adminProtected.GET("/dashboard/summary", adminHandler.DashboardSummary)
adminProtected.GET("/players", adminHandler.ListPlayers)
```

在 dashboard summary 后面新增：

```go
adminProtected.GET("/dashboard/recent-operation-logs", adminHandler.RecentOperationLogs)
```

最终变成：

```go
adminProtected.GET("/me", adminHandler.Me)
adminProtected.GET("/dashboard/summary", adminHandler.DashboardSummary)
adminProtected.GET("/dashboard/recent-operation-logs", adminHandler.RecentOperationLogs)
adminProtected.GET("/players", adminHandler.ListPlayers)
```

### 为什么放在 dashboard/summary 后面

这两个接口都服务于 dashboard 首页：

```text
summary：统计数字
recent-operation-logs：最近动态
```

放在一起更容易维护。

## 任务 3：格式化和编译

### 在哪里执行

PowerShell。

### 执行命令

```powershell
cd E:\game-realtime-gm\backend
gofmt -w .\internal\handler\admin.go .\internal\router\router.go
go test ./...
```

预期：

```text
没有 FAIL
```

如果有错误，不要急着乱改，把完整错误发给我。

## 任务 4：启动后端

如果你用 GoLand，直接运行：

```text
backend/cmd/server
```

如果你用 PowerShell：

```powershell
cd E:\game-realtime-gm\backend
go run .\cmd\server
```

如果 8080 被占用：

```text
listen tcp :8080: bind: Only one usage of each socket address
```

说明可能已经有后端实例在运行。

## 任务 5：准备管理员 token

### Apifox 请求

```http
POST http://localhost:8080/api/admin/login
Content-Type: application/json
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

后续请求加：

```text
Authorization: Bearer 管理员token
```

## 任务 6：先制造几条 GM 操作日志

如果数据库里没有操作日志，最近日志接口会返回空数组，这是正常的。

为了更容易观察，可以先请求：

```http
GET http://localhost:8080/api/admin/players?page=1&page_size=10
Authorization: Bearer 管理员token
```

这个接口会写入：

```text
admin.players.list
```

如果有玩家 id，也可以请求：

```http
GET http://localhost:8080/api/admin/players/1
Authorization: Bearer 管理员token
```

这个接口会写入：

```text
admin.players.detail
```

## 任务 7：测试最近日志接口

### 默认 limit

```http
GET http://localhost:8080/api/admin/dashboard/recent-operation-logs
Authorization: Bearer 管理员token
```

预期：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "items": [],
    "limit": 10
  }
}
```

如果有日志，`items` 会有内容。

### 指定 limit

```http
GET http://localhost:8080/api/admin/dashboard/recent-operation-logs?limit=5
Authorization: Bearer 管理员token
```

预期：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "items": [],
    "limit": 5
  }
}
```

### 超过最大值

```http
GET http://localhost:8080/api/admin/dashboard/recent-operation-logs?limit=100
Authorization: Bearer 管理员token
```

预期：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "items": [],
    "limit": 20
  }
}
```

## 任务 8：测试权限

### 不带 token

```http
GET http://localhost:8080/api/admin/dashboard/recent-operation-logs
```

预期：

```text
返回 401 未登录类错误。
```

### 使用玩家 token

```http
GET http://localhost:8080/api/admin/dashboard/recent-operation-logs
Authorization: Bearer 玩家token
```

预期：

```text
返回 403 无管理员权限类错误。
```

## 任务 9：确认它不会额外写操作日志

### 为什么要确认

这个接口本身就是读取操作日志。

如果它每次查询都写一条新日志，会污染 dashboard 最近动态。

### 验证方式

先查最近日志：

```http
GET http://localhost:8080/api/admin/dashboard/recent-operation-logs?limit=5
Authorization: Bearer 管理员token
```

记住第一条日志的：

```text
id
```

再调用一次同样接口。

预期：

```text
第一条 id 不会因为调用 recent-operation-logs 而变成一条新的“查询最近日志”记录。
```

更直接的数据库验证：

```powershell
docker exec -it game_realtime_postgres psql -U game -d game_realtime
```

执行：

```sql
SELECT id, action, detail, created_at
FROM admin_operation_logs
ORDER BY id DESC
LIMIT 5;
```

你不应该看到因为调用 Day 18 接口而新增的 action。

## 任务 10：用数据库对照验证

进入数据库：

```powershell
docker exec -it game_realtime_postgres psql -U game -d game_realtime
```

执行：

```sql
SELECT id, admin_id, admin_username, admin_role, action, target_type, target_id, detail, ip, user_agent, created_at
FROM admin_operation_logs
ORDER BY id DESC
LIMIT 10;
```

对照：

```text
接口返回的 data.items
数据库查询结果
```

应该是同样顺序：

```text
id 从大到小
```

退出：

```sql
\q
```

## 任务 11：更新接口文档

### 修改文件

```text
E:\game-realtime-gm\docs\api-overview.md
```

### 修改位置

搜索：

```text
### GET /api/admin/dashboard/summary
```

在这个接口后面、`### GET /api/admin/players` 前面新增：

```markdown
### GET /api/admin/dashboard/recent-operation-logs

用途：查询 GM 后台首页最近操作日志。

鉴权：管理员 token。

常用查询参数：

```text
limit=10
```

说明：

```text
limit 默认 10，最大 20。
该接口用于 dashboard 首页最近动态，不额外写入 GM 操作日志。
```
```

如果 Markdown 代码块不好嵌套，可以只写普通文本。

## 任务 12：更新 README 当前功能

### 修改文件

```text
E:\game-realtime-gm\README.md
```

### 修改位置 1：当前进度

搜索：

```text
当前进度：已完成 Day 01 到 Day 15。
```

如果你已经完成 Day 18，可以改成：

```text
当前进度：已完成 Day 01 到 Day 18。
```

### 修改位置 2：GM 管理侧功能列表

搜索：

```text
### GM 管理侧
```

补充这些已经完成的功能：

```text
- GM 后台首页统计
- GM 后台最近操作动态
- GM 操作日志 action 筛选选项
```

### 修改位置 3：核心接口列表

搜索：

```text
核心接口包括：
```

补充：

```text
GET  /api/admin/dashboard/summary
GET  /api/admin/dashboard/recent-operation-logs
GET  /api/admin/operation-log-actions
```

### 修改位置 4：学习过程记录

搜索：

```text
docs/day15-plan.md
```

如果你已经完成 Day 18，可以改成：

```text
docs/day18-plan.md
```

## 今日验收清单

Day 18 完成时，你应该满足：

- `go test ./...` 通过。
- `GET /api/admin/dashboard/recent-operation-logs` 已注册到路由。
- 管理员 token 可以访问。
- 不带 token 不能访问。
- 玩家 token 不能访问。
- 默认 `limit` 返回 10。
- `limit=5` 返回 5。
- `limit=100` 被限制为 20。
- 返回的日志按 `id DESC` 排序。
- 调用该接口不会新增 GM 操作日志。
- 数据库查询结果能和接口返回对上。
- `docs/api-overview.md` 已更新。
- `README.md` 已更新。

## 常见问题

### 1. router.go 报 undefined: adminHandler.RecentOperationLogs

原因：

```text
你注册了路由，但 admin.go 里没有新增 RecentOperationLogs 方法。
```

检查：

```powershell
Select-String -Path E:\game-realtime-gm\backend\internal\handler\admin.go -Pattern "RecentOperationLogs"
```

### 2. 返回 items 一直是空数组

原因：

```text
admin_operation_logs 表里没有数据。
```

解决：

先调用：

```http
GET /api/admin/players?page=1&page_size=10
Authorization: Bearer 管理员token
```

再请求 Day 18 接口。

### 3. limit=100 没有限制成 20

检查代码里是否有：

```go
if limit > 20 {
	limit = 20
}
```

### 4. 调用 recent-operation-logs 后日志 total 增加了

原因：

```text
你可能在 RecentOperationLogs 里调用了 h.recordOperation。
```

Day 18 不应该记录这个查询动作。

## 今日不要做什么

今天不要做：

- dashboard 前端页面。
- 日志图表。
- 最近日志分页。
- 最近日志筛选。
- WebSocket 实时推送日志。
- 操作日志删除。
- 日志导出。

原因：

```text
今天只做 dashboard 首页最近动态的最小后端闭环。
```

## 任务 13：提交并推送到 GitHub

### 先检查状态

```powershell
cd E:\game-realtime-gm
git status
```

确认不要提交这些内部文件：

```text
docs/codex-context.md
docs/conversation-handoff-gpt55.md
docs/mcp-adoption-plan.md
docs/skill-adoption-plan.md
```

### 选择性添加 Day 18 相关文件

如果今天按文档完成了代码和文档修改，执行：

```powershell
git add backend README.md docs/api-overview.md docs/day18-plan.md
```

不要无脑使用：

```powershell
git add .
```

### 提交

```powershell
git commit -m "Complete day18 dashboard recent logs"
```

### 推送

```powershell
git push
```

### 推送后检查

```powershell
git status -sb
```

理想状态：

```text
## main...origin/main
```

如果还看到内部文档未提交，是正常的。

## 明日预告

Day 19 可以开始进入实时服务主线：

```text
WebSocket 最小连接接口
```

例如：

```text
GET /ws
```

先跑通：

```text
客户端连接 -> 后端升级 WebSocket -> 后端返回欢迎消息 -> 客户端断开
```

这会从 GM 后台 HTTP 接口阶段，进入实时游戏服务阶段。
