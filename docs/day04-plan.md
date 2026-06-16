# Day 04 学习计划：Redis 接入与玩家在线状态

## 今日目标

今天要把已经启动的 Redis 真正接入 Go 后端，并实现一个最小的玩家在线状态闭环。

完成后，登录玩家可以：

- 调用在线心跳接口，告诉后端“我还在线”。
- 调用在线状态接口，查看自己的在线状态。
- 在 Redis 中看到带过期时间的在线状态 key。

今天的核心链路是：

```text
玩家登录拿到 JWT
        ↓
请求头携带 Authorization: Bearer token
        ↓
鉴权中间件识别 player_id
        ↓
POST /api/online/heartbeat
        ↓
Redis 保存 online:player:<id>
        ↓
TTL 到期后自动删除
```

## 为什么今天学习 Redis

PostgreSQL 和 Redis 都能保存数据，但用途不同。

```text
PostgreSQL：
适合保存长期可靠数据，例如账号、玩家资料、战绩。

Redis：
适合保存访问频繁、变化很快、可以过期的数据，例如在线状态、验证码、匹配队列、排行榜。
```

玩家在线状态会频繁变化，而且玩家长时间没有心跳时应该自动过期。

这正适合使用 Redis 的 TTL。

## 当前项目状态

你现在已经完成：

- PostgreSQL 和 Redis Docker 容器
- 注册接口
- 登录接口
- JWT 生成和解析
- 鉴权中间件
- `GET /api/me`

开始 Day 04 前，先执行：

```powershell
cd E:\game-realtime-gm\backend
go test ./...
```

如果所有包都显示 `[no test files]`，但没有 `FAIL`，说明当前代码可以继续开发。

## 今天会学到什么

- Redis 是什么。
- Redis key-value 数据如何操作。
- TTL 是什么。
- Go 后端如何连接 Redis。
- 为什么在线状态不适合直接写入 PostgreSQL。
- 如何用 HTTP 心跳模拟在线状态续期。
- 如何用 `redis-cli` 检查后端写入的数据。

## 今日最终效果

今天完成后，项目新增两个受保护接口：

```text
POST /api/online/heartbeat
GET  /api/online/status
```

调用心跳接口后，Redis 中会出现类似 key：

```text
online:player:1
```

它有过期时间：

```text
TTL = 120 秒
```

如果玩家不再发送心跳，key 会自动消失，玩家就被视为离线。

## 任务 0：先验证 Day 03

### 任务目标

确认登录和 `/api/me` 已经可用，再进入 Redis 开发。

### 为什么要做

Day 04 的在线接口依赖 Day 03 的鉴权中间件。

如果 Day 03 还没有跑通，Day 04 出错时很难判断问题出在 JWT、鉴权还是 Redis。

### 具体操作步骤

打开 Docker Desktop，然后执行：

```powershell
cd E:\game-realtime-gm\deploy
docker compose up -d
docker ps
```

启动后端：

```powershell
cd E:\game-realtime-gm\backend
go run .\cmd\server
```

用 Apifox 登录：

```text
POST http://localhost:8080/api/login
```

Body 选择 JSON：

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

再请求：

```text
GET http://localhost:8080/api/me
```

Headers 添加：

```text
Authorization: Bearer 你的token
```

### 怎么验证成功

`/api/me` 返回当前玩家信息，说明 Day 03 正常。

如果 Day 03 还没有跑通，先不要继续 Day 04。

## 任务 1：用 redis-cli 认识 Redis

### 任务目标

先在命令行里手动操作 Redis，理解 key、value、TTL。

### 这个操作有什么用

后面 Go 代码会自动写入 Redis。

在写代码前，先手动操作一遍，可以帮助你理解代码到底在做什么。

这一任务不需要改 Go 代码。它只是让你先认识 Redis 的基本操作。

你今天至少要理解三个概念：

```text
key：Redis 里数据的名字
value：Redis 里保存的数据内容
TTL：这个 key 还剩多少秒自动过期
```

### 具体操作步骤

### 第 1 步：确认 Redis 容器正在运行

先打开 Docker Desktop。

然后在 PowerShell 执行：

```powershell
docker ps
```

你应该看到类似：

