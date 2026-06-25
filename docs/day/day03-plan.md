# Day 03 学习计划：JWT 鉴权、当前玩家接口

## 今日目标

今天要把 Day 02 的“登录成功拿到 token”继续往下接，做出真正的身份识别闭环。

完成后，项目要能做到：

- 登录接口返回 JWT token。
- 后端能从 `Authorization: Bearer xxx` 里解析 token。
- 后端能识别当前请求对应哪个玩家。
- 后端能保护一个需要登录才能访问的接口。

今天的核心不是再写一套新业务，而是把“登录之后怎么证明自己是谁”这件事做完整。

## 当前项目状态

你现在已经完成：

- `POST /api/register`
- `POST /api/login`
- JWT 生成
- PostgreSQL 玩家表
- `players` 表和 `schema.sql`

所以 Day 03 的重点是：

```text
登录成功
    ↓
拿到 token
    ↓
请求受保护接口时带上 token
    ↓
后端解析 token
    ↓
后端知道当前玩家是谁
```

## 今天会学到什么

- JWT 如何解析。
- `Authorization: Bearer xxx` 是怎么工作的。
- Gin 中间件是什么。
- 为什么受保护接口要先过鉴权。
- 如何把登录信息放进 `gin.Context`。
- 如何用 token 查出当前玩家信息。

## 今日最终效果

今天做完后，你应该能完成这条链路：

```text
POST /api/login
        ↓
返回 token
        ↓
GET /api/me
        ↓
请求头带 Authorization: Bearer token
        ↓
后端返回当前登录玩家信息
```

## 任务 1：给 JWT 增加解析能力

### 任务目标

让项目不仅能“生成 token”，还能够“解析 token”。

### 你要新增或修改什么

修改：

```text
E:\game-realtime-gm\backend\internal\auth\jwt.go
```

### 这个操作有什么用

Day 02 里已经可以生成 token，但后端还不知道如何检查 token。

如果没有解析能力，登录后客户端带来的 token 就只是字符串，后端无法判断它是谁签的、是否过期、里面装了谁的信息。

### 这个文件负责什么

`internal/auth/jwt.go` 以后负责所有 JWT 相关逻辑：

- 生成 token
- 解析 token
- 校验 token 是否过期
- 提取 claims

### 具体操作步骤

打开这个文件：

```text
E:\game-realtime-gm\backend\internal\auth\jwt.go
```

这次不是新建，而是**在现有文件上增加解析函数**。

你要在当前 `GenerateToken` 的基础上，新增一个 `ParseToken` 函数。

### 代码内容

把文件内容更新为：

```go
package auth

import (
	"fmt"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	PlayerID int64  `json:"player_id"`
	Username string `json:"username"`
	jwt.RegisteredClaims
}

func GenerateToken(secret string, playerID int64, username string) (string, error) {
	claims := Claims{
		PlayerID: playerID,
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatInt(playerID, 10),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

func ParseToken(secret string, tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	return claims, nil
}
```

### 代码分析

```go
func ParseToken(secret string, tokenString string) (*Claims, error)
```

这个函数的作用是把 token 字符串解析成 claims。

- `secret`：签发 token 时用的密钥。
- `tokenString`：客户端传来的 token。
- `*Claims`：解析后得到的玩家信息。
- `error`：解析失败时返回错误。

```go
jwt.ParseWithClaims
```

表示让 JWT 库按我们自己的 `Claims` 结构体去解析 token。

```go
if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok
```

这一步是检查签名算法是不是 HMAC。

这样做是为了避免 token 使用了别的算法却被误接受。

```go
if !ok || !token.Valid
```

如果 claims 类型不对，或者 token 已经过期、签名错误，就返回失败。

### 和项目其他部分的关系

后面的鉴权中间件会调用 `ParseToken`。

也就是说：

```text
middleware -> auth.ParseToken -> Claims
```

### 怎么验证成功

后面完成登录后，你可以拿 token 去调用 `/api/me`。

如果 token 有效，就能拿到当前玩家信息。

### 常见错误

错误 1：

```text
unexpected signing method
```

原因：客户端传来的 token 签名算法不对。

解决：只接受当前项目生成的 HS256 token。

错误 2：

```text
token is malformed
```

原因：token 字符串被复制错了，或者带上了多余空格。

解决：检查 `Authorization` 头里是否真的只有 `Bearer xxx`。

## 任务 2：创建鉴权中间件

### 任务目标

让受保护接口在进入 handler 之前，先检查请求有没有携带有效 token。

### 你要新增或修改什么

新增：

```text
E:\game-realtime-gm\backend\internal\middleware\auth.go
```

如果 `middleware` 目录不存在，先创建目录：

```powershell
cd E:\game-realtime-gm\backend
mkdir internal\middleware
```

### 这个操作有什么用

如果每个受保护接口都自己写一遍“取 header、解析 token、查 claims”的逻辑，代码会重复很多。

中间件的作用是把“所有受保护接口共同需要的前置检查”集中起来。

这样做的好处是：

