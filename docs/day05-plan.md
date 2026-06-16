# Day 05 学习计划：玩家资料模块、查询与分页

## 今日目标

今天要把玩家模块从“只能查看当前登录玩家”扩展成一个更完整的基础玩家模块。

完成后，项目会新增这些能力：

- 当前玩家可以修改自己的昵称。
- 可以根据玩家 ID 查询玩家公开信息。
- 可以分页查询玩家列表。
- 可以练习 SQL 的 `SELECT`、`UPDATE`、`LIMIT`、`OFFSET`。
- 可以理解接口里的 query 参数和 path 参数。

今天的核心不是复杂权限，而是把“玩家资料”这条业务线做得更像真实项目。

## 当前项目状态

你现在已经完成：

- 注册接口：`POST /api/register`
- 登录接口：`POST /api/login`
- 当前玩家接口：`GET /api/me`
- JWT 鉴权中间件
- Redis 在线状态接口
- PostgreSQL 中的 `players` 表

Day 05 会继续使用这些基础。

## 今天会学到什么

- 什么是 path 参数，例如 `/api/players/:id`
- 什么是 query 参数，例如 `/api/players?page=1&page_size=10`
- SQL 查询和更新
- 分页为什么需要 `LIMIT` 和 `OFFSET`
- 为什么接口不能返回 `password_hash`
- 如何复用中间件里的当前玩家 ID

## 今日最终效果

今天做完后，后端新增或完善这些接口：

```text
GET   /api/me
PATCH /api/me/nickname
GET   /api/players/:id
GET   /api/players?page=1&page_size=10&keyword=test
```

这些接口都放在受保护路由下，需要携带：

```text
Authorization: Bearer <token>
```

## 任务 0：先验证 Day 04

### 任务目标

确认当前项目能编译，后端能启动，Redis 和 PostgreSQL 都正常。

### 具体操作步骤

打开 Docker Desktop，然后执行：

```powershell
cd E:\game-realtime-gm\deploy
docker compose up -d
docker ps
```

再执行：

```powershell
cd E:\game-realtime-gm\backend
go test ./...
go run .\cmd\server
```

### 怎么验证成功

后端日志应该看到：

```text
database connected
redis connected
server listening on :8080
```

如果这里失败，先不要继续 Day 05。

## 任务 1：整理当前玩家 ID 获取逻辑

### 任务目标

把“从 `gin.Context` 里取当前玩家 ID”的逻辑放到中间件包里，方便多个 handler 复用。

### 你要新增或修改什么

修改：

```text
E:\game-realtime-gm\backend\internal\middleware\auth.go
```

操作类型：在文件末尾新增函数。

### 这个操作有什么用

Day 03 和 Day 04 都需要当前玩家 ID。

如果每个 handler 都自己写一遍：

```go
playerIDValue, exists := c.Get("player_id")
```

代码会重复，而且容易写错 key。

把它整理成函数后，其他 handler 可以直接调用：

```go
middleware.CurrentPlayerID(c)
```

### 代码操作

打开：

```text
E:\game-realtime-gm\backend\internal\middleware\auth.go
```

在文件末尾新增：

```go
func CurrentPlayerID(c *gin.Context) (int64, bool) {
	playerIDValue, exists := c.Get(ContextKeyPlayerID)
	if !exists {
		return 0, false
	}

	playerID, ok := playerIDValue.(int64)
	if !ok {
		return 0, false
	}

	return playerID, true
}
```

### 代码分析

```go
func CurrentPlayerID(c *gin.Context) (int64, bool)
```

这个函数从 Gin 上下文中读取当前玩家 ID。

返回值：

- `int64`：玩家 ID。
- `bool`：是否成功取到。

```go
c.Get(ContextKeyPlayerID)
```

读取鉴权中间件之前写入的 `player_id`。

```go
playerIDValue.(int64)
```

把 `interface{}` 类型转换成 `int64`。

### 和项目其他部分的关系

后面的玩家资料接口和在线状态接口都可以复用这个函数。

### 怎么验证成功

执行：

```powershell
cd E:\game-realtime-gm\backend
gofmt -w .\internal\middleware\auth.go
go test ./...
```

没有 `FAIL` 就成功。

## 任务 2：更新在线状态 handler 使用公共函数

### 任务目标

把 Day 04 中 `online.go` 里的重复取玩家 ID 逻辑删掉，改成使用 `middleware.CurrentPlayerID`。

### 你要新增或修改什么

修改：

```text
E:\game-realtime-gm\backend\internal\handler\online.go
```

操作类型：整文件替换。

### 这个操作有什么用

这是一个小重构。

它不会改变接口功能，但能让代码更清晰：