```text
NAMES                 STATUS         PORTS
game_realtime_redis   Up ...         0.0.0.0:6379->6379/tcp
```

如果没有看到 `game_realtime_redis`，说明 Redis 容器没有运行。

这时执行：

```powershell
cd E:\game-realtime-gm\deploy
docker compose up -d
docker ps
```

命令分析：

- `docker ps`：查看当前正在运行的容器。
- `docker compose up -d`：按 `docker-compose.yml` 启动 PostgreSQL 和 Redis。
- `-d`：后台运行，不占用当前终端。

### 第 2 步：进入 Redis 命令行

执行：

```powershell
docker exec -it game_realtime_redis redis-cli
```

成功后，你会看到类似：

```text
127.0.0.1:6379>
```

这表示你已经进入 Redis CLI。

注意：看到 `127.0.0.1:6379>` 后，后面输入的是 Redis 命令，不是 PowerShell 命令。

### 命令分析：进入 redis-cli

```text
docker exec
```

表示在已经运行的 Docker 容器中执行命令。

```text
-it
```

表示打开一个可以交互输入的终端。

```text
game_realtime_redis
```

是 Redis 容器名。

```text
redis-cli
```

是 Redis 自带的命令行工具。

### 第 3 步：测试 Redis 是否可用

输入：

```text
PING
```

期望返回：

```text
PONG
```

含义：

```text
PING：问 Redis 你还活着吗？
PONG：Redis 回答我活着。
```

这是最简单的连通性测试。

### 第 4 步：写入一个普通 key

输入：

```text
SET learning:name redis
```

期望返回：

```text
OK
```

再读取：

```text
GET learning:name
```

期望返回：

```text
"redis"
```

命令分析：

```text
SET learning:name redis
```

- `SET`：保存一个 key-value。
- `learning:name`：key。
- `redis`：value。

```text
GET learning:name
```

- `GET`：读取一个 key 的 value。

这里的 `learning:name` 只是一个练习 key。

Redis 的 key 常用冒号分层，例如：

```text
online:player:1
room:1001:state
rank:score
```

冒号没有特殊语法，只是工程习惯，方便人类阅读和分类。

### 第 5 步：写入一个带 TTL 的 key

设置一个临时 key：

```text
SET learning:day04 hello EX 60
```

读取：

```text
GET learning:day04
```

期望返回：

```text
"hello"
```

查看剩余过期时间：

```text
TTL learning:day04
```

应该看到小于等于 `60` 的数字。

命令分析：

```text
SET learning:day04 hello EX 60
```

- `SET`：保存数据。
- `learning:day04`：key。
- `hello`：value。
- `EX 60`：设置 60 秒后自动过期。

```text
TTL learning:day04
```

- `TTL`：查看 key 还有多少秒过期。

可能返回：

```text
58
```

表示这个 key 还有 58 秒过期。

如果等待 60 秒后再执行：

```text
GET learning:day04
```

会看到：

```text
(nil)
```

这表示 key 已经不存在。

### 第 6 步：理解 Redis 的自动过期

再练习一次：

```text
SET online:player:1 1 EX 10
GET online:player:1
TTL online:player:1
```

你应该看到：

```text
"1"
```

以及一个小于等于 `10` 的 TTL。

等待 10 秒以上，再执行：

```text
GET online:player:1
TTL online:player:1
```

可能看到：

```text
(nil)
-2
```

`TTL` 返回值含义：

```text
正数：还有多少秒过期
-1：key 存在，但没有设置过期时间
-2：key 不存在
```

这就是在线状态的核心原理：

```text
玩家发心跳
        ↓
SET online:player:1 1 EX 120
        ↓
如果 120 秒内没有新心跳
        ↓
key 自动消失
        ↓
玩家被视为离线
```

### 第 7 步：查看和删除 key

查看当前练习 key：

```text
KEYS learning:*
KEYS online:*
```

删除一个 key：

```text
DEL learning:name
```

再读取：

```text
GET learning:name
```

应该返回：

```text
(nil)
```

命令分析：

- `KEYS learning:*`：查看所有以 `learning:` 开头的 key。
- `DEL learning:name`：删除指定 key。