- 代码不重复。
- 错误处理统一。
- 以后接入更多接口时更容易。

### 这个文件负责什么

`internal/middleware/auth.go` 负责：

- 从请求头读取 `Authorization`
- 提取 Bearer token
- 调用 JWT 解析函数
- 把 `player_id` 和 `username` 放进 `gin.Context`
- token 无效时直接返回 401

### 具体操作步骤

在 `internal/middleware/auth.go` 写入完整内容：

```go
package middleware

import (
	"net/http"
	"strings"

	tokenauth "game-realtime-gm/backend/internal/auth"

	"github.com/gin-gonic/gin"
)

const (
	ContextKeyPlayerID = "player_id"
	ContextKeyUsername = "username"
)

func Auth(jwtSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if header == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":    40102,
				"message": "authorization header missing",
			})
			c.Abort()
			return
		}

		tokenString := strings.TrimSpace(strings.TrimPrefix(header, "Bearer"))
		tokenString = strings.TrimSpace(tokenString)
		if tokenString == "" || tokenString == header {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":    40103,
				"message": "invalid authorization header",
			})
			c.Abort()
			return
		}

		claims, err := tokenauth.ParseToken(jwtSecret, tokenString)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":    40104,
				"message": "invalid token",
			})
			c.Abort()
			return
		}

		c.Set(ContextKeyPlayerID, claims.PlayerID)
		c.Set(ContextKeyUsername, claims.Username)
		c.Next()
	}
}
```

### 代码分析

```go
func Auth(jwtSecret string) gin.HandlerFunc
```

这是一个 Gin 中间件工厂函数。

它接收 JWT 密钥，返回一个可以挂到路由上的中间件。

```go
header := c.GetHeader("Authorization")
```

从 HTTP 请求头里读取 `Authorization`。

客户端后续会这么传：

```text
Authorization: Bearer eyJhbGciOiJIUzI1NiIs...
```

```go
strings.TrimPrefix(header, "Bearer")
```

把 `Bearer ` 前缀去掉，拿到真正的 token 字符串。

```go
c.Set(ContextKeyPlayerID, claims.PlayerID)
```

把解析出的玩家 ID 放进 `gin.Context`。

这样后面的 handler 就能直接拿到当前玩家身份。

### 和项目其他部分的关系

这个中间件会被挂在受保护路由上，比如：

```text
GET /api/me
```

只有 token 正常，handler 才会执行。

### 怎么验证成功

没有带 token 时，请求 `/api/me` 应该返回 401。

带上正确 token 时，应该能进入 handler。

### 常见错误

错误 1：

```text
authorization header missing
```

原因：请求没有带 `Authorization` 头。

解决：在 Apifox 里手动添加请求头。

错误 2：

```text
invalid authorization header
```

原因：`Authorization` 格式不对。

正确格式必须是：

```text
Bearer <token>
```

错误 3：

```text
invalid token
```

原因：token 过期、被改过、或者签名不对。

## 任务 3：创建“当前玩家”接口

### 任务目标

实现 `GET /api/me`，返回当前登录玩家的信息。

### 你要新增或修改什么

新增：

```text
E:\game-realtime-gm\backend\internal\handler\player.go
```

### 这个操作有什么用

登录后返回 token 还不够。

真实项目里，客户端通常还需要一个“当前我是谁”的接口，用来：

- 显示头像和昵称
- 显示登录状态
- 进入页面后重新拉取当前账号信息

### 这个文件负责什么

`internal/handler/player.go` 负责和“当前玩家”相关的接口。

今天先做最小版本：

- 读取中间件塞进 context 的 `player_id`
- 根据 `player_id` 去数据库查玩家信息
- 返回玩家公开数据

### 具体操作步骤

创建文件：

```text
E:\game-realtime-gm\backend\internal\handler\player.go
```

写入完整内容：

```go
package handler

import (
	"net/http"

	"game-realtime-gm/backend/internal/model"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PlayerHandler struct {
	db *pgxpool.Pool
}

func NewPlayerHandler(db *pgxpool.Pool) *PlayerHandler {
	return &PlayerHandler{db: db}
}

func (h *PlayerHandler) Me(c *gin.Context) {
	playerIDValue, exists := c.Get("player_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    40105,
			"message": "player identity missing",
		})
		return
	}

	playerID, ok := playerIDValue.(int64)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    40106,
			"message": "invalid player identity",
		})
		return
	}

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
```

### 代码分析

```go
playerIDValue, exists := c.Get("player_id")
```

从 `gin.Context` 里取出中间件放进去的玩家 ID。

```go
playerID, ok := playerIDValue.(int64)
```

把 `interface{}` 转回 `int64`。

```go
SELECT id, username, nickname, created_at, updated_at
FROM players
WHERE id = $1
```

根据玩家 ID 重新查询数据库。

这样可以确保返回的是数据库里最新的数据。

### 和项目其他部分的关系

这一步把 Day 02 的登录和 Day 03 的鉴权真正串起来了：

```text
login -> token -> middleware -> Me handler -> database
```

### 怎么验证成功

