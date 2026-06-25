# Day 22 学习计划：WebSocket 在线状态写入 Redis 并续期

## 你今天从任务几开始

今天从任务 1 开始。

Day 21 已经完成：

```text
玩家连接成功后，注册到 Go 进程内存连接管理器。
welcome 消息返回 online_players。
同一个玩家重复连接时，新连接替换旧连接。
```

Day 22 要把 WebSocket 和 Redis 在线状态接起来：

```text
玩家 WebSocket 连接成功
        ↓
写入 Redis online:player:<player_id>
        ↓
连接期间定时续期 TTL
        ↓
连接断开后停止续期
        ↓
Redis key 等待 TTL 自动过期
```

今天继续保持小步推进，不做 ping/pong 心跳，也不做小队状态广播。

## 今日目标

今天要实现：

```text
WebSocket 连接驱动 Redis 在线状态。
```

这个能力后续会用于共斗 PVE 小队房间、任务匹配和 GM 在线状态观察。

最终效果：

```text
玩家连上 /ws?token=xxx
        ↓
Redis 出现 online:player:<player_id>
        ↓
TTL 大约是 120 秒
        ↓
连接保持时 TTL 会被续期
        ↓
连接断开后 TTL 不再续期
        ↓
最多约 2 分钟后自动过期
```

## 今日最终效果

玩家连接：

```text
ws://localhost:8080/ws?token=玩家token
```

Redis 中可以看到：

```text
online:player:1
```

检查 TTL：

```text
TTL online:player:1
```

应该看到一个大于 0 的秒数，例如：

```text
118
```

连接保持超过 1 分钟后再查 TTL，它应该又被续回接近：

```text
120
```

断开 WebSocket 后，不会立刻删除 key，而是停止续期，让它自然过期。

## 今日会学到什么

你今天会学到：

- 为什么在线状态适合放 Redis。
- 为什么在线状态必须设置 TTL。
- WebSocket 连接如何驱动 Redis 状态。
- 什么是“续期”。
- 为什么 Day22 不主动删除 Redis key。
- `context.WithCancel` 如何停止后台续期 goroutine。
- 为什么不要把长期可靠数据只放 Redis。
- 内存连接管理器和 Redis 在线状态分别解决什么问题。

一句话理解：

```text
Day 21 让 Go 进程记住“谁连着”，Day 22 让 Redis 也知道“谁在线”。
```

## 今日文件范围

今天主要涉及：

```text
E:\game-realtime-gm\backend\internal\handler\ws.go
E:\game-realtime-gm\backend\internal\router\router.go
E:\game-realtime-gm\docs\api-overview.md
E:\game-realtime-gm\docs\day22-plan.md
```

文件职责说明：

| 文件 | 作用 |
| --- | --- |
| `handler/ws.go` | WebSocket 连接入口，今天新增 Redis 在线状态写入和续期 |
| `router/router.go` | 把 `redisClient` 传给 WebSocket handler |
| `docs/api-overview.md` | 更新 `/ws` 说明，补充 Redis 在线状态和 TTL |
| `docs/day22-plan.md` | 今天这份学习教程 |

今天不新增数据库表。

今天不新增新依赖。

今天不改 `backend/internal/handler/online.go`。

原因：

```text
online.go 里已经有 onlineTTL 和 onlinePlayerKey。
ws.go 和 online.go 同属 handler 包，可以复用这两个小工具。
```

## 任务 0：确认 Day 21 状态

### 你要做什么

先确认 Day 21 代码已经编译通过。

### 执行命令

```powershell
cd E:\game-realtime-gm\backend
go test ./...
```

成功时应该看到：

```text
?       game-realtime-gm/backend/internal/ws      [no test files]
?       game-realtime-gm/backend/internal/handler [no test files]
?       game-realtime-gm/backend/internal/router  [no test files]
```

我当前检查到：

```text
go test ./... 已通过。
```

你自己动手前仍建议再跑一次。

## 任务 1：理解今天为什么要接 Redis

### Day 21 的内存管理器解决了什么

Day 21 的 `ws.Manager` 维护：

```text
player_id -> WebSocket 连接对象
```

它解决的是：

```text
当前 Go 进程怎么找到某个玩家的连接。
```

例如以后要做：

```text
给 player_id=1 推送消息
给小队内玩家广播消息
```

就需要内存里的真实连接对象。

### Redis 在线状态解决什么

Redis 解决的是：

```text
快速判断某个玩家是否在线。
```