注意：`KEYS` 在本地学习可以用，但线上大数据量 Redis 不建议随便用，因为它可能扫描大量 key。

### 第 8 步：退出 redis-cli

退出：

```text
exit
```

退出后你会回到 PowerShell。

### 和项目其他部分的关系

后面的玩家在线状态会使用同样思路：

```text
SET online:player:1 1 EX 120
```

玩家每次发心跳，就重新设置 TTL。

### 常见错误

错误：

```text
Error response from daemon
```

原因：Redis 容器没有运行。

解决：

```powershell
cd E:\game-realtime-gm\deploy
docker compose up -d
```

错误 2：

```text
Could not connect to Redis at 127.0.0.1:6379: Connection refused
```

原因：Redis 服务没有启动，或者你不是在容器里执行 `redis-cli`。

解决：

```powershell
docker ps
docker exec -it game_realtime_redis redis-cli
```

错误 3：

```text
(nil)
```

这不是报错。

它表示 key 不存在，常见原因是：

- key 写错了
- key 已经过期
- key 被删除了

错误 4：

```text
TTL 返回 -2
```

表示 key 不存在。

错误 5：

```text
TTL 返回 -1
```

表示 key 存在，但没有设置过期时间。

在线状态 key 不应该是 `-1`，因为在线状态必须会自动过期。

## 任务 2：给配置模块增加 Redis 配置

### 任务目标

让 Go 后端从配置模块读取 Redis 地址、密码和数据库编号。

### 你要新增或修改什么

修改：

```text
E:\game-realtime-gm\backend\internal\config\config.go
```

操作类型：整文件替换。

### 这个操作有什么用

Redis 地址和数据库地址一样，属于会随环境变化的配置。

本地开发时 Redis 是：

```text
localhost:6379
```

以后部署到 Linux 服务器或 Docker 网络里，Redis 地址可能不同。

所以不能把 Redis 地址散落在业务代码里。

### 代码内容

把 `config.go` 整文件替换为：

```go
package config

import (
	"os"
	"strconv"
)

type Config struct {
	AppPort   string
	Database  DatabaseConfig
	Redis     RedisConfig
	JWTSecret string
}

type DatabaseConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	Name     string
	SSLMode  string
}

type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

func Load() Config {
	return Config{
		AppPort: getEnv("APP_PORT", "8080"),
		Database: DatabaseConfig{
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnv("DB_PORT", "15432"),
			User:     getEnv("DB_USER", "game"),
			Password: getEnv("DB_PASSWORD", "game123456"),
			Name:     getEnv("DB_NAME", "game_realtime"),
			SSLMode:  getEnv("DB_SSLMODE", "disable"),
		},
		Redis: RedisConfig{
			Addr:     getEnv("REDIS_ADDR", "localhost:6379"),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getEnvInt("REDIS_DB", 0),
		},
		JWTSecret: getEnv("JWT_SECRET", "game-realtime-dev-secret"),
	}
}

func getEnv(key string, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

func getEnvInt(key string, defaultValue int) int {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}

	parsedValue, err := strconv.Atoi(value)
	if err != nil {
		return defaultValue
	}
	return parsedValue
}
```

### 代码分析

```go
Redis RedisConfig
```

表示整个项目配置中新增一组 Redis 配置。

```go
type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}
```

字段含义：

- `Addr`：Redis 地址，例如 `localhost:6379`。
- `Password`：Redis 密码。本地 Docker Redis 当前没有密码，所以默认空字符串。
- `DB`：Redis 逻辑数据库编号。今天使用默认的 `0`。

```go
DB: getEnvInt("REDIS_DB", 0)
```

环境变量是字符串，但 Redis 的 DB 编号需要整数。

所以新增 `getEnvInt`，用 `strconv.Atoi` 把字符串转成整数。

### 和项目其他部分的关系

后面初始化 Redis 客户端时，会使用：

```go
cfg.Redis
```

### 怎么验证成功

替换文件后先执行：

```powershell
cd E:\game-realtime-gm\backend
gofmt -w .\internal\config\config.go
go test ./...
```

如果编译通过，说明配置修改正常。

## 任务 3：创建 Redis 连接模块

### 任务目标

