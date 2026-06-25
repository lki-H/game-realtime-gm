# Day 15 学习计划：GM 操作日志详情接口

## 你今天从任务几开始

你现在从：

```text
任务 0：确认 Day 14 的日志列表接口已经可用
```

开始。

Day 15 不改数据库表，不改登录逻辑，不改封禁逻辑。

今天只做一件小而完整的事：

```text
根据操作日志 id，查看一条 GM 操作日志详情。
```

## 今日目标

新增一个接口：

```text
GET /api/admin/operation-logs/:id
```

例如：

```text
GET http://localhost:8080/api/admin/operation-logs/1
```

它的作用是：

```text
GM 后台日志列表页点击某一条日志后，可以进入详情页查看完整内容。
```

## 为什么今天要做这个

Day 12 到 Day 14 已经完成了：

```text
GET /api/admin/operation-logs
```

它适合做“列表页”。

但是列表页通常只展示摘要，例如：

```text
id
管理员
操作类型
目标对象
时间
```

如果前端以后要做详情弹窗或详情页，就需要一个按 id 查询单条日志的接口。

所以 Day 15 的业务逻辑是：

```text
列表接口负责查一批。
详情接口负责查一条。
```

## 今日最终效果

正常请求：

```http
GET /api/admin/operation-logs/1
Authorization: Bearer 管理员token
```

返回：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "id": 1,
    "admin_id": 1,
    "admin_username": "admin",
    "admin_role": "super_admin",
    "action": "admin.players.list",
    "target_type": "player",
    "target_id": null,
    "detail": "page=1,page_size=10",
    "ip": "::1",
    "user_agent": "Apifox/xxx",
    "created_at": "2026-06-09T08:33:34.846804Z"
  }
}
```

id 格式错误：

```text
GET /api/admin/operation-logs/abc
```

返回：

```json
{
  "code": 40081,
  "message": "invalid operation log id"
}
```

id 不存在：

```text
GET /api/admin/operation-logs/999999
```

返回：

```json
{
  "code": 40481,
  "message": "operation log not found"
}
```

## 今日会学到什么

今天会学到：

- 列表接口和详情接口的区别。
- 如何从 URL 路径中读取 `:id`。
- 为什么 id 要校验为正整数。
- 如何用 `QueryRow` 查询单条数据。
- 如何处理 `pgx.ErrNoRows`。
- 为什么日志详情接口只能允许管理员访问。
- 为什么审计日志只做查询，不做修改和删除。

## 今日文件范围

今天只需要改两个 Go 文件：

```text
E:\game-realtime-gm\backend\internal\handler\admin.go
E:\game-realtime-gm\backend\internal\router\router.go
```

不需要改：

```text
E:\game-realtime-gm\backend\internal\database\schema.sql
E:\game-realtime-gm\backend\internal\model\admin_operation_log.go
```

原因是：

```text
admin_operation_logs 表已经存在。
model.AdminOperationLog 结构体已经存在。
```

Day 15 只是新增“查询单条日志”的 handler 和 route。

## 当前项目已有基础

当前代码里已经有：

```text
AdminHandler.ListOperationLogs
```

它在：

```text
E:\game-realtime-gm\backend\internal\handler\admin.go
```

当前路由里已经有：

```go
adminProtected.GET("/operation-logs", adminHandler.ListOperationLogs)
```

它在：

```text
E:\game-realtime-gm\backend\internal\router\router.go
```

Day 15 要在这个基础上新增：

```go
adminProtected.GET("/operation-logs/:id", adminHandler.GetOperationLogByID)
```

## 任务 0：确认 Day 14 状态

### 在哪里执行

PowerShell。

### 执行命令

```powershell
cd E:\game-realtime-gm\backend
go test ./...
```

如果通过，继续。

再执行：

```powershell
Select-String -Path E:\game-realtime-gm\backend\internal\handler\admin.go -Pattern "func (h *AdminHandler) ListOperationLogs" -SimpleMatch
Select-String -Path E:\game-realtime-gm\backend\internal\router\router.go -Pattern '"/operation-logs"'
```

你应该能看到：

```text
ListOperationLogs
adminProtected.GET("/operation-logs", adminHandler.ListOperationLogs)
```

如果看不到，说明 Day 12 到 Day 14 的代码还没完成，先不要做 Day 15。

## 任务 1：在 admin.go 新增详情方法

### 修改文件

```text
E:\game-realtime-gm\backend\internal\handler\admin.go
```

### 定位方法

打开文件后，搜索：

```text
func (h *AdminHandler) ListOperationLogs
```

然后一直往下看，找到这个方法的最后一个 `}`。

当前 `ListOperationLogs` 的结尾大概是：

```go
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

