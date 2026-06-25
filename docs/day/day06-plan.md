# Day 06 学习计划：玩家接口收尾与 GM 管理员登录地基

## 今日目标

今天做两件事：

1. 收尾 Day 05 的玩家接口规范问题：查不到玩家时返回 `404`，玩家列表补 `total`。
2. 开始 GM 管理员模块：创建管理员表、生成管理员密码哈希、实现管理员登录接口。

今天结束后，项目会新增：

```text
POST /api/admin/login
```

这个接口先只负责管理员登录并返回 JWT。完整 RBAC 权限、GM 后台菜单、操作日志放到后续 Day 继续做。

## 当前项目状态

你现在已经完成：

- 玩家注册、登录
- JWT 鉴权
- 当前玩家查询
- 玩家昵称修改
- 玩家列表查询
- Redis 在线状态
- `go test ./...` 已通过

Day 06 在这个基础上继续。

## 今天会学到什么

- `pgx.ErrNoRows` 和普通数据库错误的区别
- 为什么查不到资源应该返回 `404` 而不是 `500`
- 分页接口为什么常常需要 `total`
- 管理员账号和玩家账号为什么要分表
- bcrypt 密码哈希如何用于初始化管理员
- 同一个 Go 包内如何避免结构体/函数重名
- 如何为管理员签发不同类型的 JWT

## 今日最终效果

完成后：

```text
GET /api/players/999999
```

查不到玩家时返回：

```json
{
  "code": 40401,
  "message": "player not found"
}
```

玩家列表返回：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "items": [],
    "page": 1,
    "page_size": 10,
    "total": 1
  }
}
```

管理员登录：

```text
POST /api/admin/login
```

返回管理员 token。

## 任务 0：先验证当前代码

### 任务目标

确认 Day 05 代码处于可继续开发状态。

### 具体操作步骤

执行：

```powershell
cd E:\game-realtime-gm\backend
go test ./...
```

### 怎么验证成功

没有 `FAIL` 就可以继续。

如果这里失败，先修编译错误。

## 任务 1：优化玩家接口的 404 和分页 total

### 任务目标

让玩家查询接口更符合真实 Web API 规范。

### 你要新增或修改什么

修改：

```text
E:\game-realtime-gm\backend\internal\handler\player.go
```

操作类型：整文件替换。

### 这个操作有什么用

当前 `GetByID` 查不到玩家时会返回 `500`，这不够准确。

真实语义应该是：

```text
数据库坏了：500
玩家不存在：404
```

另外，玩家列表目前只返回 `items/page/page_size`，后续 GM 后台表格需要知道总数，所以补充 `total`。

### 代码内容

把 `player.go` 替换为：

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
	if errors.Is(err, pgx.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    40401,
			"message": "player not found",
		})
		return
	}
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
	if errors.Is(err, pgx.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    40401,
			"message": "player not found",
		})
		return
	}
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
	if errors.Is(err, pgx.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    40401,
			"message": "player not found",
		})
		return
	}
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
			"code":    50013,
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
			"code":    50014,
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
				"code":    50015,
				"message": "scan player failed",
			})
			return
		}
		players = append(players, player)
	}

	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50016,
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
errors.Is(err, pgx.ErrNoRows)
```

用于判断数据库查询结果为空。

查询为空不是服务器崩了，而是资源不存在，所以应该返回 `404`。

```go
SELECT COUNT(*) FROM players
```

用于统计玩家总数，给 GM 后台分页使用。

```go
LIMIT $n OFFSET $m
```

`LIMIT` 控制每页数量，`OFFSET` 控制跳过多少条。

### 注意

这里仍然允许 handler 直接写 SQL，因为当前项目处于学习阶段。

等玩家查询条件继续变复杂，再抽 `service`。

## 任务 2：扩展 JWT Claims 支持管理员 token

### 任务目标

让 JWT 能区分玩家 token 和管理员 token。

### 你要新增或修改什么

修改：

```text
E:\game-realtime-gm\backend\internal\auth\jwt.go
```