带 token 请求 `/api/me`，应该返回当前登录玩家信息。

不带 token 请求 `/api/me`，应该返回 401。

### 常见错误

错误 1：

```text
player identity missing
```

原因：请求没有经过鉴权中间件。

解决：确认路由组里挂了 `middleware.Auth(...)`。

错误 2：

```text
invalid player identity
```

原因：中间件写入 context 的类型和 handler 读取的类型不一致。

解决：中间件和 handler 统一使用 `int64`。

## 任务 4：把鉴权中间件挂到路由上

### 任务目标

让 `/api/me` 这样的接口只有登录后才能访问。

### 你要新增或修改什么

修改：

```text
E:\game-realtime-gm\backend\internal\router\router.go
E:\game-realtime-gm\backend\cmd\server\main.go
```

### 这个操作有什么用

中间件写好了，还要真正挂到路由上才会生效。

这一步的意义是把“鉴权规则”应用到受保护接口上。

### 代码操作

把 `router.go` 整文件替换为：

```go
package router

import (
	"net/http"

	"game-realtime-gm/backend/internal/config"
	"game-realtime-gm/backend/internal/handler"
	"game-realtime-gm/backend/internal/middleware"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func New(db *pgxpool.Pool, cfg config.Config) http.Handler {
	r := gin.Default()

	authHandler := handler.NewAuthHandler(db, cfg.JWTSecret)
	playerHandler := handler.NewPlayerHandler(db)

	r.GET("/health", handler.Health)

	api := r.Group("/api")
	api.POST("/register", authHandler.Register)
	api.POST("/login", authHandler.Login)

	protected := api.Group("")
	protected.Use(middleware.Auth(cfg.JWTSecret))
	protected.GET("/me", playerHandler.Me)

	return r
}
```

把 `main.go` 里的这一行：

```go
r := router.New(dbPool, cfg)
```

保持不变。

### 代码分析

```go
protected := api.Group("")
protected.Use(middleware.Auth(cfg.JWTSecret))
```

这表示在 `/api` 下面再分出一个受保护路由组。

只有通过 `middleware.Auth` 的请求，才会继续往下走。

```go
protected.GET("/me", playerHandler.Me)
```

这样完整路径就是：

```text
GET /api/me
```

### 怎么验证成功

没有 token 请求 `/api/me`，返回 401。

有 token 请求 `/api/me`，返回玩家数据。

### 常见错误

错误 1：

```text
undefined: middleware.Auth
```

原因：中间件文件没有建好，或者 package 名不对。

错误 2：

```text
not enough arguments in call to router.New
```

原因：`main.go` 没有按新签名传入 `cfg`。

## 任务 5：整理格式、依赖和验证

### 任务目标

让代码格式统一，依赖完整，项目能编译通过。

### 具体操作步骤

在后端目录执行：

```powershell
cd E:\game-realtime-gm\backend
gofmt -w .\internal\auth\jwt.go .\internal\middleware\auth.go .\internal\handler\player.go .\internal\router\router.go .\cmd\server\main.go
go mod tidy
go test ./...
```

### 命令分析

- `gofmt -w`：格式化刚才修改的 Go 文件。
- `go mod tidy`：补齐依赖并清理无用依赖。
- `go test ./...`：编译整个项目，顺便检查语法和依赖。

### 怎么验证成功

成功时应该看到所有包都通过编译，没有新的语法错误。

## 今日验收清单

- [ ] `internal/auth/jwt.go` 增加了解析 token 的能力。
- [ ] 新建了 JWT 鉴权中间件。
- [ ] 新建了 `/api/me` 接口。
- [ ] `/api/me` 必须带 token 才能访问。
- [ ] 登录成功后能拿到 token。
- [ ] 用 token 请求 `/api/me` 能返回当前玩家信息。
- [ ] `go test ./...` 编译通过。

## 今日不要做

今天不要做：

- 房间系统
- WebSocket 消息收发
- 匹配队列
- Redis 排行榜
- React GM 后台
- 角色权限系统

今天的目标是把“登录后如何识别身份”先做稳。

## 常见问题

### 1. token 能生成但不能解析

检查：

- `GenerateToken` 和 `ParseToken` 是否使用同一个 `secret`
- token 是否已经过期
- `Authorization` 头格式是否正确

### 2. `/api/me` 返回 401

检查：

- 有没有带 `Authorization: Bearer xxx`
- 路由有没有挂中间件
- token 是否从登录接口真正复制过来

### 3. context 里取不到 `player_id`

检查：

- 中间件里是否调用了 `c.Set("player_id", ...)`
- handler 里取的 key 是否完全一致

## 今天学完后你应该能说清楚

今天结束时，你应该能回答：

- JWT 不是登录本身，而是登录后的身份凭证。
- 中间件负责在 handler 前做统一鉴权。
- `gin.Context` 可以在中间件和 handler 之间传递数据。
- 登录接口只负责发 token，真正使用 token 的是后续受保护接口。
- `/api/me` 是一个很典型的“需要登录身份才能访问”的接口。