让 Go 后端启动时连接 Redis，并通过 `PING` 检查 Redis 是否可用。

### 你要新增或修改什么

安装依赖：

```text
github.com/redis/go-redis/v9
```

新增：

```text
E:\game-realtime-gm\backend\internal\cache\redis.go
```

### 这个操作有什么用

之前后端只连接 PostgreSQL。

现在需要增加 Redis 客户端，后续在线状态、匹配队列、排行榜都会复用它。

### 第 1 步：安装 Redis Go 客户端

在后端目录执行：

```powershell
cd E:\game-realtime-gm\backend
go get github.com/redis/go-redis/v9
```

命令分析：

- `go get`：下载并记录 Go 依赖。
- `github.com/redis/go-redis/v9`：Redis 官方推荐的 Go 客户端之一。

### 第 2 步：创建目录

执行：

```powershell
mkdir internal\cache
```

### 第 3 步：创建 `redis.go`

文件位置：

```text
E:\game-realtime-gm\backend\internal\cache\redis.go
```

操作类型：新建文件。

写入：

```go
package cache

import (
	"context"

	"game-realtime-gm/backend/internal/config"

	"github.com/redis/go-redis/v9"
)

func NewRedisClient(ctx context.Context, cfg config.RedisConfig) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, err
	}

	return client, nil
}
```

### 代码分析

```go
package cache
```

表示这个文件属于 `cache` 包。

Redis 经常被用作缓存和实时状态存储，所以这里使用 `internal/cache` 目录。

```go
redis.NewClient(&redis.Options{...})
```

创建 Redis 客户端。

```go
client.Ping(ctx).Err()
```

启动时主动发一次 `PING`。

如果 Redis 正常，会收到 `PONG`。

如果 Redis 没启动，后端启动阶段就会报错，方便尽早发现问题。

### 和项目其他部分的关系

`main.go` 会调用：

```go
cache.NewRedisClient(ctx, cfg.Redis)
```

然后把 Redis 客户端传给路由和在线状态 handler。

### 怎么验证成功

完成任务 4 修改 `main.go` 后，后端启动日志应该看到：

```text
redis connected
```

## 任务 4：在 main.go 初始化 Redis

### 任务目标

让程序启动时同时连接 PostgreSQL 和 Redis。

### 你要新增或修改什么

修改：

```text
E:\game-realtime-gm\backend\cmd\server\main.go
```

操作类型：整文件替换。

### 这个操作有什么用

`main.go` 是程序入口。

数据库连接、Redis 连接、路由创建都属于服务启动时要准备的基础设施，所以适合在这里初始化。

### 代码内容

把 `main.go` 整文件替换为：

```go
package main

import (
	"context"
	"log"
	"net/http"

	"game-realtime-gm/backend/internal/cache"
	"game-realtime-gm/backend/internal/config"
	"game-realtime-gm/backend/internal/database"
	"game-realtime-gm/backend/internal/router"
)

func main() {
	cfg := config.Load()
	ctx := context.Background()

	dbPool, err := database.NewPostgresPool(ctx, cfg.Database)
	if err != nil {
		log.Fatal("connect database failed: ", err)
	}
	defer dbPool.Close()
	log.Println("database connected")

	redisClient, err := cache.NewRedisClient(ctx, cfg.Redis)
	if err != nil {
		log.Fatal("connect redis failed: ", err)
	}
	defer redisClient.Close()
	log.Println("redis connected")

	r := router.New(dbPool, redisClient, cfg)

	srv := &http.Server{
		Addr:    ":" + cfg.AppPort,
		Handler: r,
	}

	log.Println("server listening on :" + cfg.AppPort)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
```

### 代码分析

```go
"game-realtime-gm/backend/internal/cache"
```

引入刚才创建的 Redis 连接模块。

```go
redisClient, err := cache.NewRedisClient(ctx, cfg.Redis)
```

创建 Redis 客户端，并用配置中的地址连接 Redis。

```go
defer redisClient.Close()
```

程序退出前关闭 Redis 客户端。

```go
r := router.New(dbPool, redisClient, cfg)
```

路由现在不只接收 PostgreSQL 连接池，还接收 Redis 客户端。

