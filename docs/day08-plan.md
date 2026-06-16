# Day 08 学习计划：GM 管理员查询玩家列表与玩家详情

## 今日目标

今天承接 Day 07 的管理员鉴权。

Day 07 已经完成或当前代码中已经具备：

```text
POST /api/admin/login
GET  /api/admin/me
middleware.AdminAuth
```

管理员现在可以登录、拿 token，并用管理员 token 访问 GM 受保护接口。

Day 08 要开始做第一个真正的 GM 业务功能：

```text
GM 管理员查询玩家列表和玩家详情
```

今天新增两个接口：

```text
GET /api/admin/players
GET /api/admin/players/:id
```

它们必须使用管理员 token 才能访问。

## 为什么今天做 GM 查询玩家

GM 后台的核心价值不是“管理员自己能登录”，而是管理员登录后能管理游戏里的业务对象。

最自然的第一个业务对象就是：

```text
玩家
```

真实 GM 后台通常会先有这些能力：

- 查询玩家列表。
- 按用户名或昵称搜索玩家。
- 查看玩家基础资料。
- 后续再做封禁、改昵称、查看战绩、查看在线状态、查看房间记录。

今天先做查询，不做修改。

原因是查询接口风险低、容易验证，而且能把 Day 05 的分页查询和 Day 07 的管理员鉴权连接起来。

## 当前项目状态

当前已经有玩家侧接口：

```text
GET /api/players
GET /api/players/:id
```

它们现在挂在玩家鉴权分组下，也就是登录玩家可以访问。

但从业务角度看：

```text
查看全部玩家列表更像 GM 后台能力，不像普通玩家能力。
```

今天先新增管理员版本：

```text
GET /api/admin/players
GET /api/admin/players/:id
```

暂时不删除旧的 `/api/players`，避免一下子破坏 Day 05 的学习成果。等 GM 查询接口跑通后，再决定是否把普通玩家的列表查询移除或限制。

## 今天会学到什么

今天会学到：

- 为什么同一个“查询玩家”能力，在玩家端和 GM 端的权限不同。
- 如何把数据库连接传给新的 `AdminHandler`。
- 如何在管理员接口中复用分页、搜索、按 ID 查询逻辑。
- 如何设计后台管理接口路径。
- 为什么查询接口也要做权限控制。
- 为什么不能把所有接口都混在玩家鉴权下面。

一句话理解：

```text
玩家接口服务玩家自己，GM 接口服务管理员运营；两者可以查同一张表，但权限入口必须分开。
```

## 今日最终效果

完成后，管理员可以用 Day 06/Day 07 的管理员 token 调用：

```text
GET http://localhost:8080/api/admin/players?page=1&page_size=10&keyword=test
```

成功响应类似：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "items": [
      {
        "id": 1,
        "username": "testplayer",
        "nickname": "测试玩家",
        "created_at": "2026-06-06T...",
        "updated_at": "2026-06-06T..."
      }
    ],
    "page": 1,
    "page_size": 10,
    "total": 1
  }
}
```

管理员也可以调用：

```text
GET http://localhost:8080/api/admin/players/1
```

成功响应类似：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "id": 1,
    "username": "testplayer",
    "nickname": "测试玩家",
    "created_at": "2026-06-06T...",
    "updated_at": "2026-06-06T..."
  }
}
```

如果使用玩家 token 访问管理员玩家列表，应该返回：

```json
{
  "code": 40311,
  "message": "admin permission required"
}
```

这说明权限隔离生效。

## 任务 0：先验证 Day 07 状态

### 任务目标

确认 Day 07 的管理员鉴权已经能正常工作。

Day 08 依赖：

```text
GET /api/admin/me
```

如果这个接口还没跑通，先不要继续写玩家管理接口。

### 在哪里执行

PowerShell。

### 执行命令

```powershell
cd E:\game-realtime-gm\backend
go test ./...
```

### 命令作用

