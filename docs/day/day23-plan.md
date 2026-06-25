# Day 23 学习计划：WebSocket ping/pong 心跳与读写超时

## 你今天从任务几开始

今天从任务 1 开始。

Day 22 已经完成：

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

Day 23 要继续完善 WebSocket 连接生命周期：

```text
服务端定时发送 ping
        ↓
客户端自动或手动返回 pong
        ↓
服务端收到 pong 后刷新读超时时间
        ↓
如果长期收不到 pong，ReadMessage 报错并退出连接
        ↓
连接退出后停止 Redis 在线续期
```

今天继续保持小步推进，不做小队房间、不做小队状态广播、不做 PVE 任务匹配。

## 今日目标

今天要实现：

```text
WebSocket ping/pong 心跳与读写超时。
```

这个能力后续会用于共斗 PVE 小队房间和任务副本中判断玩家连接是否还活着。

## 今日最终效果

玩家连接：

```text
ws://localhost:8080/ws?token=玩家token
```

连接成功后：

```text
服务端会按固定间隔发送 ping。
正常客户端会返回 pong。
服务端收到 pong 后延长读超时时间。
如果客户端断网、关闭页面、工具异常断开，服务端能更快发现并清理连接。
```

后端日志中可以看到类似：

```text
websocket pong received: player_id=1 username=player01 data=
websocket disconnected: player_id=1 username=player01 remote=127.0.0.1:xxxxx
```

Redis 在线状态仍然保持 Day 22 的设计：

```text
连接正常时持续续期 online:player:<player_id>
连接断开后停止续期
TTL 到期后自动过期
```

## 今日会学到什么

你今天会学到：

- WebSocket ping 和 pong 是什么。
- 为什么长连接需要心跳。
- `SetReadDeadline` 解决什么问题。
- `SetPongHandler` 什么时候触发。
- 为什么服务端要定时发送 ping。
- 为什么每次写 WebSocket 前要设置写超时。
- 为什么同一个 WebSocket 连接不能随便多个 goroutine 同时写。
- 为什么今天要给写操作加 `sync.Mutex`。
- ping/pong 心跳和 Redis 在线续期的区别。

一句话理解：

```text
Day 22 解决“玩家在线状态怎么续期”，Day 23 解决“服务端怎么发现连接已经死了”。
```

## 今日文件范围

今天主要涉及：

```text
E:\game-realtime-gm\backend\internal\handler\ws.go
E:\game-realtime-gm\docs\api-overview.md
E:\game-realtime-gm\docs\day\day23-plan.md
```

文件职责说明：

| 文件 | 作用 |
| --- | --- |
| `handler/ws.go` | WebSocket 连接入口，今天新增 ping/pong 心跳、读超时、写超时和写锁 |
| `docs/api-overview.md` | 更新 `/ws` 说明，补充 Day 23 心跳行为 |
| `docs/day/day23-plan.md` | 今天这份学习教程 |

今天不新增数据库表。

今天不新增新依赖。

今天不改 `router.go`。

原因：

```text
Day 23 只增强 WebSocket handler 的连接生命周期。
路由地址仍然是 /ws。
router.go 已经在 Day 22 把 redisClient 传给 WebSocketEcho。
```

## 任务 0：确认当前状态

### 你要做什么

先确认 Day 22 代码已经在当前工作区里。

### 执行命令

```powershell
cd E:\game-realtime-gm
git status -sb
```

理想状态：

```text
## main...origin/main
```

如果仍然看到这些内部文件未跟踪，是正常的，不要提交它们：

```text
docs/codex-context.md
docs/conversation-handoff-gpt55.md
docs/mcp-adoption-plan.md
docs/skill-adoption-plan.md
```

### 再执行后端测试

```powershell
cd E:\game-realtime-gm\backend
go test ./...
```

成功时应该看到多个包：

```text
[no test files]
```

这不是错误，表示当前这些包还没有测试文件，但代码可以编译。

## 任务 1：理解为什么 Day 23 要做 ping/pong

### WebSocket 为什么需要心跳

WebSocket 是长连接。

连接建立后，客户端和服务端不会像普通 HTTP 那样“一次请求一次响应”。

问题是：

```text
客户端断网
电脑休眠
浏览器崩溃
网络中间设备断开连接
```

这些情况不一定会立刻让服务端收到正常关闭事件。

如果服务端一直不知道连接已经死了，就可能出现：

