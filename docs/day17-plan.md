# Day 17 学习计划：GM 操作日志 Action 选项接口

## 你今天从任务几开始

你现在从：

```text
任务 0：确认 Day 16 dashboard summary 已经完成并且 go test 通过
```

开始。

Day 17 不改数据库表，不改封禁逻辑，不改 dashboard 统计逻辑。

今天只做一个给前端筛选框用的小接口：

```text
GET /api/admin/operation-log-actions
```

## 今日目标

新增 GM 操作日志 action 选项接口。

接口返回当前系统支持的 GM 操作类型，例如：

```text
admin.players.list
admin.players.detail
admin.players.ban
admin.players.unban
```

前端以后做 GM 操作日志筛选时，可以用这个接口生成下拉框。

## 为什么今天做这个

Day 12 到 Day 15 已经完成了操作日志主线：

```text
记录日志
查询日志列表
按 action 筛选
按时间筛选
查看日志详情
```

但是前端如果要做筛选框，现在还不知道有哪些 action 可以选。

如果让前端手写：

```text
admin.players.list
admin.players.detail
admin.players.ban
admin.players.unban
```

会有两个问题：

1. 后端以后新增 action，前端容易忘记同步。
2. action 是后端业务语义，最好由后端统一告诉前端。

所以 Day 17 做一个 action 选项接口。

## 今日最终效果

请求：

```http
GET /api/admin/operation-log-actions
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
        "value": "admin.players.list",
        "label": "查询玩家列表"
      },
      {
        "value": "admin.players.detail",
        "label": "查看玩家详情"
      },
      {
        "value": "admin.players.ban",
        "label": "封禁玩家"
      },
      {
        "value": "admin.players.unban",
        "label": "解封玩家"
      }
    ]
  }
}
```

不带 token：

```text
返回 401 未登录类错误。
```

使用玩家 token：

```text
返回 403 无管理员权限类错误。
```

## 今日会学到什么

今天会学到：

