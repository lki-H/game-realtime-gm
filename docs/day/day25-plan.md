# Day 25 学习计划：WebSocket 连接会话 ID

## 你今天从任务几开始

今天从任务 0 开始。

你已经完成了 Day24：

```text
WebSocket 玩家 token 鉴权
        ↓
建立玩家长连接
        ↓
服务端发送 server.welcome
        ↓
客户端发送统一 JSON 消息
        ↓
服务端按 type 处理 debug.echo
        ↓
服务端统一返回 code、message、data、server_time
```

Day25 继续 WebSocket 主线，但今天仍然不做小队房间。

今天只解决一个连接生命周期问题：

```text
同一个玩家重复连接时，服务端要能明确区分旧连接和新连接。
```

## 今日目标

今天要给每一次 WebSocket 连接生成一个独立的：

```text
connection_id
```

并在服务端连接管理器里记录：

```text
connection_id
player_id
username
connected_at
last_pong_at
conn
```

一句话理解：

```text
player_id 表示“是谁”，connection_id 表示“这是他的哪一次连接”。
```

## 今日最终效果

你用 Apifox 连接：

```text
ws://localhost:8080/ws?token=玩家token
```

连接成功后，`server.welcome` 里的 `data` 会多出连接会话信息：

```json
{
  "type": "server.welcome",
  "code": 0,
  "message": "ok",
  "data": {
    "connection_id": "conn_1_1780000000000000000_a1b2c3d4e5f60708",
    "player_id": 1,
    "username": "player01",
    "connected_at": "2026-06-29T10:00:00+08:00",
    "last_pong_at": "2026-06-29T10:00:00+08:00",
    "online_players": 1,
    "online_ttl_seconds": 120
  },
  "server_time": "2026-06-29T10:00:00+08:00"
}
```

Redis 里的在线状态 value 不再只是：

```text
1
```

而是当前连接的：

```text
connection_id
```

例如：

```text
GET online:player:1
"conn_1_1780000000000000000_a1b2c3d4e5f60708"
```

这样以后要做“安全删除在线状态”时，就可以判断：

```text
只有 Redis 里的 connection_id 仍然等于当前断开的 connection_id，才允许删除 online key。
```

今天暂时不主动删除 Redis key，仍然让 TTL 自然过期。

## 今日会学到什么

你今天会学到：

- 为什么只用 `player_id` 管连接还不够。
- 为什么同一个玩家重复连接会有“旧连接误删新连接”的风险。
- `connection_id` 和 `player_id` 的区别。
- 为什么连接会话信息适合放在 `internal/ws` 包里。
- 如何用 Go 标准库 `crypto/rand` 生成随机 ID。
- 为什么 Redis 在线 key 的 value 可以保存连接 ID。
- 为什么 pong 回调里要更新 `last_pong_at`。
- 为什么 Day25 是 Day26 小队房间前的重要铺垫。

## 今日文件范围

今天主要涉及：

```text
E:\game-realtime-gm\backend\internal\ws\session.go
E:\game-realtime-gm\backend\internal\ws\manager.go
E:\game-realtime-gm\backend\internal\ws\message.go
E:\game-realtime-gm\backend\internal\handler\ws.go
E:\game-realtime-gm\docs\api-overview.md
E:\game-realtime-gm\docs\day\day25-plan.md
```

文件职责说明：

| 文件 | 作用 |
| --- | --- |
| `internal/ws/session.go` | 新增连接 ID 生成函数 |
| `internal/ws/manager.go` | 在连接管理器中记录 connection_id 和 last_pong_at |
| `internal/ws/message.go` | 在 welcome 数据里补充连接会话字段 |
| `internal/handler/ws.go` | 建连时生成 connection_id，pong 时更新 last_pong_at，Redis value 写 connection_id |
| `docs/api-overview.md` | 更新 `/ws` 文档，补充 Day25 连接会话 ID |
| `docs/day/day25-plan.md` | 今天这份学习教程 |

今天不新增数据库表。

今天不新增第三方依赖。

今天不改路由。

今天不做小队房间。

## 任务 0：确认 Day24 状态

### 你要做什么

先确认 Day24 已经提交，当前仓库没有未提交代码。

### 执行命令

```powershell
cd E:\game-realtime-gm
git status -sb
```

理想情况：

```text
## main...origin/main
```