例如：

```text
GET /api/online/status
GM dashboard 统计在线玩家
匹配系统过滤在线玩家
好友列表显示在线状态
```

这些不一定需要拿到 WebSocket 连接对象，只需要知道：

```text
online:player:1 这个 key 是否存在。
```

### 为什么 Redis key 必须有 TTL

如果玩家断网、电脑关机、浏览器崩溃，后端未必能立刻收到正常断开事件。

如果 Redis key 没有 TTL：

```text
玩家可能永远显示在线。
```

所以在线状态必须这样设计：

```text
连接期间持续续期
断开后停止续期
TTL 到期自动消失
```

当前项目已有：

```go
const onlineTTL = 2 * time.Minute
```

今天继续复用这个 TTL。

## 任务 2：理解今天为什么不主动 Del Redis key

### 你可能会想

连接断开时直接执行：

```text
DEL online:player:1
```

看起来最干净。

### 为什么今天先不这么做

Day 21 已经支持：

```text
同一个玩家新连接替换旧连接。
```

这里有一个容易踩的坑：

```text
player01 旧连接还没完全退出
player01 新连接已经建立，并写入 Redis
旧连接的 defer 开始执行
旧连接如果 DEL online:player:1
就可能把新连接刚写入的在线状态删掉
```

要安全主动删除，需要引入：

```text
connection_id
Redis value 保存 connection_id
删除前比较当前 Redis value 是否属于自己
必要时用 Lua 脚本保证原子性
```

这对 Day22 来说偏复杂。

### 今天采用的策略

今天使用：

```text
断开后不主动删除 Redis key。
停止续期，等待 TTL 自动过期。
```

优点：

- 简单。
- 不会误删新连接。
- 符合在线状态 TTL 设计。

缺点：

```text
断开后最多约 2 分钟内，玩家可能仍显示在线。
```

这个对当前学习项目可以接受。

后续可以再优化成：

```text
connection_id + 安全删除。
```

## 任务 3：整文件替换 handler/ws.go

### 修改文件

```text
E:\game-realtime-gm\backend\internal\handler\ws.go
```

操作类型：

```text
整文件替换。
```

### 为什么整文件替换

今天要同时修改：

- import 增加 `context`
- import 增加 `go-redis`
- `WebSocketEcho` 参数增加 `redisClient`
- 连接成功后写 Redis
- 启动后台续期 goroutine
- 连接结束时停止续期
- welcome 消息增加 Redis TTL 秒数

整文件替换更不容易漏。

### 完整代码

把下面完整内容替换到：

```text
E:\game-realtime-gm\backend\internal\handler\ws.go
```