操作类型：整文件替换。

### 这个操作有什么用

玩家和管理员都需要登录，但它们不是同一种身份。

后续 GM 后台接口需要判断：

```text
这是玩家 token？
还是管理员 token？
```

所以 JWT claims 里需要增加：

```text
subject_type
admin_id
role
```

### 代码内容

把 `jwt.go` 替换为：

```go
package auth

import (
	"fmt"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	SubjectTypePlayer = "player"
	SubjectTypeAdmin  = "admin"
)

type Claims struct {
	SubjectType string `json:"subject_type"`
	PlayerID    int64  `json:"player_id,omitempty"`
	AdminID     int64  `json:"admin_id,omitempty"`
	Username    string `json:"username"`
	Role        string `json:"role,omitempty"`
	jwt.RegisteredClaims
}

func GenerateToken(secret string, playerID int64, username string) (string, error) {
	claims := Claims{
		SubjectType: SubjectTypePlayer,
		PlayerID:    playerID,
		Username:    username,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatInt(playerID, 10),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

func GenerateAdminToken(secret string, adminID int64, username string, role string) (string, error) {
	claims := Claims{
		SubjectType: SubjectTypeAdmin,
		AdminID:     adminID,
		Username:    username,
		Role:        role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatInt(adminID, 10),
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
SubjectType string
```

表示 token 属于哪种身份。

当前有：

```text
player
admin
```

```go
GenerateToken
```

保留给玩家登录使用，避免影响已有玩家接口。

```go
GenerateAdminToken
```

给管理员登录使用。

### 和项目其他部分的关系

Day 03 的玩家鉴权中间件仍然读取：

```go
claims.PlayerID
```

后续管理员鉴权中间件会读取：

```go
claims.AdminID
claims.Role
```

## 任务 3：创建管理员表

### 任务目标

在 PostgreSQL 中创建 `admins` 表，用来保存 GM 管理员账号。

### 你要新增或修改什么

修改：

```text
E:\game-realtime-gm\backend\internal\database\schema.sql
```

操作类型：在文件末尾追加 SQL。

### 为什么管理员和玩家分表

玩家账号和管理员账号职责不同。

玩家是游戏用户。

管理员是 GM 后台操作者。

它们后续会有不同权限、不同接口、不同审计要求，所以分表更清晰。

### SQL 内容

在 `schema.sql` 末尾追加：

```sql
CREATE TABLE IF NOT EXISTS admins (
    id BIGSERIAL PRIMARY KEY,
    username VARCHAR(64) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    display_name VARCHAR(64) NOT NULL,
    role VARCHAR(32) NOT NULL DEFAULT 'gm',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

### 字段分析

- `id`：管理员唯一 ID。
- `username`：管理员登录账号，唯一。
- `password_hash`：管理员密码哈希，不能保存明文密码。
- `display_name`：后台展示名称。
- `role`：管理员角色，先用 `gm`，后续可扩展 `super_admin`。
- `created_at`：创建时间。
- `updated_at`：更新时间。

### 执行 SQL

PowerShell 方式：

```powershell
psql -h localhost -p 15432 -U game -d game_realtime -f E:\game-realtime-gm\backend\internal\database\schema.sql
```

Docker Desktop 容器终端方式：

```bash
psql -U game -d game_realtime
```

进入后复制 `CREATE TABLE IF NOT EXISTS admins ...` 执行。

### 验证

在 psql 中执行：

```sql
\d admins
```

能看到表结构就成功。

## 任务 4：创建密码哈希工具

### 任务目标

生成管理员初始密码的 bcrypt hash，用于插入 `admins` 表。

### 你要新增或修改什么

新增：

```text
E:\game-realtime-gm\backend\cmd\tools\hash_password\main.go
```

操作类型：新建文件。

### 这个操作有什么用

管理员账号需要初始数据。

但是数据库不能保存明文密码。

所以你需要先把明文密码转换成 bcrypt hash，再插入数据库。

### 代码内容

创建目录：

```powershell
cd E:\game-realtime-gm\backend
mkdir cmd\tools\hash_password
```

创建 `main.go`：

```go
package main