如果看到下面这些未跟踪文件，是正常的，不要提交：

```text
?? docs/codex-context.md
?? docs/conversation-handoff-gpt55.md
?? docs/mcp-adoption-plan.md
?? docs/skill-adoption-plan.md
```

### 再跑后端测试

```powershell
cd E:\game-realtime-gm\backend
go test ./...
```

成功时可能看到：

```text
?       game-realtime-gm/backend/internal/ws      [no test files]
?       game-realtime-gm/backend/internal/handler [no test files]
```

`[no test files]` 不是错误，它表示这个包目前没有测试文件，但能正常编译。

## 任务 1：理解为什么需要 connection_id

### 当前问题

现在连接管理器大概是这样工作的：

```text
player_id -> WebSocket 连接
```

这能表示：

```text
玩家 1 当前在线。
```

但不能清楚表示：

```text
玩家 1 当前在线的是哪一次连接。
```

假设玩家 1 连了两次：

```text
第一次连接：旧连接 A
第二次连接：新连接 B
```

如果新连接 B 替换了旧连接 A，旧连接 A 随后退出时，服务端必须避免把新连接 B 的状态删掉。

所以 Day25 要让连接变成：

```text
player_id=1
connection_id=conn_1_xxx
```

这样服务端能判断：

```text
当前要注销的连接，是不是连接管理器里仍然记录的那一个连接。
```

### 和真实项目的关系

在真实游戏后端里，类似概念常被称为：

```text
Session
Connection
Player Session
Presence Session
```

本项目现在不用复杂命名，先用最直观的：

```text
connection_id
```

## 任务 2：新增连接 ID 生成文件

### 修改文件

```text
E:\game-realtime-gm\backend\internal\ws\session.go
```

操作类型：

```text
新建文件。
```

### 为什么新建 session.go

`manager.go` 负责管理连接映射。

`message.go` 负责 WebSocket 消息格式。

`session.go` 负责连接会话相关的小工具，例如生成 `connection_id`。

这样文件职责更清楚。

### 完整代码

新建文件：

```text
E:\game-realtime-gm\backend\internal\ws\session.go
```

填入：

```go
package ws

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

func NewConnectionID(playerID int64, connectedAt time.Time) (string, error) {
	randomBytes := make([]byte, 8)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", err
	}

	return fmt.Sprintf("conn_%d_%d_%s", playerID, connectedAt.UnixNano(), hex.EncodeToString(randomBytes)), nil
}
```

### 代码分析

```go
package ws
```

表示这个文件属于 `internal/ws` 包。

它可以被 `handler/ws.go` 通过下面这个别名调用：

```go
realtimews.NewConnectionID(...)
```

```go
crypto/rand
```

用于生成更可靠的随机字节。

不要用普通 `math/rand` 生成这种连接标识。

```go
encoding/hex
```

把随机字节转成字符串。

例如字节：

```text
0xa1 0xb2
```

会变成：

```text
a1b2
```

```go
connectedAt.UnixNano()
```

把连接时间转成纳秒时间戳。

最终 ID 类似：

```text
conn_1_1780000000000000000_a1b2c3d4e5f60708
```

这个 ID 包含：

| 部分 | 含义 |
| --- | --- |
| `conn` | 表示这是连接 ID |
| `1` | player_id |
| `1780000000000000000` | 建连时间戳 |
| `a1b2...` | 随机后缀，避免同一瞬间冲突 |

## 任务 3：修改 Client 结构体

### 修改文件

```text
E:\game-realtime-gm\backend\internal\ws\manager.go
```

操作类型：

```text
局部修改。
```

### 搜索关键词

```go
type Client struct {
```

### 修改前

```go
type Client struct {
	PlayerID    int64
	Username    string
	Conn        *websocket.Conn
	ConnectedAt time.Time
}
```

### 修改后

```go
type Client struct {
	ConnectionID string
	PlayerID     int64
	Username     string
	Conn         *websocket.Conn
	ConnectedAt  time.Time
	LastPongAt   time.Time
}
```

### 字段解释

| 字段 | 作用 |
| --- | --- |
| `ConnectionID` | 当前这一次 WebSocket 连接的唯一标识 |
| `PlayerID` | 这条连接属于哪个玩家 |
| `Username` | 方便日志中识别玩家 |
| `Conn` | gorilla/websocket 的真实连接对象 |
| `ConnectedAt` | 连接建立时间 |
| `LastPongAt` | 最近一次收到 pong 的时间 |