```go
package handler

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"

	tokenauth "game-realtime-gm/backend/internal/auth"
	realtimews "game-realtime-gm/backend/internal/ws"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
)

type WSMessage struct {
	Type          string    `json:"type"`
	Content       string    `json:"content,omitempty"`
	ServerTime    time.Time `json:"server_time"`
	PlayerID      int64     `json:"player_id,omitempty"`
	Username      string    `json:"username,omitempty"`
	OnlinePlayers int       `json:"online_players,omitempty"`
	OnlineTTL     int       `json:"online_ttl_seconds,omitempty"`
}

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func WebSocketEcho(jwtSecret string, wsManager *realtimews.Manager, redisClient *redis.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims, ok := websocketPlayerClaims(c, jwtSecret)
		if !ok {
			return
		}

		conn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			log.Printf("websocket upgrade failed: %v", err)
			return
		}
		defer conn.Close()

		client := &realtimews.Client{
			PlayerID:    claims.PlayerID,
			Username:    claims.Username,
			Conn:        conn,
			ConnectedAt: time.Now(),
		}

		oldConn := wsManager.Register(client)
		if oldConn != nil {
			_ = oldConn.Close()
			log.Printf("websocket replaced old connection: player_id=%d username=%s", claims.PlayerID, claims.Username)
		}
		defer wsManager.Unregister(claims.PlayerID, conn)

		onlineCtx, stopOnlineRefresh := context.WithCancel(context.Background())
		defer stopOnlineRefresh()

		if err := refreshWebSocketOnlineStatus(onlineCtx, redisClient, claims.PlayerID); err != nil {
			log.Printf("websocket update redis online status failed: player_id=%d err=%v", claims.PlayerID, err)
			_ = conn.WriteJSON(WSMessage{
				Type:       "error",
				Content:    "update online status failed",
				ServerTime: time.Now(),
				PlayerID:   claims.PlayerID,
				Username:   claims.Username,
			})
			return
		}

		go keepWebSocketOnlineStatus(onlineCtx, redisClient, claims.PlayerID)

		remoteAddr := conn.RemoteAddr().String()
		log.Printf("websocket connected: player_id=%d username=%s remote=%s online_players=%d", claims.PlayerID, claims.Username, remoteAddr, wsManager.Count())
		defer log.Printf("websocket disconnected: player_id=%d username=%s remote=%s", claims.PlayerID, claims.Username, remoteAddr)

		welcome := WSMessage{
			Type:          "welcome",
			Content:       "connected to game realtime server",
			ServerTime:    time.Now(),
			PlayerID:      claims.PlayerID,
			Username:      claims.Username,
			OnlinePlayers: wsManager.Count(),
			OnlineTTL:     int(onlineTTL.Seconds()),
		}

		if err := conn.WriteJSON(welcome); err != nil {
			log.Printf("websocket write welcome failed: %v", err)
			return
		}

		for {
			messageType, message, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
					log.Printf("websocket read failed: %v", err)
				}
				return
			}

			log.Printf("websocket received from player_id=%d: %s", claims.PlayerID, string(message))

			if err := conn.WriteMessage(messageType, message); err != nil {
				log.Printf("websocket write echo failed: %v", err)
				return
			}
		}
	}
}

func websocketPlayerClaims(c *gin.Context, jwtSecret string) (*tokenauth.Claims, bool) {
	tokenString := strings.TrimSpace(c.Query("token"))
	if tokenString == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    40121,
			"message": "websocket token missing",
		})
		return nil, false
	}

	claims, err := tokenauth.ParseToken(jwtSecret, tokenString)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    40122,
			"message": "invalid websocket token",
		})
		return nil, false
	}

	if claims.SubjectType != tokenauth.SubjectTypePlayer || claims.PlayerID <= 0 {
		c.JSON(http.StatusForbidden, gin.H{
			"code":    40331,
			"message": "player token required",
		})
		return nil, false
	}

	return claims, true
}

func refreshWebSocketOnlineStatus(ctx context.Context, redisClient *redis.Client, playerID int64) error {
	key := onlinePlayerKey(playerID)
	return redisClient.Set(ctx, key, "1", onlineTTL).Err()
}

func keepWebSocketOnlineStatus(ctx context.Context, redisClient *redis.Client, playerID int64) {
	ticker := time.NewTicker(onlineTTL / 2)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := refreshWebSocketOnlineStatus(ctx, redisClient, playerID); err != nil {
				log.Printf("websocket refresh redis online status failed: player_id=%d err=%v", playerID, err)
			}
		}
	}
}
```

## 任务 4：理解 handler/ws.go 改动

### 新增 context

```go
"context"
```

`context` 用来控制后台续期任务什么时候停止。

今天会创建：

```go
onlineCtx, stopOnlineRefresh := context.WithCancel(context.Background())
defer stopOnlineRefresh()
```

意思是：

```text
WebSocket handler 还活着时，onlineCtx 有效。
WebSocket handler 结束时，调用 stopOnlineRefresh。
后台续期 goroutine 收到 ctx.Done 后退出。
```

### 新增 Redis Client

```go
"github.com/redis/go-redis/v9"
```

因为 `WebSocketEcho` 现在需要直接写 Redis：

```go
func WebSocketEcho(jwtSecret string, wsManager *realtimews.Manager, redisClient *redis.Client) gin.HandlerFunc
```

### WSMessage 新增 OnlineTTL

```go
OnlineTTL int `json:"online_ttl_seconds,omitempty"`
```

作用：

```text
告诉客户端 Redis 在线状态 TTL 是多少秒。
```

例如：

```json
{
  "online_ttl_seconds": 120
}
```

### refreshWebSocketOnlineStatus

```go
func refreshWebSocketOnlineStatus(ctx context.Context, redisClient *redis.Client, playerID int64) error {
	key := onlinePlayerKey(playerID)
	return redisClient.Set(ctx, key, "1", onlineTTL).Err()
}
```

这个函数负责写入或续期 Redis 在线状态。

它复用了 `online.go` 里的：

```go
onlinePlayerKey(playerID)
onlineTTL
```

因为 `ws.go` 和 `online.go` 都在：

```text
package handler
```

所以可以直接调用小写函数和常量。

Redis 最终写入：

```text
SET online:player:1 1 EX 120
```