```text
鉴权相关的上下文读取逻辑
        ↓
放在 middleware 包

在线状态业务逻辑
        ↓
放在 online.go
```

### 代码内容

把 `online.go` 替换为：

```go
package handler

import (
	"fmt"
	"net/http"
	"time"

	"game-realtime-gm/backend/internal/middleware"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

const onlineTTL = 2 * time.Minute

type OnlineHandler struct {
	redisClient *redis.Client
}

func NewOnlineHandler(redisClient *redis.Client) *OnlineHandler {
	return &OnlineHandler{redisClient: redisClient}
}

func (h *OnlineHandler) Heartbeat(c *gin.Context) {
	playerID, ok := requireCurrentPlayerID(c)
	if !ok {
		return
	}

	key := onlinePlayerKey(playerID)
	if err := h.redisClient.Set(c.Request.Context(), key, "1", onlineTTL).Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50020,
			"message": "update online status failed",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "heartbeat success",
		"data": gin.H{
			"online":      true,
			"ttl_seconds": int(onlineTTL.Seconds()),
		},
	})
}

func (h *OnlineHandler) Status(c *gin.Context) {
	playerID, ok := requireCurrentPlayerID(c)
	if !ok {
		return
	}

	key := onlinePlayerKey(playerID)
	exists, err := h.redisClient.Exists(c.Request.Context(), key).Result()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50021,
			"message": "query online status failed",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "ok",
		"data": gin.H{
			"online": exists > 0,
		},
	})
}

func requireCurrentPlayerID(c *gin.Context) (int64, bool) {
	playerID, ok := middleware.CurrentPlayerID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    40105,
			"message": "player identity missing",
		})
		return 0, false
	}

	return playerID, true
}

func onlinePlayerKey(playerID int64) string {
	return fmt.Sprintf("online:player:%d", playerID)
}
```

### 代码分析

```go
requireCurrentPlayerID(c)
```

这是 handler 层的小工具函数。

它调用：

```go
middleware.CurrentPlayerID(c)
```

如果取不到玩家 ID，就直接返回 401。

### 怎么验证成功

执行：

```powershell
cd E:\game-realtime-gm\backend
gofmt -w .\internal\handler\online.go
go test ./...
```

`/api/online/heartbeat` 和 `/api/online/status` 的功能应该和 Day 04 一样。

## 任务 3：扩展玩家 handler

### 任务目标

让 `player.go` 支持：

- 查询当前玩家
- 修改当前玩家昵称
- 根据 ID 查询玩家
- 分页查询玩家列表

### 你要新增或修改什么

修改：

```text
E:\game-realtime-gm\backend\internal\handler\player.go
```

操作类型：整文件替换。

### 这个操作有什么用

真实后台系统经常需要查询玩家。

比如：

- 玩家自己查看资料
- GM 查询某个玩家
- GM 按用户名或昵称搜索玩家
- 前端表格分页展示玩家列表

今天先做基础版本，不做复杂权限。

### 代码内容

把 `player.go` 替换为：

```go
package handler

import (
	"net/http"
	"strconv"
	"strings"

	"game-realtime-gm/backend/internal/middleware"
	"game-realtime-gm/backend/internal/model"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PlayerHandler struct {
	db *pgxpool.Pool
}

type updateNicknameRequest struct {
	Nickname string `json:"nickname" binding:"required,max=64"`
}

func NewPlayerHandler(db *pgxpool.Pool) *PlayerHandler {
	return &PlayerHandler{db: db}
}

func (h *PlayerHandler) Me(c *gin.Context) {
	playerID, ok := requireCurrentPlayerID(c)
	if !ok {
		return
	}

	player, err := h.findPlayerByID(c, playerID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50010,
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

func (h *PlayerHandler) UpdateNickname(c *gin.Context) {
	playerID, ok := requireCurrentPlayerID(c)
	if !ok {
		return
	}

	var req updateNicknameRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "invalid request",
		})
		return
	}

	nickname := strings.TrimSpace(req.Nickname)
	if nickname == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40003,
			"message": "nickname cannot be empty",
		})
		return
	}

	var player model.Player
	err := h.db.QueryRow(
		c.Request.Context(),
		`UPDATE players
		 SET nickname = $1, updated_at = NOW()
		 WHERE id = $2
		 RETURNING id, username, nickname, created_at, updated_at`,
		nickname,
		playerID,
	).Scan(
		&player.ID,
		&player.Username,
		&player.Nickname,
		&player.CreatedAt,
		&player.UpdatedAt,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50011,
			"message": "update nickname failed",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "update nickname success",
		"data":    player,
	})
}

func (h *PlayerHandler) GetByID(c *gin.Context) {
	playerID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || playerID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40004,
			"message": "invalid player id",
		})
		return
	}

	player, err := h.findPlayerByID(c, playerID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50012,
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

func (h *PlayerHandler) List(c *gin.Context) {
	page := parsePositiveInt(c.DefaultQuery("page", "1"), 1)
	pageSize := parsePositiveInt(c.DefaultQuery("page_size", "10"), 10)
	if pageSize > 50 {
		pageSize = 50
	}

	keyword := strings.TrimSpace(c.Query("keyword"))
	offset := (page - 1) * pageSize

	args := []any{int32(pageSize), int32(offset)}
	whereSQL := ""
	if keyword != "" {
		whereSQL = "WHERE username ILIKE $3 OR nickname ILIKE $3"
		args = append(args, "%"+keyword+"%")
	}

	rows, err := h.db.Query(
		c.Request.Context(),
		`SELECT id, username, nickname, created_at, updated_at
		 FROM players
		 `+whereSQL+`
		 ORDER BY id DESC
		 LIMIT $1 OFFSET $2`,
		args...,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50013,
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
				"code":    50014,
				"message": "scan player failed",
			})
			return
		}
		players = append(players, player)
	}

	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50015,
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
		},
	})
}

func (h *PlayerHandler) findPlayerByID(c *gin.Context, playerID int64) (model.Player, error) {
	var player model.Player
	err := h.db.QueryRow(
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
	return player, err
}

func parsePositiveInt(value string, defaultValue int) int {
	parsedValue, err := strconv.Atoi(value)
	if err != nil || parsedValue <= 0 {
		return defaultValue
	}
	return parsedValue
}

func requireCurrentPlayerID(c *gin.Context) (int64, bool) {
	playerID, ok := middleware.CurrentPlayerID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    40105,
			"message": "player identity missing",
		})
		return 0, false
	}

	return playerID, true
}
```