## 任务 4：修改 Register 的旧连接判断

### 修改文件

```text
E:\game-realtime-gm\backend\internal\ws\manager.go
```

操作类型：

```text
局部修改。
```

### 搜索关键词

```go
func (m *Manager) Register
```

### 修改前

```go
func (m *Manager) Register(client *Client) *websocket.Conn {
	m.mu.Lock()
	defer m.mu.Unlock()

	oldClient, exists := m.clients[client.PlayerID]
	m.clients[client.PlayerID] = client

	if exists && oldClient.Conn != client.Conn {
		return oldClient.Conn
	}

	return nil
}
```

### 修改后

```go
func (m *Manager) Register(client *Client) *websocket.Conn {
	m.mu.Lock()
	defer m.mu.Unlock()

	oldClient, exists := m.clients[client.PlayerID]
	m.clients[client.PlayerID] = client

	if exists && oldClient.ConnectionID != client.ConnectionID {
		return oldClient.Conn
	}

	return nil
}
```

### 为什么这样改

之前用：

```go
oldClient.Conn != client.Conn
```

判断是否是不同连接。

Day25 后更推荐用：

```go
oldClient.ConnectionID != client.ConnectionID
```

因为 `connection_id` 是我们自己定义的业务会话标识，更容易写日志、返回给客户端、放到 Redis 里验证。

## 任务 5：修改 Unregister，避免旧连接删掉新连接

### 修改文件

```text
E:\game-realtime-gm\backend\internal\ws\manager.go
```

操作类型：

```text
局部修改。
```

### 搜索关键词

```go
func (m *Manager) Unregister
```

### 修改前

```go
func (m *Manager) Unregister(playerID int64, conn *websocket.Conn) {
	m.mu.Lock()
	defer m.mu.Unlock()

	currentClient, exists := m.clients[playerID]
	if !exists {
		return
	}

	if currentClient.Conn == conn {
		delete(m.clients, playerID)
	}
}
```

### 修改后

```go
func (m *Manager) Unregister(playerID int64, connectionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	currentClient, exists := m.clients[playerID]
	if !exists {
		return
	}

	if currentClient.ConnectionID == connectionID {
		delete(m.clients, playerID)
	}
}
```

### 为什么这样改

假设：

```text
旧连接 connection_id=conn_old
新连接 connection_id=conn_new
```

新连接注册后，管理器里保存的是：

```text
player_id=1 -> conn_new
```

旧连接退出时调用：

```go
Unregister(1, "conn_old")
```

管理器会发现当前保存的是：

```text
conn_new
```

所以不会删除。

这就是今天最重要的保护。

## 任务 6：新增 UpdateLastPong 方法

### 修改文件

```text
E:\game-realtime-gm\backend\internal\ws\manager.go
```

操作类型：

```text
在 Unregister 后新增方法。
```

### 插入位置

搜索：

```go
func (m *Manager) Count() int {
```

在它前面插入：

```go
func (m *Manager) UpdateLastPong(playerID int64, connectionID string, lastPongAt time.Time) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	currentClient, exists := m.clients[playerID]
	if !exists {
		return false
	}

	if currentClient.ConnectionID != connectionID {
		return false
	}

	currentClient.LastPongAt = lastPongAt
	return true
}
```

### 代码分析

```go
playerID int64
```

表示要更新哪个玩家的连接。

```go
connectionID string
```

表示只更新指定这一次连接。

如果旧连接的 pong 回调晚到，也不会更新新连接。

```go
lastPongAt time.Time
```

表示最近收到客户端 pong 的时间。

返回值 `bool` 表示是否真的更新成功。

今天可以不使用这个返回值，但保留它有利于后续调试。

## 任务 7：修改 WelcomeData

### 修改文件

```text
E:\game-realtime-gm\backend\internal\ws\message.go
```

操作类型：

```text
局部修改。
```

### 搜索关键词

```go
type WelcomeData struct {
```

### 修改前

```go
type WelcomeData struct {
	PlayerID      int64  `json:"player_id"`
	Username      string `json:"username"`
	OnlinePlayers int    `json:"online_players"`
	OnlineTTL     int    `json:"online_ttl_seconds"`
}
```

### 修改后