### keepWebSocketOnlineStatus

```go
func keepWebSocketOnlineStatus(ctx context.Context, redisClient *redis.Client, playerID int64)
```

这个函数负责连接期间定时续期。

它每隔：

```go
onlineTTL / 2
```

执行一次续期。

当前：

```go
onlineTTL = 2 * time.Minute
```

所以续期间隔是：

```text
1 分钟
```

为什么不是等 2 分钟再续？

```text
如果刚好网络卡顿或 Redis 慢，key 可能先过期。
提前一半时间续期更稳。
```

### 为什么不主动 Del

这版代码断开时只做：

```go
stopOnlineRefresh()
```

没有执行：

```go
redisClient.Del(...)
```

原因是避免旧连接误删新连接。

断开后 Redis key 会在最多约 2 分钟后自动过期。

## 任务 5：修改 router.go

### 修改文件

```text
E:\game-realtime-gm\backend\internal\router\router.go
```

操作类型：

```text
局部修改。
```

### 搜索关键词

搜索：

```go
r.GET("/ws", handler.WebSocketEcho(cfg.JWTSecret, wsManager))
```

### 修改前

```go
r.GET("/ws", handler.WebSocketEcho(cfg.JWTSecret, wsManager))
```

### 修改后

```go
r.GET("/ws", handler.WebSocketEcho(cfg.JWTSecret, wsManager, redisClient))
```

### 为什么这样改

Day 22 的 `WebSocketEcho` 需要写 Redis。

`router.New` 本来就有：

```go
redisClient *redis.Client
```

所以直接把它传给 WebSocket handler。

请求链路变成：

```text
cmd/server/main.go
        ↓
cache.NewRedisClient
        ↓
router.New(db, redisClient, cfg)
        ↓
handler.WebSocketEcho(cfg.JWTSecret, wsManager, redisClient)
        ↓
refreshWebSocketOnlineStatus
        ↓
Redis SET online:player:<id> 1 EX 120
```

## 任务 6：格式化和编译验证

### 执行命令

```powershell
cd E:\game-realtime-gm\backend
gofmt -w .\internal\handler\ws.go .\internal\router\router.go
go test ./...
```

### 成功结果

应该看到：

```text
?       game-realtime-gm/backend/internal/handler [no test files]
?       game-realtime-gm/backend/internal/router  [no test files]
?       game-realtime-gm/backend/internal/ws      [no test files]
```

### 可选检查数据竞争

```powershell
go test -race ./...
```

这条会慢一些。

如果通过，说明目前测试覆盖到的路径没有发现数据竞争。

## 任务 7：启动 Docker Desktop 依赖

### 执行命令

```powershell
cd E:\game-realtime-gm\deploy
docker compose up -d
docker ps
```

应该看到：

```text
game_realtime_postgres
game_realtime_redis
```

### 为什么今天 Redis 必须启动

Day 22 的 `/ws` 连接成功后会立即写 Redis。

如果 Redis 没启动：

```text
WebSocket 连接可能建立后马上返回 error 并关闭。
```

## 任务 8：启动后端服务

### 默认启动

```powershell
cd E:\game-realtime-gm\backend
go run .\cmd\server
```

### 如果 8080 端口仍被 Windows 排除

你之前遇到过：

```text
listen tcp :8080: bind: An attempt was made to access a socket in a way forbidden by its access permissions.
```

如果你还没有释放 8080，可以临时用：

```powershell
cd E:\game-realtime-gm\backend
$env:APP_PORT="18080"
go run .\cmd\server
```

此时测试地址要改成：

```text
http://localhost:18080
ws://localhost:18080/ws?token=玩家token
```

如果你已经释放了 8080，就继续使用：

```text
localhost:8080
```

## 任务 9：准备玩家 token

如果你已经有玩家账号，直接登录：

```http
POST http://localhost:8080/api/login
Content-Type: application/json

{
  "username": "player01",
  "password": "123456"
}
```

如果使用 `18080`：

```http
POST http://localhost:18080/api/login
Content-Type: application/json

{
  "username": "player01",
  "password": "123456"
}
```

复制响应里的：

```text
data.token
```

注意不要复制双引号。

## 任务 10：连接 WebSocket 并检查 Redis

### 连接 WebSocket

Apifox WebSocket 地址：

```text
ws://localhost:8080/ws?token=玩家token
```

如果使用 `18080`：

```text
ws://localhost:18080/ws?token=玩家token
```