- `cd E:\game-realtime-gm\backend`：进入后端项目目录。
- `go test ./...`：检查所有 Go 包是否能编译。

看到 `[no test files]` 不是错误，只表示还没有测试文件。

### 启动后端

PowerShell 执行：

```powershell
cd E:\game-realtime-gm\backend
go run .\cmd\server
```

成功后应该看到：

```text
database connected
redis connected
server listening on :8080
```

如果出现：

```text
bind: Only one usage of each socket address
```

说明 8080 已经有后端进程在运行。先找到旧 PowerShell 窗口按 `Ctrl + C`，或者直接用旧后端服务测试接口。

### 用 Apifox 验证

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

复制 `data.token`。

再请求：

```text
GET http://localhost:8080/api/admin/me
```

Headers：

```text
Authorization: Bearer 管理员token
```

如果返回当前管理员信息，说明 Day 07 基础没问题。

## 任务 1：理解今天要改哪些文件

### 任务目标

今天只做 GM 查询玩家，不做别的功能。

建议修改 2 个文件：

```text
E:\game-realtime-gm\backend\internal\handler\admin.go
E:\game-realtime-gm\backend\internal\router\router.go
```

### 每个文件负责什么

`admin.go`：

```text
放管理员登录后的 GM 业务接口。
```

今天要在这里新增：

```text
ListPlayers
GetPlayerByID
```

`router.go`：

```text
集中登记接口路径和中间件。
```

今天要把新接口挂到：

```text
adminProtected
```

也就是管理员鉴权分组。

### 为什么不改 player.go

`player.go` 负责玩家侧接口。

今天的业务主体是 GM 管理员，不是普通玩家。

如果把 GM 管理员接口继续塞进 `PlayerHandler`，会导致职责混乱：

```text
玩家自己的能力
GM 管理玩家的能力
```

混在一个 handler 里，后面做封禁、操作日志、权限判断时会越来越难讲清楚。

所以今天把 GM 查询能力放进：

```text
AdminHandler
```

### 为什么暂时不新建 service

今天的查询逻辑还比较短。

当前学习阶段可以让 handler 直接访问数据库。

等后续出现这些情况，再考虑提取 service：

- 玩家查询逻辑被多个 handler 重复使用。
- 查询里混入复杂权限。
- 需要同时查 PostgreSQL 和 Redis。
- 需要记录操作日志。
- 需要事务。

## 任务 2：扩展 AdminHandler 支持数据库连接

### 任务目标

修改：

```text
E:\game-realtime-gm\backend\internal\handler\admin.go
```

操作类型：整文件替换。

### 为什么要改这个文件

Day 07 的 `AdminHandler` 是这样：

```go
type AdminHandler struct{}
```

它不需要数据库，因为 `GET /api/admin/me` 只返回 token 里的管理员身份。

但 Day 08 要查询：

```text
players 表
```

所以 `AdminHandler` 需要持有数据库连接池：

```go
db *pgxpool.Pool
```

### 完整代码

把文件：

```text
E:\game-realtime-gm\backend\internal\handler\admin.go
```

整文件替换为：