```go
type WelcomeData struct {
	ConnectionID  string    `json:"connection_id"`
	PlayerID      int64     `json:"player_id"`
	Username      string    `json:"username"`
	ConnectedAt   time.Time `json:"connected_at"`
	LastPongAt    time.Time `json:"last_pong_at"`
	OnlinePlayers int       `json:"online_players"`
	OnlineTTL     int       `json:"online_ttl_seconds"`
}
```

### 为什么 welcome 要返回 connection_id

连接成功后，客户端应该知道：

```text
当前这条连接在服务端对应哪个 connection_id。
```

后续如果做断线重连、连接调试、小队状态排查，这个字段会很有用。

## 任务 8：在 handler/ws.go 生成 connection_id

### 修改文件

```text
E:\game-realtime-gm\backend\internal\handler\ws.go
```

操作类型：

```text
局部修改。
```

### 搜索关键词

```go
writeMu := &sync.Mutex{}
```

### 修改前

```go
writeMu := &sync.Mutex{}
conn.SetReadLimit(webSocketMaxMessageBytes)
_ = conn.SetReadDeadline(time.Now().Add(webSocketPongWait))
conn.SetPongHandler(func(appData string) error {
	log.Printf("websocket pong received: player_id=%d username=%s data=%s", claims.PlayerID, claims.Username, appData)
	return conn.SetReadDeadline(time.Now().Add(webSocketPongWait))
})
```

### 修改后

```go
writeMu := &sync.Mutex{}
connectedAt := time.Now()
connectionID, err := realtimews.NewConnectionID(claims.PlayerID, connectedAt)
if err != nil {
	log.Printf("websocket generate connection id failed: player_id=%d err=%v", claims.PlayerID, err)
	_ = writeWebSocketJSON(conn, writeMu, realtimews.NewErrorMessage("", 50025, "generate websocket connection id failed"))
	return
}

conn.SetReadLimit(webSocketMaxMessageBytes)
_ = conn.SetReadDeadline(time.Now().Add(webSocketPongWait))
conn.SetPongHandler(func(appData string) error {
	lastPongAt := time.Now()
	wsManager.UpdateLastPong(claims.PlayerID, connectionID, lastPongAt)
	log.Printf("websocket pong received: player_id=%d username=%s connection_id=%s data=%s", claims.PlayerID, claims.Username, connectionID, appData)
	return conn.SetReadDeadline(time.Now().Add(webSocketPongWait))
})
```

### 代码分析

`connectedAt` 只生成一次，表示这条连接建立的时间。

`connectionID` 只生成一次，后续整个连接生命周期都使用它。

`SetPongHandler` 里更新 `last_pong_at`，表示客户端最近一次回应了服务端 ping。

错误码：

```text
50025 generate websocket connection id failed
```

表示服务端生成连接 ID 失败。这个错误很少见，但要有明确返回。

## 任务 9：修改 client 初始化

### 修改文件

```text
E:\game-realtime-gm\backend\internal\handler\ws.go
```

操作类型：

```text
局部修改。
```

### 搜索关键词

```go
client := &realtimews.Client{
```

### 修改前

```go
client := &realtimews.Client{
	PlayerID:    claims.PlayerID,
	Username:    claims.Username,
	Conn:        conn,
	ConnectedAt: time.Now(),
}
```

### 修改后

```go
client := &realtimews.Client{
	ConnectionID: connectionID,
	PlayerID:     claims.PlayerID,
	Username:     claims.Username,
	Conn:         conn,
	ConnectedAt:  connectedAt,
	LastPongAt:   connectedAt,
}
```

### 为什么 LastPongAt 初始值用 connectedAt

刚连接成功时，还没有收到 pong。

但为了避免 `last_pong_at` 是零值时间，可以先设为连接建立时间。

等客户端第一次返回 pong 后，它会被更新成真正的 pong 时间。

## 任务 10：修改 Unregister 调用

### 修改文件

```text
E:\game-realtime-gm\backend\internal\handler\ws.go
```

操作类型：

```text
局部修改。
```

### 搜索关键词

```go
defer wsManager.Unregister
```

### 修改前

```go
defer wsManager.Unregister(claims.PlayerID, conn)
```

### 修改后

```go
defer wsManager.Unregister(claims.PlayerID, connectionID)
```

### 为什么这样改

`handler/ws.go` 不再用连接对象本身判断是否应该注销。