### 代码分析

```go
type updateNicknameRequest struct
```

表示修改昵称接口的请求 JSON。

```go
c.ShouldBindJSON(&req)
```

把请求 body 里的 JSON 解析到结构体中，并执行 `binding` 校验。

```go
c.Param("id")
```

读取 path 参数。

例如：

```text
GET /api/players/1
```

这里的 `1` 就是 `id`。

```go
c.Query("keyword")
```

读取 query 参数。

例如：

```text
GET /api/players?keyword=test
```

这里的 `test` 就是 `keyword`。

```sql
LIMIT $1 OFFSET $2
```

用于分页。

- `LIMIT`：本次最多查多少条。
- `OFFSET`：跳过前面多少条。

如果：

```text
page = 2
page_size = 10
```

那么：

```text
offset = 10
```

表示跳过前 10 条，查询第 11 到第 20 条。

```sql
ILIKE
```

PostgreSQL 中的大小写不敏感模糊查询。

### 注意

今天这个玩家列表接口先放在受保护路由下。

后面做 GM 管理员和权限时，再把“谁能查询玩家列表”做得更严格。

## 任务 4：更新路由

### 任务目标

把今天新增的玩家接口注册到 Gin 路由里。

### 你要新增或修改什么

修改：

```text
E:\game-realtime-gm\backend\internal\router\router.go
```

操作类型：整文件替换。

### 代码内容