```go
package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"game-realtime-gm/backend/internal/middleware"
	"game-realtime-gm/backend/internal/model"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AdminHandler struct {
	db *pgxpool.Pool
}

func NewAdminHandler(db *pgxpool.Pool) *AdminHandler {
	return &AdminHandler{db: db}
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
		`SELECT id, username, nickname, created_at, updated_at
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
		`SELECT id, username, nickname, created_at, updated_at
         FROM players
         WHERE id = $1`,
		playerID,
	).Scan(
		&player.ID,
		&player.Username,
		&player.Nickname,
		&player.CreatedAt,
		&player.UpdatedAt,
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

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "ok",
		"data":    player,
	})
}
```

### 代码逐段分析

```go
type AdminHandler struct {
	db *pgxpool.Pool
}
```

让管理员 handler 持有 PostgreSQL 连接池。

这样管理员接口可以查询 `players` 表。

```go
func NewAdminHandler(db *pgxpool.Pool) *AdminHandler
```

构造函数从外部接收数据库连接。

这里的外部就是：

```text
router.go
```

```go
func (h *AdminHandler) ListPlayers(c *gin.Context)
```

处理：

```text
GET /api/admin/players
```

支持：

```text
page
page_size
keyword
```

例如：

```text
/api/admin/players?page=1&page_size=10&keyword=test
```

```go
pageSize > 50
```

限制单次最多返回 50 条，避免一次查太多数据。

```go
WHERE username ILIKE $1 OR nickname ILIKE $1
```

按用户名或昵称模糊搜索。

`ILIKE` 是 PostgreSQL 的大小写不敏感匹配。

```go
SELECT COUNT(*) FROM players
```

查询总数。

前端分页表格需要知道总数，才能显示：

```text
一共多少条
一共多少页
```

```go
LIMIT ... OFFSET ...
```

分页查询。

`LIMIT` 控制本页最多多少条。

`OFFSET` 控制跳过前面多少条。

```go
func (h *AdminHandler) GetPlayerByID(c *gin.Context)
```

处理：

```text
GET /api/admin/players/:id
```

用于 GM 查看某个玩家详情。

今天只返回基础资料，不返回 `password_hash`。

### 为什么不返回 password_hash

`password_hash` 虽然不是明文密码，但依然属于敏感字段。

GM 查询玩家资料时不需要看到密码哈希。

所以 SQL 只查：

```text
id
username
nickname
created_at
updated_at
```

不查：

```text
password_hash
```

### 注意：这里复用了 parsePositiveInt

`parsePositiveInt` 已经定义在：

```text
E:\game-realtime-gm\backend\internal\handler\player.go
```

因为 `admin.go` 和 `player.go` 都是：

```go
package handler
```

所以 `admin.go` 可以直接调用同包里的小写函数：

```go
parsePositiveInt
```

这也是 Go 包机制的一个重要知识点：

```text
同一个目录、同一个 package 下的文件，共享包级函数。
```

## 任务 3：更新路由注册管理员玩家查询接口

### 任务目标

修改：

```text
E:\game-realtime-gm\backend\internal\router\router.go
```

操作类型：整文件替换。

### 为什么要改这个文件

`admin.go` 里新增了 handler 方法，但如果不在 `router.go` 注册，外部请求仍然访问不到。

今天要新增两个路由：

```text
GET /api/admin/players
GET /api/admin/players/:id
```

并且必须放在：

```go
adminProtected
```

下面。

### 完整代码

把文件：

```text
E:\game-realtime-gm\backend\internal\router\router.go
```

整文件替换为：

```go
package router

