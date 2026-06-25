# Day 16 学习计划：GM 后台首页统计接口

## 你今天从任务几开始

你现在从：

```text
任务 0：确认 Day 15 操作日志详情接口已经完成
```

开始。

Day 16 不改数据库表，不新增复杂权限系统，不做前端页面。

今天只做一个适合 GM 后台首页展示的统计接口：

```text
GET /api/admin/dashboard/summary
```

## 今日目标

新增 GM 后台首页统计接口，返回这些数据：

```text
玩家总数
正常玩家数
被封禁玩家数
今日新增玩家数
当前在线玩家数
今日 GM 操作次数
```

接口示例：

```http
GET http://localhost:8080/api/admin/dashboard/summary
Authorization: Bearer 管理员token
```

## 为什么今天做这个

Day 12 到 Day 15 已经把 GM 操作日志主线做到了：

```text
记录日志
查询日志列表
按条件筛选日志
查看日志详情
```

现在后台还缺一个“首页概览”接口。

真实 GM 后台打开后，一般不会第一眼就是日志列表，而是先看到一些核心数字：

```text
现在有多少玩家？
有多少玩家被封？
今天新增了多少玩家？
现在大概有多少在线玩家？
今天 GM 做了多少操作？
```

这些数字就是 dashboard summary。

## 今日最终效果

请求：

```http
GET /api/admin/dashboard/summary
Authorization: Bearer 管理员token
```