import (
	"fmt"
	"log"
	"os"

	"golang.org/x/crypto/bcrypt"
)

func main() {
	if len(os.Args) != 2 {
		log.Fatal("usage: go run ./cmd/tools/hash_password <password>")
	}

	password := os.Args[1]
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(string(hash))
}
```

### 代码分析

```go
os.Args
```

读取命令行参数。

例如：

```powershell
go run .\cmd\tools\hash_password admin123456
```

这里的 `admin123456` 就是参数。

```go
bcrypt.GenerateFromPassword
```

生成密码哈希。

输出结果可以写入数据库的 `password_hash` 字段。

### 运行

执行：

```powershell
cd E:\game-realtime-gm\backend
go run .\cmd\tools\hash_password admin123456
```

复制输出的一整串 hash。

## 任务 5：插入初始管理员

### 任务目标

向 `admins` 表插入一个本地开发用 GM 管理员账号。

### 具体操作步骤

进入 PostgreSQL：

```powershell
psql -h localhost -p 15432 -U game -d game_realtime
```

或者在 Docker Desktop PostgreSQL 容器终端：

```bash
psql -U game -d game_realtime
```

执行：

```sql
INSERT INTO admins (username, password_hash, display_name, role)
VALUES ('admin', '这里粘贴刚才生成的bcrypt哈希', '系统管理员', 'super_admin')
ON CONFLICT (username) DO NOTHING;
```

### 注意

不要把：

```text
admin123456
```

直接写进 `password_hash`。

必须写 bcrypt 输出的 hash。

### 验证

执行：

```sql
SELECT id, username, display_name, role, created_at
FROM admins;
```

能看到 `admin` 账号就成功。

## 任务 6：新增管理员模型

### 任务目标

用 Go 结构体描述 `admins` 表。

### 你要新增或修改什么

新增：

```text
E:\game-realtime-gm\backend\internal\model\admin.go
```

操作类型：新建文件。

### 代码内容

```go
package model

import "time"

type Admin struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	DisplayName  string    `json:"display_name"`
	Role         string    `json:"role"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
```

### 代码分析

```go
PasswordHash string `json:"-"`
```

管理员密码哈希也不能返回给前端。

```go
Role string
```

先用于区分：

```text
gm
super_admin
```

后续做 RBAC 时会继续扩展。

## 任务 7：新增管理员登录 handler

### 任务目标

实现：

```text
POST /api/admin/login
```

管理员输入账号密码，后端校验后返回管理员 token。

### 你要新增或修改什么

新增：

```text
E:\game-realtime-gm\backend\internal\handler\admin_auth.go
```

操作类型：新建文件。

### 避免同包重名

`handler/auth.go` 里已经有：

```go
type loginRequest struct
```

所以管理员登录请求不能再叫 `loginRequest`。

这里命名为：

```go
type adminLoginRequest struct
```

这是为了遵守技术规范：同一个 package 里不能有重复类型名。

### 代码内容

```go
package handler

import (
	"errors"
	"net/http"

	tokenauth "game-realtime-gm/backend/internal/auth"
	"game-realtime-gm/backend/internal/model"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

type AdminAuthHandler struct {
	db        *pgxpool.Pool
	jwtSecret string
}

func NewAdminAuthHandler(db *pgxpool.Pool, jwtSecret string) *AdminAuthHandler {
	return &AdminAuthHandler{
		db:        db,
		jwtSecret: jwtSecret,
	}
}

type adminLoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

func (h *AdminAuthHandler) Login(c *gin.Context) {
	var req adminLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40011,
			"message": "invalid request",
		})
		return
	}

	var admin model.Admin
	err := h.db.QueryRow(
		c.Request.Context(),
		`SELECT id, username, password_hash, display_name, role, created_at, updated_at
		 FROM admins
		 WHERE username = $1`,
		req.Username,
	).Scan(
		&admin.ID,
		&admin.Username,
		&admin.PasswordHash,
		&admin.DisplayName,
		&admin.Role,
		&admin.CreatedAt,
		&admin.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    40111,
			"message": "username or password is wrong",
		})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50031,
			"message": "query admin failed",
		})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(req.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    40111,
			"message": "username or password is wrong",
		})
		return
	}

	token, err := tokenauth.GenerateAdminToken(h.jwtSecret, admin.ID, admin.Username, admin.Role)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50032,
			"message": "generate admin token failed",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "admin login success",
		"data": gin.H{
			"token": token,
			"admin": gin.H{
				"id":           admin.ID,
				"username":     admin.Username,
				"display_name": admin.DisplayName,
				"role":         admin.Role,
			},
		},
	})
}
```