连接成功后 welcome 应该类似：

```json
{
  "type": "welcome",
  "content": "connected to game realtime server",
  "server_time": "2026-06-23T10:00:00+08:00",
  "player_id": 1,
  "username": "player01",
  "online_players": 1,
  "online_ttl_seconds": 120
}
```

### 进入 Redis

新开一个 PowerShell：

```powershell
docker exec -it game_realtime_redis redis-cli
```

### 查看 key

```text
KEYS online:*
```

应该能看到：

```text
1) "online:player:1"
```

### 查看值

```text
GET online:player:1
```

应该看到：

```text
"1"
```

### 查看 TTL

```text
TTL online:player:1
```

应该看到一个大于 0 的数字：

```text
118
```

## 任务 11：验证 TTL 会续期

### 操作步骤

保持 WebSocket 不断开。

第一次查：

```text
TTL online:player:1
```

可能是：

```text
115
```

等 70 秒左右，再查：

```text
TTL online:player:1
```

如果续期成功，它应该又接近：

```text
120
```

例如：

```text
112
119
```

都算正常。

### 为什么不是永远显示 120

TTL 每秒都在减少。

后台续期任务大约每 60 秒重新 `SET` 一次。

所以你看到的 TTL 会在：

```text
60 到 120
```

之间波动。

## 任务 12：验证断开后自动过期

### 操作步骤

1. 关闭 Apifox WebSocket 连接。
2. 立刻查：

```text
TTL online:player:1
```

你可能仍然看到：

```text
80
```

这是正常的。

因为 Day 22 不主动删除 key。

3. 等最多 2 分钟后再查：

```text
EXISTS online:player:1
```

应该返回：

```text
(integer) 0
```

### 为什么断开后不是立刻消失

因为今天采用：

```text
停止续期，等待 TTL 自动过期。
```

这比直接删除更保守，避免旧连接误删新连接。

## 任务 13：用 HTTP 在线状态接口交叉验证

### 请求

WebSocket 连接保持时，用同一个玩家 token 请求：

```http
GET http://localhost:8080/api/online/status
Authorization: Bearer 玩家token
```

如果使用 `18080`：

```http
GET http://localhost:18080/api/online/status
Authorization: Bearer 玩家token
```