- 什么是“选项接口”。
- 为什么前端下拉框不应该长期写死业务枚举。
- 如何设计 `{ value, label }` 这种前端友好的返回格式。
- 为什么 Day 17 先用固定 action 列表，而不是直接从数据库动态查。
- 如何给已有业务补一个轻量辅助接口。
- 如何让接口文档和 README 跟着代码同步。

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
```

原因：

```text
action 只是已有 admin_operation_logs.action 字段的业务枚举说明。
今天不新增表、不新增字段。
```

## 今日设计

### 接口路径

```text
GET /api/admin/operation-log-actions
```

为什么不用：

```text
GET /api/admin/operation-logs/actions
```

也可以这样设计，但今天选择更直观的：

```text
/operation-log-actions
```

因为它不是查询某一条日志，也不是日志列表的子资源，而是“操作日志筛选选项”。

### 返回格式

返回：

```json
{
  "items": [
    {
      "value": "admin.players.list",
      "label": "查询玩家列表"
    }
  ]
}
```

`value`：

```text
真正传给 /api/admin/operation-logs?action=xxx 的值。
```

`label`：

```text
前端下拉框展示给 GM 看的中文名称。
```

### 为什么先固定列表

今天不从数据库执行：

```sql
SELECT DISTINCT action FROM admin_operation_logs
```

原因是：

```text
数据库里只会出现“已经发生过”的 action。
如果某个操作还没发生，前端就看不到这个筛选项。
```

例如刚初始化数据库时，可能还没有：

```text
admin.players.ban
admin.players.unban
```

但前端筛选框仍然应该知道这两个选项存在。

所以 Day 17 先用后端固定列表。

后期可以做增强：

```text
固定 action 注册表 + 数据库实际数量统计
```

## 当前项目已有 action

当前 `admin.go` 里已经记录了这些 action：

```text
admin.players.list
admin.players.detail
admin.players.ban
admin.players.unban
```

它们分别来自：

```text
ListPlayers
GetPlayerByID
BanPlayer
UnbanPlayer
```

## 任务 0：确认 Day 16 状态

### 在哪里执行

PowerShell。

### 执行命令

```powershell
cd E:\game-realtime-gm\backend
go test ./...
```

再检查 Day 16 路由：

```powershell
Select-String -Path E:\game-realtime-gm\backend\internal\router\router.go -Pattern '"/dashboard/summary"' -SimpleMatch
```

如果 `go test ./...` 通过，并且能看到 dashboard 路由，再继续 Day 17。

## 任务 1：新增 action 选项结构体

### 修改文件

```text
E:\game-realtime-gm\backend\internal\handler\admin.go
```

### 操作类型

局部新增。

### 修改位置

搜索：

```text
type unbanPlayerRequest struct
```

你现在应该能看到：

```go
type unbanPlayerRequest struct {
	Reason string `json:"reason" binding:"required"`
}
```

在它后面新增：

```go
type operationLogActionOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}
```

### 代码分析

这个结构体表示前端下拉框里的一个选项。

```go
Value string `json:"value"`
```

表示真正传给接口的值，例如：

```text
admin.players.ban
```

```go
Label string `json:"label"`
```

表示给 GM 看的中文名称，例如：

```text
封禁玩家
```

## 任务 2：新增 action 选项列表

### 修改文件

```text
E:\game-realtime-gm\backend\internal\handler\admin.go
```

### 操作类型

局部新增。

### 修改位置

放在刚才新增的：

```go
type operationLogActionOption struct
```

后面。

新增：

```go
var operationLogActionOptions = []operationLogActionOption{
	{
		Value: "admin.players.list",
		Label: "查询玩家列表",
	},
	{
		Value: "admin.players.detail",
		Label: "查看玩家详情",
	},
	{
		Value: "admin.players.ban",
		Label: "封禁玩家",
	},
	{
		Value: "admin.players.unban",
		Label: "解封玩家",
	},
}
```

### 代码分析

这是一个固定列表。

它的好处是：

```text
数据库里暂时没有某种 action 时，前端也能显示这个选项。
```

以后新增 GM 操作时，例如：

```text
admin.items.grant
admin.mail.send
```

就可以继续往这个列表里加。

## 任务 3：新增 ListOperationLogActions 方法

### 修改文件

```text
E:\game-realtime-gm\backend\internal\handler\admin.go
```

### 修改位置

搜索：

```text
func (h *AdminHandler) ListOperationLogs
```

建议把新方法放在 `ListOperationLogs` 前面。

原因：

```text
ListOperationLogActions 是日志模块的辅助接口。
放在日志列表接口前面，阅读顺序比较自然。
```

### 操作类型

在 `ListOperationLogs` 方法前新增方法。

新增代码：

```go
func (h *AdminHandler) ListOperationLogActions(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "ok",
		"data": gin.H{
			"items": operationLogActionOptions,
		},
	})
}
```

### 代码分析

这个接口不查数据库。

原因是：

```text
它返回的是“系统支持哪些操作类型”，不是“数据库里已经出现过哪些操作类型”。
```

它也不写操作日志。

原因是：

```text
查询筛选选项是后台页面辅助能力，不属于重要 GM 操作。
否则前端每打开一次筛选框都会写一条日志，日志会变得很吵。
```

## 任务 4：注册路由

### 修改文件

```text
E:\game-realtime-gm\backend\internal\router\router.go
```

### 修改位置

搜索：

```text
adminProtected.GET("/operation-logs", adminHandler.ListOperationLogs)
```

当前应该能看到：

```go
adminProtected.GET("/operation-logs", adminHandler.ListOperationLogs)
adminProtected.GET("/operation-logs/:id", adminHandler.GetOperationLogByID)
```

在它们前面新增：

```go
adminProtected.GET("/operation-log-actions", adminHandler.ListOperationLogActions)
```

最终变成：

```go
adminProtected.GET("/operation-log-actions", adminHandler.ListOperationLogActions)
adminProtected.GET("/operation-logs", adminHandler.ListOperationLogs)
adminProtected.GET("/operation-logs/:id", adminHandler.GetOperationLogByID)
```

### 为什么放在 adminProtected 下

这个接口虽然只是选项列表，但它服务于 GM 后台操作日志筛选。

普通玩家不应该访问：

```text
GM 操作类型
GM 操作日志筛选项
```

所以它仍然放在：

```text
adminProtected
```

下面。

## 任务 5：格式化和编译

### 在哪里执行

PowerShell。

### 执行命令

```powershell
cd E:\game-realtime-gm\backend
gofmt -w .\internal\handler\admin.go .\internal\router\router.go
go test ./...
```

### 命令解释

`gofmt`：

```text
格式化 Go 代码。
```

`go test ./...`：

```text
编译并测试 backend 下所有包。
```

预期：

```text
没有 FAIL。
```

## 任务 6：启动后端

如果你用 GoLand，直接运行 `backend/cmd/server`。

如果你用 PowerShell：

```powershell
cd E:\game-realtime-gm\backend
go run .\cmd\server
```

如果出现：

```text
listen tcp :8080: bind: Only one usage of each socket address
```

说明 8080 已经有服务在运行。

## 任务 7：准备管理员 token

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

## 任务 8：测试 action 选项接口

### Apifox 请求

```http
GET http://localhost:8080/api/admin/operation-log-actions
Authorization: Bearer 管理员token
```

预期返回：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "items": [
      {
        "value": "admin.players.list",
        "label": "查询玩家列表"
      },
      {
        "value": "admin.players.detail",
        "label": "查看玩家详情"
      },
      {
        "value": "admin.players.ban",
        "label": "封禁玩家"
      },
      {
        "value": "admin.players.unban",
        "label": "解封玩家"
      }
    ]
  }
}
```