```text
Go 内存里还记录 player_id -> connection
Redis 在线状态还在续期
GM 后台看到玩家仍然在线
后续小队房间里玩家一直占着位置
```

### ping/pong 是什么

可以简单理解为：

```text
ping：服务端问“你还在吗？”
pong：客户端回“我还在。”
```

WebSocket 协议本身支持 ping/pong 控制帧。

很多客户端工具或浏览器会自动响应服务端 ping。

### 今天的设计

今天采用：

```text
服务端每 30 秒发送一次 ping
客户端需要在 70 秒内返回 pong
服务端收到 pong 后把读超时延长到当前时间 + 70 秒
如果 70 秒内没有任何 pong，ReadMessage 会因为超时返回 error
handler 退出，连接被清理
Redis 在线续期停止
```

## 任务 2：理解 gorilla/websocket 的并发规则

### 为什么要小心并发写

Day 22 的 `ws.go` 中，只有读循环收到客户端消息后会写回 echo。

Day 23 会新增一个 goroutine 定时发送 ping。

这意味着同一个连接可能出现两个写操作来源：

```text
读循环收到消息 -> 写 echo
ping goroutine 到时间 -> 写 ping
```

`gorilla/websocket` 的规则是：

```text
一个连接可以有一个读 goroutine 和一个写 goroutine。
不要让多个 goroutine 同时写同一个连接。
```

为了当前学习阶段少改结构，今天不拆 `readPump/writePump`，而是在写操作前加一个写锁：

```go
writeMu := &sync.Mutex{}
```

所有写操作通过同一个 helper：

```go
writeWebSocketJSON(...)
writeWebSocketMessage(...)
writeWebSocketPing(...)
```

这样可以避免 ping 和 echo 同时写连接。

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

- import 增加 `sync`
- 新增 WebSocket 心跳时间常量
- 连接建立后设置最大消息大小
- 连接建立后设置读超时
- 设置 `SetPongHandler`
- 启动 ping goroutine
- 给所有写操作加写锁
- 保留 Day 22 的 Redis 在线续期逻辑

这些改动都集中在 `ws.go`，整文件替换更不容易漏。

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
	"sync"
	"time"

	tokenauth "game-realtime-gm/backend/internal/auth"
	realtimews "game-realtime-gm/backend/internal/ws"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
)

const (
	webSocketWriteWait       = 10 * time.Second
	webSocketPongWait        = 70 * time.Second
	webSocketPingPeriod      = 30 * time.Second
	webSocketMaxMessageBytes = 4096
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

		writeMu := &sync.Mutex{}
		conn.SetReadLimit(webSocketMaxMessageBytes)
		_ = conn.SetReadDeadline(time.Now().Add(webSocketPongWait))
		conn.SetPongHandler(func(appData string) error {
			log.Printf("websocket pong received: player_id=%d username=%s data=%s", claims.PlayerID, claims.Username, appData)
			return conn.SetReadDeadline(time.Now().Add(webSocketPongWait))
		})

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
			_ = writeWebSocketJSON(conn, writeMu, WSMessage{
				Type:       "error",
				Content:    "update online status failed",
				ServerTime: time.Now(),
				PlayerID:   claims.PlayerID,
				Username:   claims.Username,
			})
			return
		}

		go keepWebSocketOnlineStatus(onlineCtx, redisClient, claims.PlayerID)
		go keepWebSocketAlive(onlineCtx, conn, writeMu, claims.PlayerID, claims.Username)

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

		if err := writeWebSocketJSON(conn, writeMu, welcome); err != nil {
			log.Printf("websocket write welcome failed: %v", err)
			return
		}

		for {
			messageType, message, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
					log.Printf("websocket read failed: player_id=%d err=%v", claims.PlayerID, err)
				}
				return
			}

			log.Printf("websocket received from player_id=%d: %s", claims.PlayerID, string(message))

			if err := writeWebSocketMessage(conn, writeMu, messageType, message); err != nil {
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

func keepWebSocketAlive(ctx context.Context, conn *websocket.Conn, writeMu *sync.Mutex, playerID int64, username string) {
	ticker := time.NewTicker(webSocketPingPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := writeWebSocketPing(conn, writeMu); err != nil {
				log.Printf("websocket ping failed: player_id=%d username=%s err=%v", playerID, username, err)
				_ = conn.Close()
				return
			}
		}
	}
}