现在使用更明确的：

```text
player_id + connection_id
```

判断。

## 任务 11：让 Redis 在线状态保存 connection_id

### 修改文件

```text
E:\game-realtime-gm\backend\internal\handler\ws.go
```

操作类型：

```text
修改 refreshWebSocketOnlineStatus 和 keepWebSocketOnlineStatus。
```

### 第一步：修改调用位置

搜索：

```go
refreshWebSocketOnlineStatus(onlineCtx, redisClient, claims.PlayerID)
```

### 修改前

```go
if err := refreshWebSocketOnlineStatus(onlineCtx, redisClient, claims.PlayerID); err != nil {
```

### 修改后

```go
if err := refreshWebSocketOnlineStatus(onlineCtx, redisClient, claims.PlayerID, connectionID); err != nil {
```

再搜索：

```go
go keepWebSocketOnlineStatus(onlineCtx, redisClient, claims.PlayerID)
```

### 修改前

```go
go keepWebSocketOnlineStatus(onlineCtx, redisClient, claims.PlayerID)
```

### 修改后

```go
go keepWebSocketOnlineStatus(onlineCtx, redisClient, claims.PlayerID, connectionID)
```

### 第二步：修改函数定义

搜索：

```go
func refreshWebSocketOnlineStatus
```

### 修改前

```go
func refreshWebSocketOnlineStatus(ctx context.Context, redisClient *redis.Client, playerID int64) error {
	key := onlinePlayerKey(playerID)
	return redisClient.Set(ctx, key, "1", onlineTTL).Err()
}
```

### 修改后

```go
func refreshWebSocketOnlineStatus(ctx context.Context, redisClient *redis.Client, playerID int64, connectionID string) error {
	key := onlinePlayerKey(playerID)
	return redisClient.Set(ctx, key, connectionID, onlineTTL).Err()
}
```

再搜索：

```go
func keepWebSocketOnlineStatus
```

### 修改前

```go
func keepWebSocketOnlineStatus(ctx context.Context, redisClient *redis.Client, playerID int64) {
```

### 修改后

```go
func keepWebSocketOnlineStatus(ctx context.Context, redisClient *redis.Client, playerID int64, connectionID string) {
```

函数内部搜索：

```go
refreshWebSocketOnlineStatus(ctx, redisClient, playerID)
```

改成：

```go
refreshWebSocketOnlineStatus(ctx, redisClient, playerID, connectionID)
```

### 为什么 Redis value 要保存 connection_id

Day22 里 Redis key 是：

```text
online:player:<player_id>
```

以前 value 是：

```text
1
```

它只能表示“在线”，不能表示“是哪条连接写入的在线状态”。

Day25 改成 connection_id 后，Redis 变成：

```text
key   = online:player:1
value = conn_1_xxx
ttl   = 120
```

以后断线时就可以做安全判断：

```text
如果 Redis value 仍然等于当前 connection_id，才删除。
如果 Redis value 已经是新 connection_id，就不能删除。
```

今天仍然不主动删除，只为后续能力打基础。

## 任务 12：修改日志，让连接问题更好排查

### 修改文件

```text
E:\game-realtime-gm\backend\internal\handler\ws.go
```

操作类型：

```text
局部修改。
```

### 搜索关键词

```go
websocket connected:
```

### 修改前

```go
log.Printf("websocket connected: player_id=%d username=%s remote=%s online_players=%d", claims.PlayerID, claims.Username, remoteAddr, wsManager.Count())
defer log.Printf("websocket disconnected: player_id=%d username=%s remote=%s", claims.PlayerID, claims.Username, remoteAddr)
```

### 修改后

```go
log.Printf("websocket connected: player_id=%d username=%s connection_id=%s remote=%s online_players=%d", claims.PlayerID, claims.Username, connectionID, remoteAddr, wsManager.Count())
defer log.Printf("websocket disconnected: player_id=%d username=%s connection_id=%s remote=%s", claims.PlayerID, claims.Username, connectionID, remoteAddr)
```

### 为什么要加 connection_id

以后你同时开两个 Apifox WebSocket 连接时，日志能看出：

```text
哪条连接建立
哪条连接被替换
哪条连接断开
```

这对排查重复连接、断线重连、小队广播都很重要。

## 任务 13：修改 welcome 数据

### 修改文件