成功返回：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "total_players": 12,
    "normal_players": 10,
    "banned_players": 2,
    "today_new_players": 3,
    "online_players": 1,
    "today_admin_operations": 8
  }
}
```

不带 token：

```json
{
  "code": 40112,
  "message": "missing authorization header"
}
```

使用玩家 token：

```json
{
  "code": 40311,
  "message": "admin permission required"
}
```

实际错误码以你当前中间件实现为准。

## 今日会学到什么

今天会学到：

- 什么是 dashboard summary。
- 为什么后台首页需要聚合统计接口。
- 如何在一个 handler 里同时使用 PostgreSQL 和 Redis。
- 如何用 `COUNT(*)` 做基础统计。
- 如何按 `status` 统计玩家数量。
- 如何按当天时间范围统计今日新增玩家和今日 GM 操作。
- 如何用 Redis `SCAN` 统计在线玩家 key。
- 为什么线上不建议用 `KEYS online:*`。

## 今日文件范围

今天主要修改两个 Go 文件：

```text
E:\game-realtime-gm\backend\internal\handler\admin.go
E:\game-realtime-gm\backend\internal\router\router.go
```

可能同步更新两个文档：

```text
E:\game-realtime-gm\README.md
E:\game-realtime-gm\docs\api-overview.md
```

不需要改：

```text
E:\game-realtime-gm\backend\internal\database\schema.sql
```

原因是今天用到的字段已经存在：

```text
players.status
players.created_at
admin_operation_logs.created_at
```

## 今日设计

### 接口路径

```text
GET /api/admin/dashboard/summary
```

为什么这样命名：

```text
dashboard 表示后台首页。
summary 表示首页概览统计。
```

它不是某一个玩家资源，也不是某一条日志资源，所以不放在：

```text
/api/admin/players
/api/admin/operation-logs
```

下面。

### 返回字段

| 字段 | 含义 | 数据来源 |
| --- | --- | --- |
| `total_players` | 玩家总数 | PostgreSQL `players` |
| `normal_players` | 正常玩家数 | PostgreSQL `players.status = 'normal'` |
| `banned_players` | 被封禁玩家数 | PostgreSQL `players.status = 'banned'` |
| `today_new_players` | 今日新增玩家数 | PostgreSQL `players.created_at` |
| `online_players` | 当前在线玩家数 | Redis `online:player:*` |
| `today_admin_operations` | 今日 GM 操作次数 | PostgreSQL `admin_operation_logs.created_at` |

### 今日时区规则

今日统计使用 UTC+8 业务时区。

也就是今天的开始时间是：

```text
当天 00:00:00 +08:00
```

今天的结束时间可以使用：

```text
当前时间
```

这和 Day 14 的 `range=today` 思路保持一致。

## 当前项目已有基础

当前 `AdminHandler` 在：

```text
E:\game-realtime-gm\backend\internal\handler\admin.go
```

现在结构大概是：

```go
type AdminHandler struct {
	db *pgxpool.Pool
}
```

这说明它现在只能访问 PostgreSQL。

但是 Day 16 要统计在线人数，而在线状态存在 Redis：

```text
online:player:<player_id>
```

所以今天需要把 `AdminHandler` 改成同时保存：

```text
db
redisClient
```

## 任务 0：确认 Day 15 状态

### 在哪里执行

PowerShell。

### 执行命令

```powershell
cd E:\game-realtime-gm\backend
go test ./...
```

再检查 Day 15 的方法和路由：

```powershell
Select-String -Path E:\game-realtime-gm\backend\internal\handler\admin.go -Pattern "GetOperationLogByID" -SimpleMatch
Select-String -Path E:\game-realtime-gm\backend\internal\router\router.go -Pattern '"/operation-logs/:id"' -SimpleMatch
```

如果都能看到输出，再继续 Day 16。

## 任务 1：让 AdminHandler 能访问 Redis

### 你要做什么

把 `AdminHandler` 从“只能访问 PostgreSQL”改成“可以访问 PostgreSQL + Redis”。

### 为什么要做

今日统计里：

```text
玩家总数、封禁玩家数、今日新增、今日 GM 操作
```

来自 PostgreSQL。

但是：

```text
当前在线玩家数
```

来自 Redis。

所以 `AdminHandler` 必须拿到 Redis 客户端。

### 修改文件

```text
E:\game-realtime-gm\backend\internal\handler\admin.go
```

### 操作类型

局部修改。

### 修改位置 1：import 增加 go-redis

在 `admin.go` 文件顶部找到 import。

当前应该能看到：

```go
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
```

改成：

```go
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
```

### 修改位置 2：AdminHandler 结构体

搜索：

```text
type AdminHandler struct
```

修改前：

```go
type AdminHandler struct {
	db *pgxpool.Pool
}
```

修改后：

```go
type AdminHandler struct {
	db          *pgxpool.Pool
	redisClient *redis.Client
}
```

这表示：

```text
AdminHandler 以后既可以查数据库，也可以查 Redis。
```

### 修改位置 3：构造函数

搜索：

```text
func NewAdminHandler
```

修改前：

```go
func NewAdminHandler(db *pgxpool.Pool) *AdminHandler {
	return &AdminHandler{db: db}
}
```

修改后：

```go
func NewAdminHandler(db *pgxpool.Pool, redisClient *redis.Client) *AdminHandler {
	return &AdminHandler{
		db:          db,
		redisClient: redisClient,
	}
}
```

### 代码分析

`NewAdminHandler` 是创建 `AdminHandler` 的地方。

之前只传：

```text
db
```

现在多传：

```text
redisClient
```

这样新增的 dashboard 接口才能调用 Redis。

## 任务 2：修改 router.go 传入 Redis

### 修改文件

```text
E:\game-realtime-gm\backend\internal\router\router.go
```

### 修改位置

搜索：

```text
handler.NewAdminHandler
```

修改前：

```go
adminHandler := handler.NewAdminHandler(db)
```

修改后：

```go
adminHandler := handler.NewAdminHandler(db, redisClient)
```

### 为什么要改 router.go

`router.New` 这个函数本来就有 Redis 参数：

```go
func New(db *pgxpool.Pool, redisClient *redis.Client, cfg config.Config) http.Handler
```

之前 Redis 只传给了：

```text
OnlineHandler
```

今天要让 GM 后台统计也能查 Redis，所以要把同一个 `redisClient` 也传给：

```text
AdminHandler
```

## 任务 3：新增 DashboardSummary 方法

### 修改文件

```text
E:\game-realtime-gm\backend\internal\handler\admin.go
```

### 修改位置

搜索：

```text
func (h *AdminHandler) Me
```

建议把新方法放在 `Me` 方法后面、`recordOperation` 方法前面。

原因是：

```text
DashboardSummary 是一个对外接口方法。
recordOperation 是内部辅助方法。
```

这样文件阅读顺序更清楚。

### 操作类型

在 `Me` 方法结束后新增方法。

找到 `Me` 方法结尾：

```go
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
```

在这个 `}` 后面新增：

```go
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
			"total_players":           totalPlayers,
			"normal_players":          normalPlayers,
			"banned_players":          bannedPlayers,
			"today_new_players":       todayNewPlayers,
			"online_players":          onlinePlayers,
			"today_admin_operations":  todayAdminOperations,
		},
	})
}
```

### 代码分析

这一段做了 6 个统计：

```sql
SELECT COUNT(*) FROM players
```

统计玩家总数。

```sql
SELECT COUNT(*) FROM players WHERE status = 'normal'
```

统计正常玩家数。

```sql
SELECT COUNT(*) FROM players WHERE status = 'banned'
```

统计被封禁玩家数。

```sql
SELECT COUNT(*) FROM players WHERE created_at >= todayStart
```

统计今天新增玩家。

```text
h.countOnlinePlayers(c)
```

统计 Redis 里当前在线玩家。

```sql
SELECT COUNT(*) FROM admin_operation_logs WHERE created_at >= todayStart
```

统计今天 GM 操作次数。

## 任务 4：新增 Redis 在线人数统计 helper

### 修改文件

```text
E:\game-realtime-gm\backend\internal\handler\admin.go
```

### 修改位置

建议放在 `DashboardSummary` 方法后面。

### 操作类型

新增 helper 方法。

新增代码：

```go
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
```

### 为什么用 SCAN

Redis 里在线 key 的格式是：

```text
online:player:<player_id>
```

你可能会想到：

```text
KEYS online:player:*
```

但是线上不推荐 `KEYS`。

原因是：

```text
KEYS 会一次性扫描所有 key，数据多时可能阻塞 Redis。
```

`SCAN` 是分批扫描，适合后台统计这种场景。

### helper 为什么放在 admin.go

今天这个 helper 只给：

```text
AdminHandler.DashboardSummary
```

使用。

所以先放在 `admin.go` 里，不单独新建文件。

如果后面多个模块都要统计在线人数，再考虑提到公共 service 或 cache helper。

## 任务 5：注册 dashboard 路由

### 修改文件

```text
E:\game-realtime-gm\backend\internal\router\router.go
```

### 修改位置

搜索：

```text
adminProtected.GET("/me", adminHandler.Me)
```

当前应该能看到：

```go
adminProtected.GET("/me", adminHandler.Me)
adminProtected.GET("/players", adminHandler.ListPlayers)
```

在 `/me` 后面新增：

```go
adminProtected.GET("/dashboard/summary", adminHandler.DashboardSummary)
```

最终变成：

```go
adminProtected.GET("/me", adminHandler.Me)
adminProtected.GET("/dashboard/summary", adminHandler.DashboardSummary)
adminProtected.GET("/players", adminHandler.ListPlayers)
```

### 为什么放在 adminProtected 下

因为 dashboard 是 GM 后台数据。

普通玩家不应该看到：

```text
玩家总数
封禁玩家数
GM 操作次数
```

所以必须走：

```text
middleware.AdminAuth
```

## 任务 6：格式化和编译

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
格式化 Go 文件，让缩进和 import 排列符合 Go 标准。
```