import (
	"net/http"

	"game-realtime-gm/backend/internal/config"
	"game-realtime-gm/backend/internal/handler"
	"game-realtime-gm/backend/internal/middleware"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

func New(db *pgxpool.Pool, redisClient *redis.Client, cfg config.Config) http.Handler {
	r := gin.Default()

	authHandler := handler.NewAuthHandler(db, cfg.JWTSecret)
	adminAuthHandler := handler.NewAdminAuthHandler(db, cfg.JWTSecret)
	adminHandler := handler.NewAdminHandler(db)
	playerHandler := handler.NewPlayerHandler(db)
	onlineHandler := handler.NewOnlineHandler(redisClient)

	r.GET("/health", handler.Health)

	api := r.Group("/api")
	api.POST("/register", authHandler.Register)
	api.POST("/login", authHandler.Login)
	api.POST("/admin/login", adminAuthHandler.Login)

	protected := api.Group("")
	protected.Use(middleware.Auth(cfg.JWTSecret))
	protected.GET("/me", playerHandler.Me)
	protected.PATCH("/me/nickname", playerHandler.UpdateNickname)
	protected.GET("/players", playerHandler.List)
	protected.GET("/players/:id", playerHandler.GetByID)
	protected.POST("/online/heartbeat", onlineHandler.Heartbeat)
	protected.GET("/online/status", onlineHandler.Status)

	adminProtected := api.Group("/admin")
	adminProtected.Use(middleware.AdminAuth(cfg.JWTSecret))
	adminProtected.GET("/me", adminHandler.Me)
	adminProtected.GET("/players", adminHandler.ListPlayers)
	adminProtected.GET("/players/:id", adminHandler.GetPlayerByID)

	return r
}
```

### 代码逐段分析

```go
adminHandler := handler.NewAdminHandler(db)
```

Day 07 是：

```go
handler.NewAdminHandler()
```

Day 08 改成：

```go
handler.NewAdminHandler(db)
```

原因是管理员 handler 现在需要查询数据库。

```go
adminProtected.GET("/players", adminHandler.ListPlayers)
```

注册：

```text
GET /api/admin/players
```

用于管理员分页查询玩家列表。

```go
adminProtected.GET("/players/:id", adminHandler.GetPlayerByID)
```

注册：

```text
GET /api/admin/players/:id
```

用于管理员按 ID 查询玩家详情。

### 和项目其他部分的关系

现在路由分成三类：

公开接口：

```text
GET  /health
POST /api/register
POST /api/login
POST /api/admin/login
```

玩家接口：

```text
GET   /api/me
PATCH /api/me/nickname
GET   /api/players
GET   /api/players/:id
POST  /api/online/heartbeat
GET   /api/online/status
```

管理员接口：

```text
GET /api/admin/me
GET /api/admin/players
GET /api/admin/players/:id
```

今天的重点是把 GM 查询能力放入管理员接口分组。

## 任务 4：格式化、整理依赖并编译

### 任务目标

确认代码格式正确，并且所有 Go 包能编译。

### 在哪里执行

PowerShell。

### 执行命令

```powershell
cd E:\game-realtime-gm\backend
gofmt -w .\internal\handler\admin.go .\internal\router\router.go
go mod tidy
go test ./...
```

### 命令分析

```powershell
gofmt -w ...
```

格式化 Go 文件。

如果你手动复制代码时缩进乱了，`gofmt` 会自动整理。

```powershell
go mod tidy
```

整理依赖。

今天没有新增第三方依赖，但仍建议执行，保持依赖文件干净。

```powershell
go test ./...
```

编译检查所有包。

如果看到：

```text
[no test files]
```

不是错误。

### 常见编译错误

#### 1. `not enough arguments in call to handler.NewAdminHandler`

原因：

你把 `admin.go` 里的构造函数改成了：

```go
func NewAdminHandler(db *pgxpool.Pool) *AdminHandler
```

但 `router.go` 里还写着：

```go
handler.NewAdminHandler()
```

解决：

改成：

```go
handler.NewAdminHandler(db)
```

#### 2. `undefined: parsePositiveInt`

原因：

`player.go` 里的 `parsePositiveInt` 被删掉或改名了。

解决：

确认 `player.go` 里还存在：

```go
func parsePositiveInt(value string, defaultValue int) int
```

#### 3. `imported and not used`

原因：

Go 文件 import 了某个包，但代码里没用。

解决：

删掉未使用 import，然后执行：

```powershell
gofmt -w 报错文件
```

## 任务 5：用 Apifox 测试管理员查询玩家列表

### 任务目标

验证管理员 token 可以访问：

```text
GET /api/admin/players
```

### 第 1 步：启动后端

PowerShell 执行：

```powershell
cd E:\game-realtime-gm\backend
go run .\cmd\server
```

看到：

```text
server listening on :8080
```

说明后端已启动。

### 第 2 步：登录管理员拿 token

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

复制响应里的：

```text
data.token
```

### 第 3 步：请求玩家列表

Apifox 请求：

```text
GET http://localhost:8080/api/admin/players?page=1&page_size=10
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

如果数据库里有玩家，`items` 就会有玩家数据。

### 第 4 步：测试 keyword 搜索

请求：

```text
GET http://localhost:8080/api/admin/players?page=1&page_size=10&keyword=test
```

如果有用户名或昵称包含 `test` 的玩家，就会返回匹配结果。

### 第 5 步：测试 page_size 上限

请求：

```text
GET http://localhost:8080/api/admin/players?page=1&page_size=999
```

预期响应中的：

```json
"page_size": 50
```

因为代码把最大 page size 限制为 50。

## 任务 6：用 Apifox 测试管理员查询玩家详情

### 任务目标

验证：

```text
GET /api/admin/players/:id
```

### 第 1 步：先从列表里找一个玩家 ID

请求：

```text
GET http://localhost:8080/api/admin/players?page=1&page_size=10
```

从响应的 `items` 中复制一个：

```text
id
```

### 第 2 步：按 ID 查询

例如玩家 ID 是 `1`：

```text
GET http://localhost:8080/api/admin/players/1
```

Headers：

```text
Authorization: Bearer 管理员token
```

成功响应：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "id": 1,
    "username": "testplayer",
    "nickname": "测试玩家",
    "created_at": "...",
    "updated_at": "..."
  }
}
```

### 第 3 步：测试不存在的玩家

请求：

```text
GET http://localhost:8080/api/admin/players/999999
```

预期返回：

```json
{
  "code": 40441,
  "message": "player not found"
}
```

### 第 4 步：测试非法 ID

请求：

```text
GET http://localhost:8080/api/admin/players/abc
```

预期返回：

```json
{
  "code": 40041,
  "message": "invalid player id"
}
```

## 任务 7：测试权限隔离

### 任务目标

证明：

```text
只有管理员 token 能访问 GM 玩家查询接口。
```

### 不带 token 测试

请求：

```text
GET http://localhost:8080/api/admin/players
```

不带 Authorization。

预期返回：

```json
{
  "code": 40112,
  "message": "authorization header missing"
}
```

### 用玩家 token 测试

先用普通玩家登录：

```text
POST http://localhost:8080/api/login
```

Body：

```json
{
  "username": "testplayer",
  "password": "123456"
}
```

复制玩家 token。

然后请求：

```text
GET http://localhost:8080/api/admin/players
```

Headers：

```text
Authorization: Bearer 玩家token
```

预期返回：

```json
{
  "code": 40311,
  "message": "admin permission required"
}
```

### 用管理员 token 测试

请求同一个接口：

```text
GET http://localhost:8080/api/admin/players
```

Headers：

```text
Authorization: Bearer 管理员token
```

预期返回：

```json
{
  "code": 0,
  "message": "ok"
}
```

这个对比说明权限隔离成功。

## 任务 8：用 PostgreSQL 验证数据来源

### 任务目标

让你知道接口返回的数据来自哪里。

### 在 Docker Desktop 里执行

进入 `game_realtime_postgres` 容器终端。

执行：

```bash
psql -U game -d game_realtime
```

看到：

```text
game_realtime=#
```

后执行：

```sql
SELECT id, username, nickname, created_at, updated_at
FROM players
ORDER BY id DESC;
```

### 验证什么

你在数据库里看到的玩家数据，应该能在：

```text
GET /api/admin/players
```

响应里找到对应记录。

这说明接口查询的确实是 PostgreSQL 的 `players` 表。

### 退出 psql

```sql
\q
```

## 请求链路复盘

### 管理员查询玩家列表

```text
Apifox
        ↓