func writeWebSocketJSON(conn *websocket.Conn, writeMu *sync.Mutex, value any) error {
	writeMu.Lock()
	defer writeMu.Unlock()

	if err := conn.SetWriteDeadline(time.Now().Add(webSocketWriteWait)); err != nil {
		return err
	}
	return conn.WriteJSON(value)
}

func writeWebSocketMessage(conn *websocket.Conn, writeMu *sync.Mutex, messageType int, message []byte) error {
	writeMu.Lock()
	defer writeMu.Unlock()

	if err := conn.SetWriteDeadline(time.Now().Add(webSocketWriteWait)); err != nil {
		return err
	}
	return conn.WriteMessage(messageType, message)
}

func writeWebSocketPing(conn *websocket.Conn, writeMu *sync.Mutex) error {
	writeMu.Lock()
	defer writeMu.Unlock()

	deadline := time.Now().Add(webSocketWriteWait)
	if err := conn.SetWriteDeadline(deadline); err != nil {
		return err
	}
	return conn.WriteControl(websocket.PingMessage, []byte("ping"), deadline)
}
```

## 任务 4：理解新增常量

### webSocketWriteWait

```go
webSocketWriteWait = 10 * time.Second
```

意思是：

```text
每次写 WebSocket 最多等待 10 秒。
```

如果客户端网络很差，服务端不能永远卡在写操作上。

### webSocketPongWait

```go
webSocketPongWait = 70 * time.Second
```

意思是：

```text
服务端希望在 70 秒内收到客户端 pong。
```

收到 pong 后，会重新执行：

```go
conn.SetReadDeadline(time.Now().Add(webSocketPongWait))
```

### webSocketPingPeriod

```go
webSocketPingPeriod = 30 * time.Second
```

意思是：

```text
服务端每 30 秒发一次 ping。
```

为什么 30 秒小于 70 秒？

```text
因为 ping 间隔必须小于 pong 等待时间。
这样客户端有机会在读超时前返回 pong。
```

### webSocketMaxMessageBytes

```go
webSocketMaxMessageBytes = 4096
```

意思是：

```text
单条 WebSocket 消息最大 4096 字节。
```

这可以避免客户端发送超大消息占用服务端内存。

当前只是学习项目，4KB 足够测试 echo、心跳和后续简单小队消息。

## 任务 5：理解 SetReadDeadline 和 SetPongHandler

### SetReadDeadline

```go
_ = conn.SetReadDeadline(time.Now().Add(webSocketPongWait))
```

它的作用是：

```text
如果在指定时间前没有读到任何数据，ReadMessage 会返回超时错误。
```

这里的“数据”包括普通消息和控制消息。

### SetPongHandler

```go
conn.SetPongHandler(func(appData string) error {
	log.Printf("websocket pong received: player_id=%d username=%s data=%s", claims.PlayerID, claims.Username, appData)
	return conn.SetReadDeadline(time.Now().Add(webSocketPongWait))
})
```

它的作用是：

```text
服务端收到客户端 pong 后，刷新读超时时间。
```

这就像客户端每次回复“我还在”，服务端就愿意继续等它。

## 任务 6：理解写锁和写 helper

### 为什么要有 writeMu

今天新增：

```go
writeMu := &sync.Mutex{}
```

因为现在有多个地方会写 WebSocket：

```text
welcome 消息
Redis 失败时 error 消息
echo 回显消息
定时 ping 消息
```

其中 ping 是 goroutine 里写，echo 是读循环里写。

为了避免同时写同一个连接，所有写操作都通过 helper 统一加锁。

### writeWebSocketJSON

```go
func writeWebSocketJSON(conn *websocket.Conn, writeMu *sync.Mutex, value any) error
```

负责：

```text
给 JSON 写操作加锁
设置写超时
写出 JSON
```

### writeWebSocketMessage

```go
func writeWebSocketMessage(conn *websocket.Conn, writeMu *sync.Mutex, messageType int, message []byte) error
```

负责：

```text
给普通消息写操作加锁
设置写超时
回写客户端发来的消息
```

### writeWebSocketPing

```go
func writeWebSocketPing(conn *websocket.Conn, writeMu *sync.Mutex) error
```

负责：

```text
给 ping 写操作加锁
设置写超时
发送 WebSocket PingMessage
```

## 任务 7：格式化和编译验证

### 执行命令

```powershell
cd E:\game-realtime-gm\backend
gofmt -w .\internal\handler\ws.go
go test ./...
```

### 命令解释

```text
gofmt：格式化 Go 文件，让 import 和缩进符合 Go 标准。
go test ./...：编译并测试 backend 下所有包。
```

### 成功结果

应该看到类似：

```text
?       game-realtime-gm/backend/internal/handler [no test files]
?       game-realtime-gm/backend/internal/ws      [no test files]
```

如果出现：

```text
imported and not used
```

说明 import 里有没用到的包，先检查 `sync`、`time` 是否真的被使用。

## 任务 8：启动 Docker Desktop 依赖

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

### 为什么今天仍然需要 Redis

Day 23 没有改变 Day 22 的在线状态逻辑。

WebSocket 连接成功后仍然会写：

```text
online:player:<player_id>
```

如果 Redis 没启动，WebSocket 连接可能会返回：

```text
update online status failed
```

然后关闭连接。

## 任务 9：启动后端服务

### 默认启动

```powershell
cd E:\game-realtime-gm\backend
go run .\cmd\server
```

### 如果 8080 端口不能用

你之前遇到过 Windows 端口排除范围问题。

如果再次报：

```text
listen tcp :8080: bind
```

临时使用：

```powershell
cd E:\game-realtime-gm\backend
$env:APP_PORT="18080"
go run .\cmd\server
```

测试地址也要改成：

```text
http://localhost:18080
ws://localhost:18080/ws?token=玩家token
```

## 任务 10：准备玩家 token

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

不要复制双引号。

## 任务 11：用 Apifox 测试 WebSocket 心跳

### 连接 WebSocket

Apifox WebSocket 地址：

```text
ws://localhost:8080/ws?token=玩家token
```

如果使用 `18080`：

```text
ws://localhost:18080/ws?token=玩家token
```

连接成功后 welcome 应该仍然类似：

```json
{
  "type": "welcome",
  "content": "connected to game realtime server",
  "server_time": "2026-06-25T10:00:00+08:00",
  "player_id": 1,
  "username": "player01",
  "online_players": 1,
  "online_ttl_seconds": 120
}
```

### 观察后端日志

保持连接 30 秒以上。

正常情况下，客户端收到 ping 后会返回 pong，后端日志可能看到：

```text
websocket pong received: player_id=1 username=player01 data=ping
```

如果 Apifox 不展示 ping/pong，也没关系，重点看后端日志和连接是否保持。

### 发送普通消息

在 Apifox 里发送：

```text
hello day23
```

预期：

```text
服务端仍然原样回显 hello day23
```

这证明：

```text
ping/pong 心跳没有破坏原来的 echo 能力。
```

## 任务 12：验证 Redis 在线状态仍然续期

### 进入 Redis

新开一个 PowerShell：

```powershell
docker exec -it game_realtime_redis redis-cli
```

### 查看 key

```text
KEYS online:*
```

应该看到：

```text
1) "online:player:1"
```

### 查看 TTL

```text
TTL online:player:1
```

第一次可能看到：

```text
110
```

保持 WebSocket 连接 70 秒左右，再查：

```text
TTL online:player:1
```

如果 Day 22 的续期逻辑仍然正常，TTL 应该会回到接近：

```text
120
```

## 任务 13：验证异常断开后的行为

### 操作步骤

1. 关闭 Apifox WebSocket 连接。
2. 看后端日志是否出现：

```text
websocket disconnected
```

3. Redis 中立刻查：

```text
TTL online:player:1
```

你可能仍然看到大于 0 的数字。

这是正常的。

因为 Day 22 设计就是：

```text
断开后不主动删除 key，只停止续期，等待 TTL 自动过期。
```

4. 等最多约 2 分钟后查：

```text
EXISTS online:player:1
```

应该返回：

```text
(integer) 0
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

