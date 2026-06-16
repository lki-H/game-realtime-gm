# Day 07 学习计划：GM 管理员鉴权中间件与受保护接口

## 今日目标

今天承接 Day 06 已经跑通的管理员登录。

Day 06 已经完成：

```text
POST /api/admin/login
```

管理员可以用账号密码登录，并拿到一个管理员 JWT。

但是现在还缺一个关键闭环：

```text
管理员 token 拿到以后，到底能访问哪些 GM 接口？
```

所以 Day 07 的目标是：

1. 给项目新增 GM 管理员鉴权中间件。
2. 让管理员 token 和玩家 token 分开使用。
3. 新增一个最小 GM 受保护接口：

```text
GET /api/admin/me
```

完成后，你可以用 Day 06 登录得到的管理员 token 访问 `GET /api/admin/me`，验证这个 token 确实是管理员身份。

## 当前项目状态

你现在已经完成：

- PostgreSQL 和 Redis 容器。
- `players` 表。
- `admins` 表。
- 玩家注册和登录。
- 玩家 JWT。
- 管理员登录。
- 管理员 JWT。
- 玩家鉴权中间件。
- 玩家资料接口。
- Redis 在线状态接口。

当前代码中已经有：

```text
E:\game-realtime-gm\backend\internal\auth\jwt.go
```

里面已经定义：

```go
SubjectTypePlayer = "player"
SubjectTypeAdmin  = "admin"
```

也就是说 token 里已经能区分：

```text
这是玩家 token
这是管理员 token
```

今天要做的是让中间件也认识这个区别。

## 今天会学到什么

今天会学到：

- 为什么“登录成功”和“有权限访问接口”不是一回事。
- JWT 里的 `subject_type` 有什么用。
- 为什么玩家 token 不能访问 GM 接口。
- 为什么管理员 token 也不应该访问玩家自己的接口。
- Gin 中间件如何在请求进入 handler 前拦截请求。
- 如何把管理员身份写入 `gin.Context`。
- 如何设计第一个 GM 受保护接口。

一句话理解：

```text
登录负责签发 token，鉴权中间件负责检查 token，权限控制负责判断这个 token 能不能做某件事。
```

## 今日最终效果

完成后，项目新增：

```text
GET /api/admin/me
```

调用方式：

```text
Authorization: Bearer 管理员token
```

成功响应类似：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "id": 1,
    "username": "admin",
    "role": "super_admin"
  }
}
```

如果不带 token：

```json
{
  "code": 40112,
  "message": "authorization header missing"
}
```

如果拿玩家 token 调用 GM 接口：

```json
{
  "code": 40311,
  "message": "admin permission required"
}
```

这说明后端已经开始区分“玩家接口”和“管理员接口”。

## 任务 0：先验证 Day 06 状态

### 任务目标

确认 Day 06 的管理员登录仍然成功。

如果管理员登录还没成功，今天不能继续做管理员鉴权。因为鉴权依赖管理员 token。

### 在哪里执行

PowerShell。

### 执行命令

```powershell
cd E:\game-realtime-gm\backend
go test ./...
```

### 命令作用

- `cd E:\game-realtime-gm\backend`：进入 Go 后端项目目录。
- `go test ./...`：让 Go 检查所有包是否能编译。

现在项目还没有测试文件，所以看到：

```text
[no test files]
```

不是错误。

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

这个窗口不要关。

### 用 Apifox 验证管理员登录

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

成功后复制响应里的：

```text
data.token
```

后面测试 `GET /api/admin/me` 会用它。

### 常见错误

如果返回：

```text
username or password is wrong
```

优先检查：

- `admins` 表里是否有 `admin`。
- `password_hash` 是否是完整 bcrypt hash。
- 是否把明文 `admin123456` 写进了 `password_hash`。

如果后端启动失败，优先检查：

- Docker Desktop 是否运行。
- PostgreSQL 容器是否运行。
- Redis 容器是否运行。

## 任务 1：理解今天要改哪些文件

### 任务目标

先知道今天会动哪些文件，避免把代码乱放。

今天建议修改或新增 3 个文件：

```text
E:\game-realtime-gm\backend\internal\middleware\auth.go
E:\game-realtime-gm\backend\internal\handler\admin.go
E:\game-realtime-gm\backend\internal\router\router.go
```

### 每个文件负责什么

`middleware/auth.go`：

```text
负责鉴权中间件。
```

今天要在这里新增管理员鉴权能力：

```text
AdminAuth
CurrentAdminID
CurrentAdminUsername
CurrentAdminRole
```

`handler/admin.go`：

```text
负责 GM 管理员相关的受保护接口。
```

今天先只新增一个最小接口：

```text
GET /api/admin/me
```

这个接口不查数据库，只返回 token 里解析出来的管理员身份。

`router.go`：

```text
负责注册路由。
```

今天要把 `GET /api/admin/me` 挂到管理员鉴权分组下。

### 为什么不新建 service 层

今天只是做管理员身份验证和一个简单接口。

业务逻辑还不复杂，所以暂时不引入：

```text
internal/service
```

后续当 GM 接口开始涉及玩家封禁、操作日志、权限角色、查询统计时，再考虑提取 service。

## 任务 2：扩展玩家鉴权和新增管理员鉴权

### 任务目标

修改：

```text
E:\game-realtime-gm\backend\internal\middleware\auth.go
```

操作类型：整文件替换。

### 为什么要改这个文件

现在项目已经有玩家鉴权中间件：

```go
func Auth(jwtSecret string) gin.HandlerFunc
```

它会解析 token，然后把 `player_id` 写入 `gin.Context`。

但是 Day 06 新增了管理员 token，管理员 token 里没有 `player_id`，而是：

```text
admin_id
subject_type = admin
role
```

所以今天要做两件事：

1. 玩家鉴权中间件只接受玩家 token。
2. 新增管理员鉴权中间件，只接受管理员 token。

这样可以避免：

```text
玩家 token 访问 GM 接口
管理员 token 访问玩家个人接口
```

### 完整代码

把文件：

```text
E:\game-realtime-gm\backend\internal\middleware\auth.go
```

整文件替换为：

```go
package middleware