`go test ./...`：

```text
编译并测试当前 backend 下所有 Go 包。
```

如果看到：

```text
[no test files]
```

不是错误。

它表示当前包没有测试文件，但编译通过。

## 任务 7：启动依赖和后端

### 启动 Docker 依赖

```powershell
cd E:\game-realtime-gm\deploy
docker compose up -d
docker ps
```

你应该能看到：

```text
game_realtime_postgres
game_realtime_redis
```

### 启动后端

如果你用 GoLand，直接运行 `backend/cmd/server`。

如果你用 PowerShell：

```powershell
cd E:\game-realtime-gm\backend
go run .\cmd\server
```

如果 8080 被占用：

```text
listen tcp :8080: bind: Only one usage of each socket address
```

说明后端已经有一个实例在运行，不一定是错误。

## 任务 8：准备管理员 token

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

复制返回里的：

```text
data.token
```

后续请求加 Header：

```text
Authorization: Bearer 管理员token
```

## 任务 9：制造一些可观察数据

如果你的数据库数据比较少，统计接口返回 0 很正常。

为了让结果更容易看，可以先做这些操作。

### 注册两个玩家

```http
POST http://localhost:8080/api/register
Content-Type: application/json

{
  "username": "day16_player_1",
  "password": "123456",
  "nickname": "Day16玩家1"
}
```