在 `/ws` 说明里补充 Day 23：

```text
Day 23 已为 WebSocket 增加 ping/pong 心跳与读写超时。
服务端会定时发送 ping。
客户端返回 pong 后，服务端刷新读超时时间。
如果长期收不到 pong，服务端会关闭连接并停止 Redis 在线续期。
当前暂不做小队状态广播。
```

可以放在已有说明代码块中。

## 任务 15：今日复盘

完成后用自己的话回答：

1. WebSocket ping/pong 解决什么问题？
2. 为什么只靠 Redis TTL 还不够？
3. `SetReadDeadline` 的作用是什么？
4. `SetPongHandler` 在什么时候触发？
5. 为什么 `webSocketPingPeriod` 要小于 `webSocketPongWait`？
6. 为什么 WebSocket 写操作要加 `sync.Mutex`？
7. Day 22 的 Redis 续期和 Day 23 的 ping/pong 心跳有什么区别？
8. 如果客户端断网，后端大概会经历哪些清理步骤？

参考表达：

```text
Day 23 我给 WebSocket 增加了 ping/pong 心跳和读写超时。
服务端每隔一段时间发送 ping，客户端返回 pong 后，服务端会刷新读超时时间。
如果长时间收不到 pong，ReadMessage 会超时返回错误，handler 退出，连接管理器会注销该玩家连接，Redis 在线状态续期 goroutine 也会停止。
为了避免 ping goroutine 和 echo 写操作同时写同一个 WebSocket 连接，我给写操作加了 sync.Mutex。
```