GET /api/admin/players
        ↓
router.go
        ↓
middleware.AdminAuth
        ↓
AdminHandler.ListPlayers
        ↓
SELECT COUNT(*) FROM players
        ↓
SELECT ... FROM players LIMIT/OFFSET
        ↓
JSON response
```

### 管理员查询玩家详情

```text
Apifox
        ↓
GET /api/admin/players/:id
        ↓
router.go
        ↓
middleware.AdminAuth
        ↓
AdminHandler.GetPlayerByID
        ↓
SELECT ... FROM players WHERE id = $1
        ↓
JSON response
```

## 今日验收清单

- [ ] `go test ./...` 通过。
- [ ] `GET /api/admin/me` 仍然可用。
- [ ] `AdminHandler` 已经接收数据库连接。
- [ ] `GET /api/admin/players` 已注册。
- [ ] `GET /api/admin/players/:id` 已注册。
- [ ] 管理员 token 可以查询玩家列表。
- [ ] 管理员 token 可以按 ID 查询玩家详情。
- [ ] 不带 token 访问管理员玩家接口返回 401。
- [ ] 玩家 token 访问管理员玩家接口返回 403。
- [ ] 响应中不包含 `password_hash`。
- [ ] 能在 PostgreSQL 里查到接口返回的数据来源。

## 今日不要做

今天不要做：

- 封禁玩家。
- 解封玩家。
- 修改玩家昵称。
- 删除玩家。
- 操作日志。
- RBAC 权限表。
- React GM 后台。
- WebSocket。
- 房间系统。

原因：

今天只做 GM 后台第一个查询闭环：

```text
管理员登录 -> 管理员鉴权 -> 查询玩家列表 -> 查询玩家详情
```

这个闭环跑通后，再做修改类操作才稳。

## 常见错误

### 1. `not enough arguments in call to handler.NewAdminHandler`

原因：

`admin.go` 里的 `NewAdminHandler` 已经要求传入 `db`，但 `router.go` 没改。

解决：

```go
adminHandler := handler.NewAdminHandler(db)
```

### 2. `GET /api/admin/players` 返回 404

原因：

路由没注册。

检查 `router.go` 是否有：

```go
adminProtected.GET("/players", adminHandler.ListPlayers)
```

### 3. 返回 401

原因：

没带 Authorization header，或者格式不对。

正确格式：

```text
Authorization: Bearer 管理员token
```

### 4. 返回 403

原因：

你带的是玩家 token，不是管理员 token。

解决：

调用：

```text
POST /api/admin/login
```

重新复制管理员 token。

### 5. 返回空列表

这不一定是错误。

可能是 `players` 表里没有玩家，或者 keyword 没匹配到。

可以在 PostgreSQL 里执行：

```sql
SELECT id, username, nickname FROM players;
```

确认是否有数据。

### 6. `password_hash` 出现在响应中

这是需要立刻修的问题。

GM 查询玩家接口不应该返回密码哈希。

检查 SQL 是否只查询：

```text
id, username, nickname, created_at, updated_at
```

不要查询：

```text
password_hash
```

## 今天学完后你应该能说清楚

- 为什么玩家列表查询更像 GM 后台功能。
- 为什么 GM 查询接口必须使用管理员鉴权。
- `AdminHandler` 为什么需要数据库连接。
- `LIMIT` 和 `OFFSET` 如何实现分页。
- `COUNT(*)` 为什么用于分页总数。
- 为什么响应不能返回 `password_hash`。
- 为什么今天不直接做封禁玩家。

## 面试怎么讲这一部分

可以这样说：

```text
在完成管理员登录和管理员鉴权后，我开始实现 GM 后台的第一个业务能力：玩家查询。接口设计为 /api/admin/players 和 /api/admin/players/:id，并挂在 AdminAuth 中间件后面，只有管理员 token 能访问。列表接口支持 page、page_size 和 keyword，使用 COUNT(*) 返回分页总数，并通过 LIMIT/OFFSET 查询当前页数据。接口只返回玩家公开资料，不返回 password_hash 等敏感字段。通过这个功能，我把 JWT 权限隔离、Gin 路由分组、PostgreSQL 查询和后台管理场景串成了一个完整闭环。
```

## 明日预告

Day 09 建议做：

```text
GM 操作日志基础
```

原因：

当管理员开始做查询、封禁、修改等操作时，系统需要知道：

```text
谁在什么时候做了什么
```

这会为后续封禁玩家、修改玩家状态和 GM 后台审计打基础。