```text
E:\game-realtime-gm\backend\internal\handler\ws.go
```

操作类型：

```text
局部修改。
```

### 搜索关键词

```go
realtimews.WelcomeData{
```

### 修改前

```go
realtimews.WelcomeData{
	PlayerID:      claims.PlayerID,
	Username:      claims.Username,
	OnlinePlayers: wsManager.Count(),
	OnlineTTL:     int(onlineTTL.Seconds()),
},
```

### 修改后

```go
realtimews.WelcomeData{
	ConnectionID:  connectionID,
	PlayerID:      claims.PlayerID,
	Username:      claims.Username,
	ConnectedAt:   connectedAt,
	LastPongAt:    connectedAt,
	OnlinePlayers: wsManager.Count(),
	OnlineTTL:     int(onlineTTL.Seconds()),
},
```

### 验证重点

连接成功后，你应该能在 welcome 里看到：

```text
connection_id
connected_at
last_pong_at
```

## 任务 14：执行 gofmt 和测试

### 执行命令

```powershell
cd E:\game-realtime-gm\backend
gofmt -w .\internal\ws\session.go .\internal\ws\manager.go .\internal\ws\message.go .\internal\handler\ws.go
go test ./...
```

### 命令解释

`gofmt`：

```text
格式化 Go 文件，自动整理缩进和 import。
```

`go test ./...`：

```text
编译并测试 backend 下所有 Go 包。
```

### 成功结果

你应该看到所有包都没有 `FAIL`。

如果看到：

```text
undefined: connectionID
```

说明你在生成 `connectionID` 之前就使用了它。

优先检查：

```text
connectionID, err := realtimews.NewConnectionID(...)
```

是不是放在 `SetPongHandler` 前面。

如果看到：

```text
too many arguments in call to refreshWebSocketOnlineStatus
```

说明函数定义和调用没有一起改。

所有调用和函数定义都要统一变成：

```go
refreshWebSocketOnlineStatus(ctx, redisClient, playerID, connectionID)
```

## 任务 15：启动 Docker 依赖

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

### 为什么今天需要 Redis

Day25 会把 WebSocket 在线状态写入 Redis：

```text
online:player:<player_id> = connection_id
```

所以 Redis 必须启动。

## 任务 16：启动后端

### 默认启动

```powershell
cd E:\game-realtime-gm\backend
go run .\cmd\server
```

如果 8080 端口被 Windows 排除范围占用，可以临时使用：

```powershell
cd E:\game-realtime-gm\backend
$env:APP_PORT="18080"
go run .\cmd\server
```

使用 18080 时，后面所有接口地址都要同步改成：

```text
http://localhost:18080
ws://localhost:18080/ws?token=玩家token
```

## 任务 17：准备玩家 token

如果你已经有玩家账号，登录：

```http
POST http://localhost:8080/api/login
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

如果没有玩家账号，先注册：

```http
POST http://localhost:8080/api/register
Content-Type: application/json