## 今日验收清单

- [ ] `handler/ws.go` 已新增 `sync` import。
- [ ] `handler/ws.go` 已新增 WebSocket 心跳相关常量。
- [ ] WebSocket 连接建立后已设置 `SetReadLimit`。
- [ ] WebSocket 连接建立后已设置 `SetReadDeadline`。
- [ ] WebSocket 连接建立后已设置 `SetPongHandler`。
- [ ] 已新增 `keepWebSocketAlive`。
- [ ] 已新增 `writeWebSocketJSON`。
- [ ] 已新增 `writeWebSocketMessage`。
- [ ] 已新增 `writeWebSocketPing`。
- [ ] welcome 消息仍然可以正常返回。
- [ ] 普通文本消息仍然可以 echo。
- [ ] 后端能收到 pong 或连接能保持。
- [ ] WebSocket 断开后会停止 Redis 在线续期。
- [ ] Redis 在线状态仍然能随连接保持而续期。
- [ ] `gofmt` 执行成功。
- [ ] `go test ./...` 通过。
- [ ] `docs/api-overview.md` 已更新。

## 常见问题

### 1. go test 报 undefined: sync

检查 `handler/ws.go` import 里是否有：

```go
"sync"
```

### 2. go test 报 imported and not used

说明某个 import 没用上。

执行：

```powershell
gofmt -w .\internal\handler\ws.go
```

如果仍然报错，就检查是不是代码片段漏粘，导致 `sync` 或 `time` 没被使用。

### 3. 连接后很快断开

优先检查：

```text
Redis 是否启动
玩家 token 是否正确
后端日志是否有 websocket update redis online status failed
后端日志是否有 websocket ping failed
```

如果是 ping 失败，可能是客户端工具没有正常维持连接，或连接已经被关闭。

### 4. 看不到 pong 日志

可能原因：

```text
Apifox 没有展示控制帧
客户端没有自动回应 pong
连接还没保持到 30 秒
后端日志被其他内容刷过去
```

先保持连接 1 分钟，再观察后端终端。

### 5. TTL 还是会过期

优先检查：

```text
WebSocket 是否还连接着
后端是否仍在运行
Redis 是否仍在运行
后端日志是否有 websocket disconnected
后端日志是否有 websocket refresh redis online status failed
```

如果 WebSocket 断开，TTL 过期是正常行为。

### 6. concurrent write to websocket connection

如果看到类似：

```text
concurrent write to websocket connection
```

说明有某个写操作没有经过 `writeMu`。

检查所有写 WebSocket 的地方是否都使用：

```go
writeWebSocketJSON
writeWebSocketMessage
writeWebSocketPing
```

不要直接写：

```go
conn.WriteJSON(...)
conn.WriteMessage(...)
conn.WriteControl(...)
```

## 今日不要做什么

今天不要做：

- 小队房间系统。
- 小队状态广播。
- PVE 任务匹配。
- 任务副本生命周期。
- connection_id 安全删除 Redis key。
- WebSocket 私聊消息。
- 统一消息协议。
- 前端 GM 页面。
- UDP / KCP / QUIC。

原因：

```text
今天只解决“WebSocket 怎么主动发现死连接”的最小闭环。
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

### 选择性添加 Day 23 相关文件

```powershell
git add backend/internal/handler/ws.go docs/api-overview.md docs/day/day23-plan.md
```

不要使用：

```powershell
git add .
```

### 提交

```powershell
git commit -m "Complete day23 websocket heartbeat"
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

Day 24 建议继续 WebSocket 主线：

```text
统一 WebSocket 消息协议
```

目标：

```text
不再只回显普通字符串。
开始定义 type、request_id、data、server_time 这类统一消息格式。
```

这会为后续：

```text
小队准备
小队广播
任务匹配
错误返回
```

打基础。