### 和项目其他部分的关系

调用链变成：

```text
main.go
  ├── 初始化 PostgreSQL
  ├── 初始化 Redis
  └── 创建 router
           ↓
      创建 handler
```

### 怎么验证成功

先确保 Docker Desktop 已启动，然后执行：

```powershell
cd E:\game-realtime-gm\deploy
docker compose up -d

cd E:\game-realtime-gm\backend
go run .\cmd\server
```

期望日志：

```text
database connected
redis connected
server listening on :8080
```

## 任务 5：创建在线状态接口

### 任务目标

实现玩家在线心跳和在线状态查询。

### 你要新增或修改什么

新增：

```text
E:\game-realtime-gm\backend\internal\handler\online.go
```

操作类型：新建文件。

### 这个操作有什么用

真实实时服务需要知道哪些玩家在线。

今天先用 HTTP 心跳模拟：

```text
玩家每隔一段时间请求一次 heartbeat
        ↓
Redis 在线 key 的 TTL 被刷新
        ↓
如果长时间没有心跳，key 自动过期
        ↓
玩家被视为离线
```

后面进入 WebSocket 后，在线状态会和连接建立、心跳消息、连接断开结合起来。

### 代码内容

创建 `online.go` 并写入：

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
	playerID, ok := currentPlayerID(c)
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
	playerID, ok := currentPlayerID(c)
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

func currentPlayerID(c *gin.Context) (int64, bool) {
	playerIDValue, exists := c.Get(middleware.ContextKeyPlayerID)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    40105,
			"message": "player identity missing",
		})
		return 0, false
	}

	playerID, ok := playerIDValue.(int64)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    40106,
			"message": "invalid player identity",
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
const onlineTTL = 2 * time.Minute
```

在线状态 key 的有效时间是 2 分钟。

每次收到心跳，TTL 会重新刷新为 2 分钟。

```go
h.redisClient.Set(ctx, key, "1", onlineTTL)
```

向 Redis 写入：

```text
key:   online:player:<玩家ID>
value: 1
TTL:   120 秒
```

```go
h.redisClient.Exists(ctx, key)
```

判断 key 是否存在。

- 存在：玩家在线。
- 不存在：玩家离线。

```go
func currentPlayerID(c *gin.Context) (int64, bool)
```

从鉴权中间件写入的 `gin.Context` 里获取玩家 ID。

```go
func onlinePlayerKey(playerID int64) string
```

统一生成 Redis key。

集中生成 key 可以避免不同文件里手写字符串导致格式不一致。

### 和项目其他部分的关系

Day 03 已经在中间件中保存：

```go
c.Set(ContextKeyPlayerID, claims.PlayerID)
```

Day 04 的 handler 会读取这个玩家 ID，然后写 Redis。

调用链是：

```text
JWT -> middleware -> player_id -> online handler -> Redis
```

## 任务 6：把在线接口挂到路由上

### 任务目标

让客户端可以访问在线心跳和在线状态接口。

### 你要新增或修改什么

修改：

```text
E:\game-realtime-gm\backend\internal\router\router.go
```

操作类型：整文件替换。

### 代码内容

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
	protected.POST("/online/heartbeat", onlineHandler.Heartbeat)
	protected.GET("/online/status", onlineHandler.Status)

	return r
}
```

### 代码分析

```go
"github.com/redis/go-redis/v9"
```

路由初始化函数需要接收 Redis 客户端，所以引入 Redis 包。

```go
func New(db *pgxpool.Pool, redisClient *redis.Client, cfg config.Config) http.Handler
```

和之前相比，新增：

```go
redisClient *redis.Client
```

```go
onlineHandler := handler.NewOnlineHandler(redisClient)
```

创建在线状态 handler。

```go
protected.POST("/online/heartbeat", onlineHandler.Heartbeat)
protected.GET("/online/status", onlineHandler.Status)
```

两个接口都放在 `protected` 路由组里。

这表示它们必须通过 JWT 鉴权。

完整路径：

```text
POST /api/online/heartbeat
GET  /api/online/status
```

## 任务 7：格式化、编译并启动

### 任务目标

检查代码能否编译，确认 Redis 连接成功。