再注册一个：

```http
POST http://localhost:8080/api/register
Content-Type: application/json

{
  "username": "day16_player_2",
  "password": "123456",
  "nickname": "Day16玩家2"
}
```

### 让一个玩家在线

先玩家登录：

```http
POST http://localhost:8080/api/login
Content-Type: application/json

{
  "username": "day16_player_1",
  "password": "123456"
}
```

复制玩家 token。

调用心跳：

```http
POST http://localhost:8080/api/online/heartbeat
Authorization: Bearer 玩家token
```

这个请求会在 Redis 写入：

```text
online:player:<player_id>
```

有效期是 2 分钟。

### 产生 GM 操作日志

用管理员 token 请求：

```http
GET http://localhost:8080/api/admin/players?page=1&page_size=10
Authorization: Bearer 管理员token
```

这个请求会产生一条：

```text
admin.players.list
```

操作日志。

## 任务 10：测试 dashboard summary 接口

### Apifox 请求

```http
GET http://localhost:8080/api/admin/dashboard/summary
Authorization: Bearer 管理员token
```

预期返回：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "total_players": 2,
    "normal_players": 2,
    "banned_players": 0,
    "today_new_players": 2,
    "online_players": 1,
    "today_admin_operations": 1
  }
}
```

数字不一定完全一样。

你要重点看：

```text
字段都存在
字段类型都是数字
online_players 在心跳后能变成 1 或更大
today_admin_operations 在调用 GM 接口后会增加
```

## 任务 11：测试权限

### 不带 token

```http
GET http://localhost:8080/api/admin/dashboard/summary
```

预期：

```text
返回 401 未登录类错误。
```

### 使用玩家 token

```http
GET http://localhost:8080/api/admin/dashboard/summary
Authorization: Bearer 玩家token
```

预期：

```text
返回 403 无管理员权限类错误。
```

## 任务 12：用数据库和 Redis 对照验证

### PostgreSQL 对照

进入数据库：

```powershell
docker exec -it game_realtime_postgres psql -U game -d game_realtime
```

执行：

```sql
SELECT COUNT(*) FROM players;
SELECT COUNT(*) FROM players WHERE status = 'normal';
SELECT COUNT(*) FROM players WHERE status = 'banned';
SELECT COUNT(*) FROM players WHERE created_at >= date_trunc('day', now() AT TIME ZONE 'Asia/Shanghai');
SELECT COUNT(*) FROM admin_operation_logs WHERE created_at >= date_trunc('day', now() AT TIME ZONE 'Asia/Shanghai');
```

说明：

```text
这些 SQL 是用来帮助你理解统计来源。
实际接口代码里使用 Go 计算 todayStart。
```

退出：

```sql
\q
```

### Redis 对照

进入 Redis：

```powershell
docker exec -it game_realtime_redis redis-cli
```

查看在线 key：

```redis
SCAN 0 MATCH online:player:* COUNT 100
```

如果刚刚调用过心跳，应该能看到类似：

```text
online:player:1
```

查看 TTL：

```redis
TTL online:player:1
```

退出：

```redis
exit
```

## 任务 13：更新接口文档

### 修改文件

```text
E:\game-realtime-gm\docs\api-overview.md
```

### 修改位置

搜索：

```text
## GM 管理模块
```

在：

```text
### GET /api/admin/me
```

后面新增：

```markdown
### GET /api/admin/dashboard/summary

用途：查询 GM 后台首页统计数据。