import (
	"net/http"
	"strings"

	tokenauth "game-realtime-gm/backend/internal/auth"

	"github.com/gin-gonic/gin"
)

const (
	ContextKeyPlayerID      = "player_id"
	ContextKeyUsername      = "username"
	ContextKeyAdminID       = "admin_id"
	ContextKeyAdminUsername = "admin_username"
	ContextKeyAdminRole     = "admin_role"
)

func Auth(jwtSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenString, ok := bearerTokenFromHeader(c, 40102, 40103)
		if !ok {
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

		if claims.SubjectType != tokenauth.SubjectTypePlayer || claims.PlayerID <= 0 {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    40301,
				"message": "player permission required",
			})
			c.Abort()
			return
		}

		c.Set(ContextKeyPlayerID, claims.PlayerID)
		c.Set(ContextKeyUsername, claims.Username)
		c.Next()
	}
}

func AdminAuth(jwtSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenString, ok := bearerTokenFromHeader(c, 40112, 40113)
		if !ok {
			return
		}

		claims, err := tokenauth.ParseToken(jwtSecret, tokenString)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":    40114,
				"message": "invalid token",
			})
			c.Abort()
			return
		}

		if claims.SubjectType != tokenauth.SubjectTypeAdmin || claims.AdminID <= 0 {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    40311,
				"message": "admin permission required",
			})
			c.Abort()
			return
		}

		c.Set(ContextKeyAdminID, claims.AdminID)
		c.Set(ContextKeyAdminUsername, claims.Username)
		c.Set(ContextKeyAdminRole, claims.Role)
		c.Next()
	}
}

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

func CurrentAdminID(c *gin.Context) (int64, bool) {
	adminIDValue, exists := c.Get(ContextKeyAdminID)
	if !exists {
		return 0, false
	}

	adminID, ok := adminIDValue.(int64)
	if !ok {
		return 0, false
	}

	return adminID, true
}

func CurrentAdminUsername(c *gin.Context) string {
	usernameValue, exists := c.Get(ContextKeyAdminUsername)
	if !exists {
		return ""
	}

	username, ok := usernameValue.(string)
	if !ok {
		return ""
	}

	return username
}

func CurrentAdminRole(c *gin.Context) string {
	roleValue, exists := c.Get(ContextKeyAdminRole)
	if !exists {
		return ""
	}

	role, ok := roleValue.(string)
	if !ok {
		return ""
	}

	return role
}