Day 15 的新方法放在它后面。

也就是：

```text
ListOperationLogs 方法结束后
文件末尾
```

新增下面代码：

```go
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
```

### 重要提醒

这段代码需要用到：

```go
errors
net/http
strconv
model
pgx
```

这些当前 `admin.go` 里已经在使用了，所以正常情况下不需要新增 import。

如果你粘贴后出现：

```text
undefined: errors
undefined: pgx
```

再检查文件顶部 import，不要盲目乱加。

当前正确方向是顶部应该已经有类似：

```go
import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"game-realtime-gm/backend/internal/model"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)
```

## 任务 2：在 router.go 注册详情路由

### 修改文件

```text
E:\game-realtime-gm\backend\internal\router\router.go
```

### 定位方法

搜索：

```text
adminProtected.GET("/operation-logs", adminHandler.ListOperationLogs)
```

你现在应该能看到这一段：

```go
	adminProtected.POST("/players/:id/ban", adminHandler.BanPlayer)
	adminProtected.POST("/players/:id/unban", adminHandler.UnbanPlayer)
	adminProtected.GET("/operation-logs", adminHandler.ListOperationLogs)
```

在它下面新增一行：

```go
	adminProtected.GET("/operation-logs/:id", adminHandler.GetOperationLogByID)
```

最终变成：

```go
	adminProtected.POST("/players/:id/ban", adminHandler.BanPlayer)
	adminProtected.POST("/players/:id/unban", adminHandler.UnbanPlayer)
	adminProtected.GET("/operation-logs", adminHandler.ListOperationLogs)
	adminProtected.GET("/operation-logs/:id", adminHandler.GetOperationLogByID)
```

### 路由解释

这两个接口不冲突：

```text
GET /api/admin/operation-logs
GET /api/admin/operation-logs/1
```

第一个是列表。

第二个是详情。

它们都放在：

```text
adminProtected
```

下面，所以都必须使用管理员 token。

## 任务 3：格式化代码

### 在哪里执行

PowerShell。

### 执行命令

```powershell
cd E:\game-realtime-gm\backend
gofmt -w .\internal\handler\admin.go .\internal\router\router.go
```

如果这里报错，不要继续运行服务。

先看错误提示是哪一行。

最常见原因是：

```text
少了 }
多了 )
把方法粘贴到了另一个方法里面
```

你可以把错误和 `admin.go` 对应行附近 20 行发给我。

## 任务 4：编译测试

### 执行命令

```powershell
cd E:\game-realtime-gm\backend
go test ./...
```

预期结果类似：

```text
?       game-realtime-gm/backend/cmd/server        [no test files]
?       game-realtime-gm/backend/internal/handler  [no test files]
...
```

只要没有 `FAIL`，就说明编译通过。

## 任务 5：启动后端服务

如果你在 GoLand 里启动，直接运行 `cmd/server`。

如果你用 PowerShell：

```powershell
cd E:\game-realtime-gm\backend
go run .\cmd\server
```

如果出现：

```text
listen tcp :8080: bind: Only one usage of each socket address
```

说明 8080 已经有一个后端在运行。

这不是代码错误。

处理方式是：

```powershell
netstat -ano | findstr :8080
```

找到 PID 后再决定是否结束旧进程。

## 任务 6：准备管理员 token

Day 15 的接口必须用管理员 token。

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

登录成功后，复制返回里的：

```text
data.token
```

然后后续请求都加 Header：

```text
Authorization: Bearer 这里粘贴管理员token
```

注意：

```text
Bearer 后面有一个空格。
```

## 任务 7：先用列表接口拿到一条日志 id

### Apifox 请求

```http
GET http://localhost:8080/api/admin/operation-logs?page=1&page_size=10
Authorization: Bearer 管理员token
```

成功后，看返回：

```json
{
  "data": {
    "items": [
      {
        "id": 1
      }
    ]
  }
}
```

复制第一条日志的 `id`。

如果 `items` 是空数组，先调用一次管理员接口制造日志，例如：

```http
GET http://localhost:8080/api/admin/players?page=1&page_size=10
Authorization: Bearer 管理员token
```

这个接口会写入一条：

```text
admin.players.list
```

操作日志。

然后再查：

```http
GET http://localhost:8080/api/admin/operation-logs?page=1&page_size=10
```

## 任务 8：测试日志详情接口

假设你拿到的日志 id 是：

```text
1
```

请求：

```http
GET http://localhost:8080/api/admin/operation-logs/1
Authorization: Bearer 管理员token
```