## 任务 9：测试它能配合日志列表筛选使用

### 先复制一个 value

例如：

```text
admin.players.list
```

### 请求日志列表

```http
GET http://localhost:8080/api/admin/operation-logs?action=admin.players.list&page=1&page_size=10
Authorization: Bearer 管理员token
```

预期：

```text
只返回 action 为 admin.players.list 的日志。
```

如果没有数据，先调用一次：

```http
GET http://localhost:8080/api/admin/players?page=1&page_size=10
Authorization: Bearer 管理员token
```

这个接口会产生：

```text
admin.players.list
```

日志。

## 任务 10：测试权限

### 不带 token

```http
GET http://localhost:8080/api/admin/operation-log-actions
```

预期：

```text
返回 401 未登录类错误。
```

### 使用玩家 token

```http
GET http://localhost:8080/api/admin/operation-log-actions
Authorization: Bearer 玩家token
```

预期：

```text
返回 403 无管理员权限类错误。
```

## 任务 11：确认不会额外写操作日志

### 为什么要确认

Day 17 的接口只是给前端拿筛选选项。

它不应该每请求一次就写一条操作日志。

否则前端打开筛选框、刷新页面都会产生大量无意义日志。

### 验证方式

先查日志数量：

```http
GET http://localhost:8080/api/admin/operation-logs?page=1&page_size=10
Authorization: Bearer 管理员token
```

记住返回里的：

```text
data.total
```

然后调用：

```http
GET http://localhost:8080/api/admin/operation-log-actions
Authorization: Bearer 管理员token
```

再查一次：

```http
GET http://localhost:8080/api/admin/operation-logs?page=1&page_size=10
Authorization: Bearer 管理员token
```

预期：

```text
operation-log-actions 本身不会让 total 增加。
```

注意：

```text
如果你调用的是 /api/admin/players，那会增加日志。
operation-log-actions 不应该增加日志。
```