鉴权：管理员 token。

响应重点：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "total_players": 12,
    "normal_players": 10,
    "banned_players": 2,
    "today_new_players": 3,
    "online_players": 1,
    "today_admin_operations": 8
  }
}
```
```

注意：上面 Markdown 代码块嵌套时容易粘错。如果编辑器不好处理，可以把 JSON 示例外层代码块去掉。

## 任务 14：更新 README 当前功能

### 修改文件

```text
E:\game-realtime-gm\README.md
```

### 修改位置

搜索：

```text
### GM 管理侧
```

在功能列表里新增：

```text
- GM 后台首页统计
```

再搜索：

```text
核心接口包括：
```

在核心接口列表中新增：

```text
GET  /api/admin/dashboard/summary
```

## 今日验收清单

Day 16 完成时，你应该满足：

- `go test ./...` 通过。
- `GET /api/admin/dashboard/summary` 已注册到路由。
- 管理员 token 可以访问 dashboard summary。
- 不带 token 不能访问。
- 玩家 token 不能访问。
- 返回字段包含 `total_players`、`normal_players`、`banned_players`、`today_new_players`、`online_players`、`today_admin_operations`。
- 注册玩家后，玩家数量统计能变化。
- 玩家心跳后，`online_players` 能变化。
- 调用 GM 接口后，`today_admin_operations` 能变化。
- `docs/api-overview.md` 已补充新接口。
- `README.md` 已补充新功能。

## 常见问题

### 1. router.go 报参数数量不匹配

错误类似：

```text
not enough arguments in call to handler.NewAdminHandler
```

原因：

```text
你改了 NewAdminHandler，让它需要 db 和 redisClient 两个参数，
但是 router.go 里还只传了 db。
```

检查：

```text
E:\game-realtime-gm\backend\internal\router\router.go
```

应该是：

```go
adminHandler := handler.NewAdminHandler(db, redisClient)
```

### 2. admin.go 报 undefined: redis

原因：

```text
你用了 *redis.Client，但没有 import go-redis。
```

检查 import 是否包含：

```go
"github.com/redis/go-redis/v9"
```

### 3. online_players 一直是 0

可能原因：

```text
没有调用玩家心跳接口。
心跳 token 用错了，必须是玩家 token。
超过 2 分钟 TTL 过期了。
Redis 容器不是当前后端连接的那个 Redis。
```

检查 Redis：

```powershell
docker exec -it game_realtime_redis redis-cli
SCAN 0 MATCH online:player:* COUNT 100
```

### 4. 今日新增玩家数和数据库 SQL 对不上

可能原因：

```text
时区理解不同。
created_at 是 timestamptz。
接口使用 UTC+8 的 todayStart。
你手写 SQL 使用了数据库默认时区。
```

今天先理解原则：

```text
业务上按中国时间的一天来统计。
```

### 5. today_admin_operations 没增加

原因可能是：

```text
你调用的不是会记录日志的 GM 接口。
```

建议调用：

```http
GET /api/admin/players?page=1&page_size=10
```

这个接口当前会记录：

```text
admin.players.list
```

## 今日不要做什么

今天不要做：

- 前端 dashboard 页面。
- 图表。
- 周活跃、月活跃。
- 复杂统计缓存。
- 定时任务。
- Prometheus 指标。
- 多维度报表。

原因是今天的目标是：

```text
先跑通后台首页最小统计闭环。
```

## 任务 15：提交并推送到 GitHub

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

### 选择性添加 Day 16 相关文件

如果今天按文档完成了代码和文档修改，执行：

```powershell
git add backend README.md docs/api-overview.md docs/day16-plan.md
```

不要无脑使用：

```powershell
git add .
```

### 提交

```powershell
git commit -m "Complete day16 dashboard summary"
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

Day 17 可以做：

```text
GM 操作日志操作类型选项接口
```

例如：

```text
GET /api/admin/operation-log-actions
```

它可以给前端筛选框使用。

也可以开始做：

```text
dashboard 最近操作日志列表
```

让后台首页不仅有统计数字，也有最近 GM 操作动态。