### 预期响应

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "online": true
  }
}
```

### 断开后

断开 WebSocket 后，最多约 2 分钟内可能仍然返回：

```json
{
  "online": true
}
```

等 TTL 过期后，再请求应该变成：

```json
{
  "online": false
}
```

## 任务 14：更新接口文档

### 修改文件

```text
E:\game-realtime-gm\docs\api-overview.md
```

操作类型：

```text
局部修改。
```

### 搜索关键词

搜索：

```markdown
### GET /ws
```

### 修改重点

welcome 示例增加：

```json
{
  "online_ttl_seconds": 120
}
```

说明里补充：

```text
Day 22 已将 WebSocket 在线状态写入 Redis。
Redis key 使用 online:player:<player_id>。
连接期间后端会定时续期 TTL。
断开后停止续期，等待 TTL 自动过期。
当前不主动删除 Redis key，避免旧连接误删新连接的在线状态。
```

## 任务 15：今日复盘

完成后用自己的话回答：

1. 为什么 WebSocket 在线状态要写入 Redis？
2. Day 21 的内存连接管理器和 Day 22 的 Redis 在线状态有什么区别？
3. 为什么在线状态必须有 TTL？
4. 为什么今天不主动 `DEL online:player:<id>`？
5. `context.WithCancel` 在今天代码里解决了什么问题？
6. 为什么续期间隔用 `onlineTTL / 2`？
7. WebSocket 断开后，为什么 `/api/online/status` 可能短时间仍显示在线？
8. 如果 Redis 挂了，今天的 WebSocket 会有什么表现？

参考表达：

```text
Day 22 我把 WebSocket 连接和 Redis 在线状态打通。
玩家连接成功后，后端会写入 online:player:<player_id>，TTL 使用已有的 onlineTTL。
连接保持时，后台 goroutine 每隔 onlineTTL/2 续期一次。
连接断开时，通过 cancel 停止续期，不主动删除 Redis key，等待 TTL 自动过期。
这样可以避免旧连接断开时误删新连接的在线状态。
```

## 今日验收清单

- [ ] `handler/ws.go` 已引入 `context`。
- [ ] `handler/ws.go` 已引入 `github.com/redis/go-redis/v9`。
- [ ] `WebSocketEcho` 已接收 `redisClient *redis.Client`。
- [ ] `router.go` 已把 `redisClient` 传给 `WebSocketEcho`。
- [ ] WebSocket 连接成功后会写入 `online:player:<id>`。
- [ ] welcome 消息包含 `online_ttl_seconds`。
- [ ] Redis 中能查到 `online:player:<id>`。
- [ ] `TTL online:player:<id>` 大于 0。
- [ ] WebSocket 保持连接时 TTL 会被续期。
- [ ] WebSocket 断开后续期停止。
- [ ] 断开后 Redis key 等待 TTL 自动过期。
- [ ] `/api/online/status` 能和 WebSocket 在线状态联动。
- [ ] `gofmt` 执行成功。
- [ ] `go test ./...` 通过。
- [ ] 日志没有打印完整 token。
- [ ] `docs/api-overview.md` 已更新。

## 常见问题

### 1. go test 报 not enough arguments in call to handler.WebSocketEcho

说明 `router.go` 还停留在 Day 21 写法。

修改为：

```go
r.GET("/ws", handler.WebSocketEcho(cfg.JWTSecret, wsManager, redisClient))
```

### 2. go test 报 undefined: redis

检查 `handler/ws.go` 是否有：

```go
"github.com/redis/go-redis/v9"
```

并且函数参数是否是：

```go
redisClient *redis.Client
```

### 3. go test 报 undefined: context

检查 import 是否有：

```go
"context"
```

### 4. Redis 没有 online:player key

优先检查：

```text
WebSocket 是否真的连接成功
welcome 是否返回
Redis 容器是否运行
后端日志是否有 update redis online status failed
你查的 player_id 是否正确
```

### 5. TTL 是 -2

Redis 里：

```text
TTL key 返回 -2
```

表示 key 不存在。

原因可能是：

```text
WebSocket 没连接成功
key 已经过期
查错了 player_id
Redis 写入失败
```

### 6. TTL 是 -1

Redis 里：

```text
TTL key 返回 -1
```

表示 key 存在，但没有过期时间。

这不符合在线状态设计。

检查代码是否使用了：

```go
redisClient.Set(ctx, key, "1", onlineTTL)
```

不要写成：

```go
redisClient.Set(ctx, key, "1", 0)
```

### 7. 断开后状态没有立刻变 false

这是 Day 22 的预期行为。

断开后要等 TTL 自动过期。

当前 TTL 是：

```text
120 秒
```

所以最多约 2 分钟后才会变 false。

### 8. 8080 端口仍然不能用

如果报：

```text
listen tcp :8080: bind
```

先检查 Windows 排除范围或端口占用。

临时方案：

```powershell
$env:APP_PORT="18080"
go run .\cmd\server
```

测试地址也要改成 `18080`。

## 今日不要做什么

今天不要做：

- WebSocket ping/pong 心跳。
- Redis 主动安全删除 connection_id。
- 小队房间系统。
- 小队状态广播。
- PVE 任务匹配。
- 任务副本生命周期。
- 私聊消息。
- GM 后台在线玩家列表接口。
- 分布式多实例连接同步。
- 压测和性能优化。

原因：

```text
今天只解决“WebSocket 连接如何驱动 Redis 在线状态”的最小闭环。
```

## 任务 16：提交并推送到 GitHub

### 先检查状态

```powershell
cd E:\game-realtime-gm
git status
```

不要提交这些内部文件：

```text
docs/codex-context.md
docs/conversation-handoff-gpt55.md
docs/mcp-adoption-plan.md
docs/skill-adoption-plan.md
```

### 选择性添加 Day 22 相关文件

```powershell
git add backend/internal/handler/ws.go backend/internal/router/router.go README.md AGENTS.md docs/api-overview.md docs/day22-plan.md docs/learning-roadmap.md docs/project-technical-standards.md docs/project-direction-pve.md
```

如果某些文件当天没有修改，可以不加。

不要使用：

```powershell
git add .
```

### 提交

```powershell
git commit -m "Complete day22 websocket redis online"
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

如果仍然看到内部文档未跟踪，是正常的。

## 明日预告

Day 23 建议继续 WebSocket 主线：

```text
WebSocket ping/pong 心跳与读写超时
```

目标：

```text
服务端定时发送 ping
客户端返回 pong
服务端根据 pong 判断连接是否还活着
异常连接及时断开
```

这会让 WebSocket 连接从“能在线续期”进一步变成“能主动发现死连接”。