func bearerTokenFromHeader(c *gin.Context, missingCode int, invalidCode int) (string, bool) {
	header := c.GetHeader("Authorization")
	if header == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    missingCode,
			"message": "authorization header missing",
		})
		c.Abort()
		return "", false
	}

	tokenString := strings.TrimSpace(strings.TrimPrefix(header, "Bearer"))
	tokenString = strings.TrimSpace(tokenString)
	if tokenString == "" || tokenString == header {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    invalidCode,
			"message": "invalid authorization header",
		})
		c.Abort()
		return "", false
	}

	return tokenString, true
}
```

### 代码逐段分析

```go
ContextKeyPlayerID
ContextKeyUsername
```

这两个 key 给玩家接口使用。

```go
ContextKeyAdminID
ContextKeyAdminUsername
ContextKeyAdminRole
```

这三个 key 给管理员接口使用。

```go
func Auth(jwtSecret string) gin.HandlerFunc
```

玩家鉴权中间件。它只允许：

```text
subject_type = player
player_id > 0
```

如果管理员 token 调用玩家接口，会返回：

```text
player permission required
```

```go
func AdminAuth(jwtSecret string) gin.HandlerFunc
```

管理员鉴权中间件。它只允许：

```text
subject_type = admin
admin_id > 0
```

如果玩家 token 调用 GM 接口，会返回：

```text
admin permission required
```

```go
bearerTokenFromHeader
```

这是一个小 helper，用来统一解析：

```text
Authorization: Bearer <token>
```

为什么要提取它？

因为玩家鉴权和管理员鉴权都要做同样的 header 解析。如果复制两遍，后面修改格式时容易漏改。

### 和项目其他部分的关系

`router.go` 后面会这样使用：

```go
adminProtected.Use(middleware.AdminAuth(cfg.JWTSecret))
```

受保护的 GM 接口会先经过 `AdminAuth`，通过后才进入 handler。

### 常见错误

如果看到：

```text
undefined: middleware.AdminAuth
```

说明 `auth.go` 还没保存，或者函数名写错。

如果看到：

```text
imported and not used
```

说明 import 里有没用到的包。执行 `gofmt` 前后都要检查。

## 任务 3：新增管理员受保护 handler

### 任务目标

新增：

```text
E:\game-realtime-gm\backend\internal\handler\admin.go
```

操作类型：新建文件。

### 为什么要新增这个文件

Day 06 的：

```text
admin_auth.go
```

负责管理员登录。

但是登录接口和登录后的 GM 业务接口不是一类职责。

所以今天新建：

```text
admin.go
```

专门放登录后的管理员接口。

今天先做一个最小接口：

```text
GET /api/admin/me
```

它的作用是：

```text
验证管理员 token 能正常通过管理员鉴权，并返回当前管理员身份。
```

### 完整代码

新建文件：

```text
E:\game-realtime-gm\backend\internal\handler\admin.go
```

写入：

```go
package handler

import (
	"net/http"

	"game-realtime-gm/backend/internal/middleware"

	"github.com/gin-gonic/gin"
)

type AdminHandler struct{}