### 具体操作步骤

执行：

```powershell
cd E:\game-realtime-gm\backend
gofmt -w .\internal\config\config.go .\internal\cache\redis.go .\internal\handler\online.go .\internal\router\router.go .\cmd\server\main.go
go mod tidy
go test ./...
```

如果测试通过，启动后端：

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

### 常见错误

错误：

```text
connect redis failed
```

原因：Redis 容器没有运行。

解决：

```powershell
cd E:\game-realtime-gm\deploy
docker compose up -d
```

## 任务 8：用 Apifox 测试在线状态

### 任务目标

验证在线状态接口真的会写入 Redis。

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

### 第 2 步：发送在线心跳

请求：

```text
POST http://localhost:8080/api/online/heartbeat
```

Headers：

```text
Authorization: Bearer 你的token
```

成功返回：

```json
{
  "code": 0,
  "message": "heartbeat success",
  "data": {
    "online": true,
    "ttl_seconds": 120
  }
}
```

### 第 3 步：查询在线状态

请求：

```text
GET http://localhost:8080/api/online/status
```

Headers：

```text
Authorization: Bearer 你的token
```

成功返回：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "online": true
  }
}
```

## 任务 9：用 redis-cli 检查在线 key

### 任务目标

直接观察 Go 后端写入 Redis 的数据。

### 具体操作步骤

在新的 PowerShell 窗口执行：

```powershell
docker exec -it game_realtime_redis redis-cli
```

进入 Redis CLI 后执行：

```text
KEYS online:*
```

应该看到类似：

```text
1) "online:player:1"
```

读取 value：

```text
GET online:player:1
```

期望返回：

```text
"1"
```

查看 TTL：

```text
TTL online:player:1
```

应该看到小于等于 `120` 的数字。

### 命令分析

```text
KEYS online:*
```

查找以 `online:` 开头的 key。

注意：`KEYS` 适合今天本地学习，不适合在数据量很大的线上 Redis 使用。

线上通常使用：

```text
SCAN
```

```text
TTL online:player:1
```

查看这个玩家的在线状态还有多少秒过期。

### 怎么观察自动离线

停止发送心跳，等待 2 分钟以上。

再次执行：

```text
GET online:player:1
```

期望返回：

```text
(nil)
```

这表示 key 已经过期，玩家自动变成离线。

## 今日验收清单

- [ ] 能用 `redis-cli` 执行 `PING` 并看到 `PONG`。
- [ ] 配置模块新增 Redis 配置。
- [ ] Go 后端能连接 Redis。
- [ ] 后端日志出现 `redis connected`。
- [ ] 登录玩家能调用 `/api/online/heartbeat`。
- [ ] Redis 中出现 `online:player:<id>`。
- [ ] 在线 key 有 TTL。
- [ ] `/api/online/status` 返回在线状态。
- [ ] 停止心跳后，在线 key 会自动过期。
- [ ] `go test ./...` 编译通过。

## 今日不要做

今天不要做：

- WebSocket 长连接
- WebSocket 心跳消息
- 房间创建和广播
- 匹配队列
- 排行榜
- Redis 集群
- 分布式锁

今天先理解 Redis key、TTL 和在线状态。

## 今天学完后你应该能说清楚

- PostgreSQL 和 Redis 的分工。
- Redis 的 key-value 是什么。
- TTL 为什么适合在线状态。
- 为什么在线状态不适合每几秒写一次 PostgreSQL。
- Go 后端启动时如何检查 Redis 是否可用。
- JWT 中间件如何把 player_id 传给在线状态 handler。
- HTTP 心跳如何刷新 Redis TTL。
- 后续 WebSocket 如何接替 HTTP 心跳。

## 面试怎么讲这一部分

可以这样说：

```text
项目使用 PostgreSQL 保存玩家账号等长期数据，使用 Redis 保存玩家在线状态等高频临时数据。玩家发送心跳时，后端按 online:player:<id> 的格式写入 Redis，并设置 TTL。后续心跳会刷新 TTL；如果连接中断且不再续期，key 会自动过期，从而实现玩家离线判定。接口通过 JWT 鉴权中间件获取 player_id，避免客户端伪造玩家身份。
```