## 任务 12：更新接口文档

### 修改文件

```text
E:\game-realtime-gm\docs\api-overview.md
```

### 修改位置

搜索：

```text
## GM 操作日志模块
```

在：

```text
### GET /api/admin/operation-logs
```

前面新增：

```markdown
### GET /api/admin/operation-log-actions

用途：查询 GM 操作日志 action 筛选选项。

鉴权：管理员 token。

响应重点：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "items": [
      {
        "value": "admin.players.list",
        "label": "查询玩家列表"
      }
    ]
  }
}
```
```

如果 Markdown 代码块不好嵌套，可以只写普通文本，不强求嵌套 JSON 示例。

## 任务 13：更新 README 当前功能

### 修改文件

```text
E:\game-realtime-gm\README.md
```

### 修改位置 1：GM 管理侧功能列表

搜索：

```text
### GM 管理侧
```

在操作日志相关功能后新增：

```text
- GM 操作日志 action 筛选选项
```

### 修改位置 2：核心接口列表

搜索：

```text
核心接口包括：
```

在操作日志接口附近新增：

```text
GET  /api/admin/operation-log-actions
```

## 今日验收清单

Day 17 完成时，你应该满足：

- `go test ./...` 通过。
- `GET /api/admin/operation-log-actions` 已注册到路由。
- 管理员 token 可以访问。
- 不带 token 不能访问。
- 玩家 token 不能访问。
- 返回 `items` 数组。
- 每个 item 有 `value` 和 `label`。
- 返回的 `value` 可以用于 `/api/admin/operation-logs?action=xxx` 筛选。
- 调用 action 选项接口不会额外写入 GM 操作日志。
- `docs/api-overview.md` 已更新。
- `README.md` 已更新。

## 常见问题

### 1. router.go 报 undefined: adminHandler.ListOperationLogActions

原因：

```text
你注册了路由，但 admin.go 里没有成功新增 ListOperationLogActions 方法。
```

检查：

```powershell
Select-String -Path E:\game-realtime-gm\backend\internal\handler\admin.go -Pattern "ListOperationLogActions"
```

### 2. admin.go 报 operationLogActionOptions undefined

原因：

```text
你新增了 ListOperationLogActions 方法，但没有新增 operationLogActionOptions 变量。
```

检查：

```powershell
Select-String -Path E:\game-realtime-gm\backend\internal\handler\admin.go -Pattern "operationLogActionOptions"
```

### 3. action 下拉框有接口，但日志筛选没数据

原因：

```text
数据库里还没有对应 action 的操作日志。
```

例如要产生：

```text
admin.players.list
```

就调用：

```http
GET /api/admin/players?page=1&page_size=10
```

要产生：

```text
admin.players.ban
```

就封禁一个玩家。

### 4. 为什么不从数据库查 DISTINCT action

因为：

```text
数据库只知道已经发生过的 action。
但前端需要知道系统支持哪些 action。
```

所以 Day 17 先用固定列表。

## 今日不要做什么

今天不要做：

- 动态统计每个 action 的日志数量。
- action 多语言配置。
- 前端筛选组件。
- 日志导出。
- 操作日志删除。
- 复杂 RBAC 权限。

原因：

```text
今天只做“后端告诉前端有哪些 action 可以筛选”这个最小闭环。
```

## 任务 14：提交并推送到 GitHub

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

### 选择性添加 Day 17 相关文件

如果今天按文档完成了代码和文档修改，执行：

```powershell
git add backend README.md docs/api-overview.md docs/day17-plan.md
```

不要无脑使用：

```powershell
git add .
```

### 提交

```powershell
git commit -m "Complete day17 operation log actions"
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

Day 18 可以做：

```text
dashboard 最近 GM 操作日志接口
```

例如：

```text
GET /api/admin/dashboard/recent-operation-logs
```

这样后台首页就不只有统计数字，还能看到最近发生了哪些 GM 操作。