func NewAdminHandler() *AdminHandler {
	return &AdminHandler{}
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
```

### 代码逐段分析

```go
package handler
```

表示这个文件属于 `handler` 包。

注意：`handler` 目录下的所有 `.go` 文件都是同一个包，所以不能定义重复的类型或函数名。

```go
type AdminHandler struct{}
```

这是管理员接口的 handler。

今天这个接口暂时不查数据库，所以结构体里不需要放 `db`。

后续如果要做：

- 查询管理员资料
- 管理玩家
- 查看操作日志

再给 `AdminHandler` 增加数据库连接。

```go
func NewAdminHandler() *AdminHandler
```

这是构造函数。`router.go` 会调用它创建 handler。

```go
func (h *AdminHandler) Me(c *gin.Context)
```

处理：

```text
GET /api/admin/me
```

它从 `gin.Context` 中读取管理员 ID、用户名和角色。

这些信息不是客户端 Body 传来的，而是 `AdminAuth` 中间件从 JWT 里解析出来后写入的。

### 和项目其他部分的关系

请求链路是：

```text
Apifox
        ↓
GET /api/admin/me
        ↓
router.go
        ↓
middleware.AdminAuth
        ↓
AdminHandler.Me
        ↓
JSON response
```

如果中间件不通过，请求不会进入 `AdminHandler.Me`。

## 任务 4：把管理员受保护接口挂到路由上

### 任务目标

修改：

```text
E:\game-realtime-gm\backend\internal\router\router.go
```

操作类型：整文件替换。

### 为什么要改 router.go

`router.go` 是接口地址的登记表。

你新增了 `AdminHandler.Me` 后，如果不在 `router.go` 注册，外部就访问不到：

```text
GET /api/admin/me
```

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
	adminHandler := handler.NewAdminHandler()
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

	return r
}
```

### 代码逐段分析

```go
adminHandler := handler.NewAdminHandler()
```

创建管理员受保护接口的 handler。

它和：

```go
adminAuthHandler := handler.NewAdminAuthHandler(db, cfg.JWTSecret)
```

不是一回事。

`adminAuthHandler` 负责登录。

`adminHandler` 负责登录后的 GM 接口。

```go
api.POST("/admin/login", adminAuthHandler.Login)
```

这是公开接口。登录前没有 token，所以不能加管理员鉴权。

```go
adminProtected := api.Group("/admin")
adminProtected.Use(middleware.AdminAuth(cfg.JWTSecret))
adminProtected.GET("/me", adminHandler.Me)
```

这是管理员受保护接口分组。

最终接口地址是：

```text
GET /api/admin/me
```

它必须带管理员 token。

### 和项目其他部分的关系

现在项目有两套受保护接口：

玩家接口：

```text
/api/me
/api/players
/api/online/heartbeat
```

使用：

```go
middleware.Auth
```

管理员接口：

```text
/api/admin/me
```

使用：

```go
middleware.AdminAuth
```

这就是基础权限隔离。

## 任务 5：格式化、整理依赖并编译

### 任务目标

确认代码格式正确，依赖干净，所有包能编译。

### 在哪里执行

PowerShell。

### 执行命令

```powershell
cd E:\game-realtime-gm\backend
gofmt -w .\internal\middleware\auth.go .\internal\handler\admin.go .\internal\router\router.go
go mod tidy
go test ./...
```

### 命令分析

```powershell
gofmt -w ...
```

格式化 Go 文件。

Go 项目里不要手动调整缩进，统一交给 `gofmt`。

```powershell
go mod tidy
```

整理依赖。

今天没有新增第三方依赖，但执行一次可以确认 `go.mod` 和 `go.sum` 没有多余内容。

```powershell
go test ./...
```

编译检查整个后端。

如果看到：

```text
[no test files]
```

不是错误。

### 怎么验证成功

没有红色报错。

所有包都能通过：

```text
?    game-realtime-gm/backend/...
```

## 任务 6：启动后端并测试管理员受保护接口

### 任务目标

验证：

```text
GET /api/admin/me
```

必须使用管理员 token 才能访问。

### 第 1 步：启动后端

PowerShell 执行：

```powershell
cd E:\game-realtime-gm\backend
go run .\cmd\server
```

成功后看到：

```text
database connected
redis connected
server listening on :8080
```

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

### 第 3 步：访问管理员接口

Apifox 请求：

```text
GET http://localhost:8080/api/admin/me
```

Headers：

```text
Authorization: Bearer 你的管理员token
```

成功响应：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "id": 1,
    "username": "admin",
    "role": "super_admin"
  }
}
```

### 第 4 步：故意不带 token 测试

请求：

```text
GET http://localhost:8080/api/admin/me
```

不要带 Authorization。

预期返回：

```json
{
  "code": 40112,
  "message": "authorization header missing"
}
```

### 第 5 步：故意用玩家 token 测试

先调用玩家登录：

```text
POST http://localhost:8080/api/login
```

拿到玩家 token 后，调用：

```text
GET http://localhost:8080/api/admin/me
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

这个测试非常重要。

因为它证明：

```text
不是所有登录用户都能访问 GM 接口。
```

## 任务 7：用请求链路复盘今天的功能

### 任务目标

把今天新增的功能讲清楚。

### 管理员登录链路

```text
Apifox
        ↓
POST /api/admin/login
        ↓
router.go
        ↓
AdminAuthHandler.Login
        ↓
SELECT FROM admins
        ↓
bcrypt.CompareHashAndPassword
        ↓
GenerateAdminToken
        ↓
返回管理员 JWT
```

### 管理员受保护接口链路