### 代码分析

```go
AdminAuthHandler
```

专门处理管理员认证，不和玩家 `AuthHandler` 混在一起。

```go
adminLoginRequest
```

避免和玩家登录请求 `loginRequest` 重名。

```go
GenerateAdminToken
```

生成管理员身份 token。

## 任务 8：更新路由

### 任务目标

把管理员登录接口注册到 Gin。

### 你要新增或修改什么

修改：

```text
E:\game-realtime-gm\backend\internal\router\router.go
```

操作类型：整文件替换。

### 代码内容

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

	return r
}
```

### 代码分析

```go
api.POST("/admin/login", adminAuthHandler.Login)
```

管理员登录暂时是公开接口，因为登录前还没有 token。

后续 GM 后台其他接口会放进管理员鉴权中间件。

## 任务 9：格式化、编译并启动

### 具体操作步骤

执行：

```powershell
cd E:\game-realtime-gm\backend
gofmt -w .\internal\auth\jwt.go .\internal\handler\player.go .\internal\handler\admin_auth.go .\internal\model\admin.go .\internal\router\router.go .\cmd\tools\hash_password\main.go
go mod tidy
go test ./...
```

如果通过，启动后端：

```powershell
go run .\cmd\server
```

期望日志：

```text
database connected
redis connected
server listening on :8080
```

## 任务 10：用 Apifox 测试管理员登录

### 请求

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

### 成功响应

```json
{
  "code": 0,
  "message": "admin login success",
  "data": {
    "token": "一长串管理员JWT",
    "admin": {
      "id": 1,
      "username": "admin",
      "display_name": "系统管理员",
      "role": "super_admin"
    }
  }
}
```

## 今日验收清单

- [ ] `go test ./...` 通过。
- [ ] 玩家查不到时返回 404。
- [ ] 玩家列表返回 `total`。
- [ ] `schema.sql` 包含 `admins` 表。
- [ ] 数据库里能查到 `admins` 表。
- [ ] 已生成管理员密码 bcrypt hash。
- [ ] 已插入初始管理员 `admin`。
- [ ] `POST /api/admin/login` 能返回管理员 token。

## 今日不要做

今天不要做：

- 完整 RBAC
- 管理员权限中间件
- GM 操作日志
- GM 前端页面
- 玩家封禁
- WebSocket

这些放到后续 Day。

## 常见错误

### 1. `loginRequest redeclared`

原因：你在 `handler` 包里又定义了一个 `loginRequest`。

解决：管理员登录请求必须叫：

```go
adminLoginRequest
```

### 2. 管理员登录一直 401

检查：

- `admins` 表是否有 `admin` 账号。
- `password_hash` 是否是 bcrypt hash。
- 你是否把明文 `admin123456` 直接写进了 `password_hash`。

### 3. `relation "admins" does not exist`

原因：没有执行建表 SQL。

解决：执行 `schema.sql` 或手动创建 `admins` 表。

## 今天学完后你应该能说清楚

- 玩家账号和管理员账号为什么分表。
- 为什么管理员密码也必须保存 bcrypt hash。
- 为什么 `handler` 包里不能重复定义 `loginRequest`。
- 为什么 JWT 里要区分 `subject_type`。
- 为什么管理员登录接口是公开接口，而管理员业务接口需要管理员鉴权。