把 `router.go` 替换为：

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
	playerHandler := handler.NewPlayerHandler(db)
	onlineHandler := handler.NewOnlineHandler(redisClient)

	r.GET("/health", handler.Health)

	api := r.Group("/api")
	api.POST("/register", authHandler.Register)
	api.POST("/login", authHandler.Login)

	protected := api.Group("")
	protected.Use(middleware.Auth(cfg.JWTSecret))
	protected.GET("/me", playerHandler.Me)
	protected.PATCH("/me/nickname", playerHandler.UpdateNickname)
	protected.GET("/players", playerHandler.List)
	protected.GET("/players/:id", playerHandler.GetByID)
	protected.POST("/online/heartbeat", onlineHandler.Heartbeat)
	protected.GET("/online/status", onlineHandler.Status)

	return r
}
```

### 代码分析

```go
protected.PATCH("/me/nickname", playerHandler.UpdateNickname)
```

注册修改当前玩家昵称接口。

使用 `PATCH`，因为它表示局部更新资源。

```go
protected.GET("/players", playerHandler.List)
```

注册玩家列表接口。

```go
protected.GET("/players/:id", playerHandler.GetByID)
```

注册按 ID 查询玩家接口。

`:id` 是 Gin 的 path 参数。

### 怎么验证成功

执行：

```powershell
cd E:\game-realtime-gm\backend
gofmt -w .\internal\router\router.go
go test ./...
```

无 `FAIL` 即成功。

## 任务 5：格式化、编译并启动

### 任务目标

确保 Day 05 的代码能编译，并启动后端。

### 具体操作步骤

执行：

```powershell
cd E:\game-realtime-gm\backend
gofmt -w .\internal\middleware\auth.go .\internal\handler\online.go .\internal\handler\player.go .\internal\router\router.go
go mod tidy
go test ./...
```

如果成功，启动后端：

```powershell
go run .\cmd\server
```

### 怎么验证成功

日志应该包含：

```text
database connected
redis connected
server listening on :8080
```

## 任务 6：用 Apifox 测试玩家接口

### 任务目标

确认玩家资料接口真的可用。

### 第 1 步：登录拿 token

请求：

```text
POST http://localhost:8080/api/login
```

Body：

```json
{
  "username": "test001",
  "password": "123456"
}
```

复制返回的：

```text
data.token
```

后续所有请求都加 Header：

```text
Authorization: Bearer 你的token
```

### 第 2 步：查看当前玩家

请求：

```text
GET http://localhost:8080/api/me
```

成功返回当前玩家信息。

### 第 3 步：修改昵称

请求：

```text
PATCH http://localhost:8080/api/me/nickname
```

Body：

```json
{
  "nickname": "新昵称"
}
```

成功返回：

```json
{
  "code": 0,
  "message": "update nickname success"
}
```

### 第 4 步：按 ID 查询玩家

请求：

```text
GET http://localhost:8080/api/players/1
```

这里的 `1` 换成你自己的玩家 ID。

### 第 5 步：分页查询玩家列表

请求：

```text
GET http://localhost:8080/api/players?page=1&page_size=10
```

带关键词查询：

```text
GET http://localhost:8080/api/players?page=1&page_size=10&keyword=test
```

### 怎么验证成功

玩家列表应该返回：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "items": [],
    "page": 1,
    "page_size": 10
  }
}
```

如果数据库里有玩家，`items` 中会有玩家数据。

## 任务 7：用 PostgreSQL 检查数据

### 任务目标

确认修改昵称后，数据库里的数据真的变了。

### 具体操作步骤

PowerShell 连接 PostgreSQL：

```powershell
psql -h localhost -p 15432 -U game -d game_realtime
```

输入密码：

```text
game123456
```

查询：

```sql
SELECT id, username, nickname, updated_at
FROM players
ORDER BY id DESC;
```

### 怎么验证成功

你应该看到昵称已经变成刚才接口提交的新昵称。

退出：

```sql
\q
```

## 今日验收清单

- [ ] `go test ./...` 通过。
- [ ] `GET /api/me` 正常。
- [ ] `PATCH /api/me/nickname` 能修改昵称。
- [ ] PostgreSQL 中的昵称真的更新。
- [ ] `GET /api/players/:id` 能按 ID 查询玩家。
- [ ] `GET /api/players?page=1&page_size=10` 能分页查询玩家。
- [ ] 所有玩家接口都需要 token。

## 今日不要做

今天不要做：

- GM 管理员账号
- RBAC 权限
- WebSocket
- 房间系统
- 匹配队列
- 排行榜

这些都可以后面继续做。今天只把玩家基础资料接口打稳。

## 常见错误

### 1. 请求返回 401

原因：没有带 token，或者 token 格式不对。

正确 Header：

```text
Authorization: Bearer 你的token
```

### 2. `PATCH /api/me/nickname` 返回 invalid request

原因：Body 不是 JSON，或者缺少 `nickname` 字段。

正确 Body：

```json
{
  "nickname": "新昵称"
}
```

### 3. `GET /api/players/:id` 返回 invalid player id

原因：路径里的 id 不是正整数。

正确示例：

```text
GET /api/players/1
```

错误示例：

```text
GET /api/players/abc
```

### 4. 玩家列表为空

这不一定是错误。

原因可能是：

- 数据库里还没有玩家。
- `keyword` 条件没有匹配到玩家。

先注册几个账号再查。

## 今天学完后你应该能说清楚

- path 参数和 query 参数的区别。
- 为什么修改昵称使用 `PATCH`。
- `LIMIT` 和 `OFFSET` 如何实现分页。
- 为什么接口不能返回 `password_hash`。
- 玩家资料接口为什么需要 JWT 鉴权。
- 当前玩家接口和按 ID 查询玩家接口有什么区别。

## 面试怎么讲这一部分

可以这样说：

```text
账号模块完成后，我继续补充了玩家资料模块，实现当前玩家查询、昵称更新、按 ID 查询玩家和分页查询玩家列表。接口统一走 JWT 鉴权中间件，通过 gin.Context 获取当前 player_id。数据库查询使用参数化 SQL，避免 SQL 注入；玩家列表使用 LIMIT/OFFSET 做基础分页，并且接口响应中不会返回 password_hash 等敏感字段。
```