```text
Apifox
        ↓
GET /api/admin/me
        ↓
router.go
        ↓
middleware.AdminAuth
        ↓
ParseToken
        ↓
检查 subject_type 是否为 admin
        ↓
把 admin_id、username、role 写入 gin.Context
        ↓
AdminHandler.Me
        ↓
返回当前管理员信息
```

### 你今天应该能说清楚

- 管理员登录为什么是公开接口。
- 管理员业务接口为什么必须加管理员鉴权。
- 玩家 token 和管理员 token 有什么区别。
- `subject_type` 为什么重要。
- `gin.Context` 在中间件和 handler 之间起什么作用。
- 为什么今天不做完整 RBAC。

## 今日验收清单

- [ ] `go test ./...` 通过。
- [ ] 管理员登录能返回 token。
- [ ] 新增 `middleware.AdminAuth`。
- [ ] 玩家鉴权中间件会拒绝管理员 token。
- [ ] 管理员鉴权中间件会拒绝玩家 token。
- [ ] 新增 `GET /api/admin/me`。
- [ ] 不带 token 访问 `GET /api/admin/me` 返回 401。
- [ ] 带玩家 token 访问 `GET /api/admin/me` 返回 403。
- [ ] 带管理员 token 访问 `GET /api/admin/me` 返回当前管理员信息。

## 今日不要做

今天不要做：

- React GM 后台页面。
- 完整 RBAC 权限表。
- 菜单权限。
- 操作日志。
- 玩家封禁接口。
- WebSocket。
- 房间系统。
- 匹配系统。

原因：

今天只做“管理员 token 能保护 GM 接口”这个最小闭环。

如果这个闭环没打牢，后面做再多 GM 功能都会缺权限基础。

## 常见错误

### 1. `undefined: handler.NewAdminHandler`

原因：

没有新建：

```text
E:\game-realtime-gm\backend\internal\handler\admin.go
```

或者函数名写错。

正确函数名是：

```go
func NewAdminHandler() *AdminHandler
```

### 2. `undefined: middleware.AdminAuth`

原因：

`middleware/auth.go` 没有保存，或者没有写：

```go
func AdminAuth(jwtSecret string) gin.HandlerFunc
```

### 3. `imported and not used`

原因：

Go 不允许没用到的 import。

解决：

检查报错文件，把未使用的 import 删除，然后执行：

```powershell
gofmt -w 报错文件
```

### 4. `GET /api/admin/me` 返回 404

原因：

路由没有注册成功。

检查 `router.go` 是否有：

```go
adminProtected := api.Group("/admin")
adminProtected.Use(middleware.AdminAuth(cfg.JWTSecret))
adminProtected.GET("/me", adminHandler.Me)
```

### 5. 管理员 token 访问仍然返回 403

检查：

- 你拿的是不是 `POST /api/admin/login` 返回的 token。
- 你是否误用了 `POST /api/login` 的玩家 token。
- `jwt.go` 里的 `GenerateAdminToken` 是否设置了 `SubjectTypeAdmin`。
- Apifox Header 是否写成：

```text
Authorization: Bearer token
```

不要写成：

```text
Bearer: token
```

也不要只填 token，不写 `Bearer`。

### 6. 玩家接口突然不能访问

原因可能是你误用了管理员 token 调玩家接口。

玩家接口现在只接受玩家 token。

正确方式：

```text
POST /api/login
```

拿玩家 token，再访问：

```text
GET /api/me
```

## 面试怎么讲这一部分

可以这样说：

```text
在玩家登录和管理员登录都完成后，我没有把所有 token 混用，而是在 JWT Claims 中增加 subject_type，用来区分 player 和 admin。随后在 Gin 中实现了两套鉴权中间件：玩家接口使用 Auth，只允许 player token；GM 接口使用 AdminAuth，只允许 admin token。管理员登录接口本身保持公开，因为登录前还没有 token；登录后的 /api/admin/me 则必须携带管理员 token 才能访问。这样项目开始具备基础权限隔离能力，为后续 GM 后台、RBAC 和操作日志打基础。
```

## 明日预告

Day 08 可以考虑两个方向，建议优先第一个：

1. GM 管理员查看玩家列表，并把玩家列表接口从玩家保护迁移到管理员保护。
2. 新增操作日志表，记录管理员做了什么操作。

更推荐先做：

```text
GM 管理员查看玩家列表
```

因为这会直接连接 GM 后台的真实业务：

```text
管理员登录 -> 管理员鉴权 -> 查询玩家 -> 后续管理玩家
```