{
  "username": "player01",
  "password": "123456",
  "nickname": "玩家01"
}
```

再登录拿 token。

## 任务 18：用 Apifox 验证 welcome

### 连接地址

```text
ws://localhost:8080/ws?token=玩家token
```

### 预期结果

连接成功后，服务端返回：

```json
{
  "type": "server.welcome",
  "code": 0,
  "message": "ok",
  "data": {
    "connection_id": "conn_1_1780000000000000000_a1b2c3d4e5f60708",
    "player_id": 1,
    "username": "player01",
    "connected_at": "2026-06-29T10:00:00+08:00",
    "last_pong_at": "2026-06-29T10:00:00+08:00",
    "online_players": 1,
    "online_ttl_seconds": 120
  },
  "server_time": "2026-06-29T10:00:00+08:00"
}
```

### 验证重点

重点看：

```text
connection_id 是否存在
connection_id 是否以 conn_ 开头
connected_at 是否存在
last_pong_at 是否存在
```

如果没有这些字段，优先检查：

```text
E:\game-realtime-gm\backend\internal\ws\message.go
E:\game-realtime-gm\backend\internal\handler\ws.go
```

## 任务 19：验证 Redis value

### 进入 Redis

```powershell
docker exec -it game_realtime_redis redis-cli
```

### 查看在线 key

```text
KEYS online:*
```

假设看到：

```text
1) "online:player:1"
```

继续查看 value：

```text
GET online:player:1
```

预期结果：

```text
"conn_1_1780000000000000000_a1b2c3d4e5f60708"
```

再查看 TTL：

```text
TTL online:player:1
```

预期结果是一个大于 0 的秒数，例如：

```text
108
```

连接保持时，TTL 应该会被续期。

### 为什么不是 1 了

Day22 时 value 是：

```text
1
```

Day25 后 value 是：

```text
connection_id
```

判断在线状态仍然只看 key 是否存在，所以不会影响：

```text
GET /api/online/status
GET /api/admin/dashboard/summary
```

## 任务 20：验证重复连接替换

### 操作步骤

1. 打开 Apifox 第一个 WebSocket 连接，使用玩家 token 连接 `/ws`。
2. 记录第一个 welcome 里的 `connection_id`，例如：

```text
conn_old
```

3. 不关闭第一个连接，再打开第二个 WebSocket 连接，使用同一个玩家 token 连接 `/ws`。
4. 记录第二个 welcome 里的 `connection_id`，例如：

```text
conn_new
```

### 预期结果

后端日志应该能看到类似：

```text
websocket replaced old connection: player_id=1 username=player01
websocket connected: player_id=1 username=player01 connection_id=conn_new ...
```

Redis 中：

```text
GET online:player:1
```

应该更接近第二个连接的：

```text
conn_new
```

### 验证重点

你要确认：

```text
两个 connection_id 不一样。
新连接能正常收发 debug.echo。
旧连接断开时，不会把新连接从 wsManager 里删除。
```

## 任务 21：验证 Day24 消息协议没有被破坏

### 发送消息

```json
{
  "type": "debug.echo",
  "request_id": "req-day25-001",
  "data": {
    "text": "hello day25"
  }
}
```

### 预期响应

```json
{
  "type": "debug.echo.result",
  "request_id": "req-day25-001",
  "code": 0,
  "message": "ok",
  "data": {
    "received_type": "debug.echo",
    "received_data": {
      "text": "hello day25"
    }
  },
  "server_time": "2026-06-29T10:00:00+08:00"
}
```

这一步证明：

```text
Day25 只增强连接会话，不破坏 Day24 统一消息协议。
```

## 任务 22：更新接口文档

### 修改文件

```text
E:\game-realtime-gm\docs\api-overview.md
```

操作类型：

```text
局部修改。
```

### 搜索关键词

```markdown
连接成功后，服务端会主动发送统一格式的欢迎消息：
```

把 welcome 示例中的 `data` 更新为包含：

```json
{
  "connection_id": "conn_1_1780000000000000000_a1b2c3d4e5f60708",
  "player_id": 1,
  "username": "player01",
  "connected_at": "2026-06-29T10:00:00+08:00",
  "last_pong_at": "2026-06-29T10:00:00+08:00",
  "online_players": 1,
  "online_ttl_seconds": 120
}
```

### 建议补充说明

在 `/ws` 说明中补充：

```text
Day 25 起，服务端会为每一次 WebSocket 连接生成 connection_id。
connection_id 用于区分同一个玩家的新旧连接。
Redis 在线状态 online:player:<player_id> 的 value 会保存当前连接的 connection_id。
当前仍然不主动删除 Redis 在线 key，连接断开后等待 TTL 自动过期。
```

### 错误码补充

在 WebSocket 错误表里补充：

| code | message | 场景 |
| --- | --- | --- |
| `50025` | `generate websocket connection id failed` | 服务端生成连接 ID 失败 |

## 任务 23：今日复盘

完成后用自己的话回答：

1. `player_id` 和 `connection_id` 有什么区别？
2. 为什么同一个玩家重复连接时，需要区分旧连接和新连接？
3. 为什么 Redis 在线 key 的 value 改成 connection_id 更好？
4. 为什么今天仍然不主动删除 Redis 在线 key？
5. `last_pong_at` 表示什么？
6. `UpdateLastPong` 为什么要同时检查 player_id 和 connection_id？
7. Day25 和 Day26 小队房间有什么关系？

参考表达：

```text
Day25 我给每次 WebSocket 连接增加了 connection_id，并把它记录到连接管理器、welcome 消息和 Redis 在线状态中。这样同一个玩家重复连接时，服务端能区分旧连接和新连接，旧连接退出时不会误删新连接状态。这个能力会为后续小队房间、状态广播和断线重连打基础。
```

## 今日验收清单

- [ ] 已新增 `backend/internal/ws/session.go`。
- [ ] `session.go` 中有 `NewConnectionID`。
- [ ] `Client` 结构体已新增 `ConnectionID`。
- [ ] `Client` 结构体已新增 `LastPongAt`。
- [ ] `Register` 使用 `ConnectionID` 判断旧连接。
- [ ] `Unregister` 参数已从 `conn` 改成 `connectionID`。
- [ ] `Manager` 已新增 `UpdateLastPong`。
- [ ] `WelcomeData` 已新增 `connection_id`。
- [ ] `WelcomeData` 已新增 `connected_at`。
- [ ] `WelcomeData` 已新增 `last_pong_at`。
- [ ] `handler/ws.go` 建连时生成 `connectionID`。
- [ ] pong 回调会更新 `LastPongAt`。
- [ ] Redis 在线状态 value 保存 `connection_id`。
- [ ] Apifox 连接 `/ws` 能看到 `connection_id`。
- [ ] 重复连接同一玩家时，新旧 `connection_id` 不同。
- [ ] `debug.echo` 仍然能正常返回。
- [ ] `gofmt` 执行成功。
- [ ] `go test ./...` 通过。
- [ ] `docs/api-overview.md` 已更新。

## 常见问题

### 1. go test 报 undefined: realtimews.NewConnectionID

检查是否已经新建：

```text
E:\game-realtime-gm\backend\internal\ws\session.go
```

并且文件开头是：

```go
package ws
```

不要写成：

```go
package handler
```

### 2. go test 报 cannot use conn as string

说明你已经把 `Unregister` 的定义改成：

```go
Unregister(playerID int64, connectionID string)
```

但 `handler/ws.go` 里仍然调用：

```go
wsManager.Unregister(claims.PlayerID, conn)
```

要改成：

```go
wsManager.Unregister(claims.PlayerID, connectionID)
```

### 3. Redis GET online:player:1 还是 1

说明 `refreshWebSocketOnlineStatus` 里还在写：

```go
redisClient.Set(ctx, key, "1", onlineTTL)
```

要改成：

```go
redisClient.Set(ctx, key, connectionID, onlineTTL)
```

### 4. welcome 里没有 connection_id

优先检查两个地方：

```text
E:\game-realtime-gm\backend\internal\ws\message.go
E:\game-realtime-gm\backend\internal\handler\ws.go
```

`WelcomeData` 结构体要有 `ConnectionID` 字段。

创建 welcome 时也要传入：

```go
ConnectionID: connectionID,
```

### 5. 两次连接的 connection_id 一样

正常情况下不应该一样。

检查 `NewConnectionID` 是否使用了：

```go
connectedAt.UnixNano()
rand.Read(randomBytes)
```

如果你手动把 connection_id 写死了，就会导致重复。

### 6. 旧连接断开后新连接不能用了

优先检查 `Unregister` 是否按 `connectionID` 判断。

正确逻辑是：

```go
if currentClient.ConnectionID == connectionID {
	delete(m.clients, playerID)
}
```

不能无条件：

```go
delete(m.clients, playerID)
```

## 今日不要做什么

今天不要做：

- 小队创建。
- 小队加入退出。
- 小队状态广播。
- 匹配队列。
- 任务副本生命周期。
- 主动删除 Redis 在线 key。
- 断线重连恢复业务状态。
- UDP / KCP / QUIC。
- 前端页面。

原因：

```text
今天只补连接会话身份。
先把单连接生命周期打牢，再进入多人小队业务。
```

## 任务 24：提交并推送到 GitHub

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

### 选择性添加 Day25 相关文件

如果今天已经完成代码和文档，执行：

```powershell
git add backend/internal/ws/session.go backend/internal/ws/manager.go backend/internal/ws/message.go backend/internal/handler/ws.go docs/api-overview.md docs/day/day25-plan.md
```

不要使用：

```powershell
git add .
```

### 提交

```powershell
git commit -m "Complete day25 websocket connection id"
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

Day26 建议进入真正的共斗 PVE 业务：

```text
小队房间基础
```

目标：

```text
创建小队
加入小队
离开小队
队长
成员列表
准备状态
```

Day26 会开始把 WebSocket 从“连接能力”推进到“多人业务能力”。