预期返回：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "id": 1,
    "admin_username": "admin",
    "action": "admin.players.list"
  }
}
```

返回字段不只这几个。

只要能看到完整的日志对象，就说明成功。

## 任务 9：测试错误 id

### 非数字 id

请求：

```http
GET http://localhost:8080/api/admin/operation-logs/abc
Authorization: Bearer 管理员token
```

预期：

```json
{
  "code": 40081,
  "message": "invalid operation log id"
}
```

### 不存在的 id

请求：

```http
GET http://localhost:8080/api/admin/operation-logs/999999
Authorization: Bearer 管理员token
```

预期：

```json
{
  "code": 40481,
  "message": "operation log not found"
}
```

## 任务 10：测试权限

### 不带 token

请求：

```http
GET http://localhost:8080/api/admin/operation-logs/1
```

预期应该是未登录错误。

如果你之前的中间件错误码没有改过，通常是：

```json
{
  "code": 40112,
  "message": "missing authorization header"
}
```

### 使用玩家 token

先用玩家登录接口拿玩家 token：

```http
POST http://localhost:8080/api/login
```

然后请求：

```http
GET http://localhost:8080/api/admin/operation-logs/1
Authorization: Bearer 玩家token
```

预期应该是无管理员权限。

如果你之前的中间件错误码没有改过，通常是：

```json
{
  "code": 40311,
  "message": "admin permission required"
}
```

## 任务 11：用数据库对照验证

### 进入 PostgreSQL

```powershell
docker exec -it game_realtime_postgres psql -U game -d game_realtime
```

### 查询指定日志

假设接口里查的是 id 为 1 的日志：

```sql
SELECT id, admin_id, admin_username, admin_role, action, target_type, target_id, detail, ip, user_agent, created_at
FROM admin_operation_logs
WHERE id = 1;
```

你要对照：

```text
接口返回的 data
数据库查询结果
```

是否是同一条。

退出 psql：

```sql
\q
```

## 今日验收标准

Day 15 完成时，你应该满足：

- `go test ./...` 通过。
- `GET /api/admin/operation-logs` 列表接口还能正常用。
- `GET /api/admin/operation-logs/:id` 能查到单条日志。
- 非数字 id 返回 `40081`。
- 不存在 id 返回 `40481`。
- 不带 token 不能访问。
- 玩家 token 不能访问。
- 数据库查询结果和接口返回能对上。

## 常见问题

### 1. router.go 报 undefined: adminHandler.GetOperationLogByID

原因：

```text
你注册了路由，但是 admin.go 里没有成功新增 GetOperationLogByID 方法。
```

检查：

```text
E:\game-realtime-gm\backend\internal\handler\admin.go
```

搜索：

```text
func (h *AdminHandler) GetOperationLogByID
```

如果搜不到，说明方法没加进去。

### 2. gofmt 报 expected declaration

原因通常是：

```text
你把 GetOperationLogByID 粘贴到了 ListOperationLogs 方法内部。
```

正确位置是：

```text
ListOperationLogs 的最后一个 } 后面。
```

不是它的 `c.JSON(...)` 里面。

### 3. 查询 999999 返回 500，不是 404

原因：

```text
没有正确判断 pgx.ErrNoRows。
```

检查代码里是否有：

```go
if errors.Is(err, pgx.ErrNoRows) {
```

并且这个判断要在普通 `if err != nil` 前面。

### 4. /operation-logs/1 返回 404 page not found

原因：

```text
router.go 没有注册新路由，或者服务没有重启。
```

检查：

```text
E:\game-realtime-gm\backend\internal\router\router.go
```

是否有：

```go
adminProtected.GET("/operation-logs/:id", adminHandler.GetOperationLogByID)
```

然后重启后端服务。

### 5. items 是空数组，没有日志 id 可以测试

原因：

```text
数据库刚恢复，admin_operation_logs 里还没有数据。
```

解决：

用管理员 token 请求一次：

```http
GET http://localhost:8080/api/admin/players?page=1&page_size=10
```

这个请求会产生一条 GM 操作日志。

## 今日不要做什么

今天不要做：

- 删除操作日志。
- 修改操作日志。
- 日志导出 Excel。
- 日志详情前端页面。
- 操作日志统计图表。

原因是操作日志属于审计数据。

当前阶段先把：

```text
列表
筛选
详情
权限
```

这条主线学扎实。

## 明日预告

Day 16 可以继续做：

```text
GM 操作日志按操作类型下拉选项接口
```

例如：

```text
GET /api/admin/operation-log-actions
```

用于前端筛选框。

也可以开始进入：

```text
GM 后台数据统计接口
```

例如：

```text
玩家总数
今日新增玩家
被封禁玩家数
在线玩家数
今日 GM 操作次数
```

这会更像真正后台首页的 dashboard。
