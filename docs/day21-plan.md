# Day 21 学习计划：WebSocket 连接与 player_id 绑定

## 你今天从任务几开始

今天从任务 1 开始。

Day 20 已经完成：

```text
ws://localhost:8080/ws?token=玩家token
```

并且后端已经能做到：

```text
读取 token
解析 JWT
确认是玩家 token
连接成功后返回 player_id 和 username
```

Day 21 要继续往实时服务核心走一步：

```text
玩家连接成功后，把 player_id 和 WebSocket 连接关系保存到内存管理器。
玩家断开时，从管理器删除。
```

这一步是后续房间、广播、匹配成功通知的基础。

## 今日目标

今天要实现一个最小连接管理器：

```text
internal/ws/manager.go
```

它负责维护：

```text
player_id -> WebSocket client
```

今天最终形成的连接流程：

```text
玩家登录拿到 token
        ↓
连接 /ws?token=玩家token
        ↓
后端校验 token
        ↓
升级 WebSocket
        ↓
创建 ws.Client
        ↓
注册到 ws.Manager
        ↓
返回 welcome，里面带当前在线连接数
        ↓
连接断开
        ↓
从 ws.Manager 注销
```

## 今日最终效果

### 玩家 1 连接

连接：

```text
ws://localhost:8080/ws?token=player01的token
```

收到：

```json
{
  "type": "welcome",
  "content": "connected to game realtime server",
  "server_time": "2026-06-23T10:00:00+08:00",
  "player_id": 1,
  "username": "player01",
  "online_players": 1
}
```

### 玩家 2 连接

再连接：

```text
ws://localhost:8080/ws?token=player02的token
```

收到：

```json
{
  "type": "welcome",
  "content": "connected to game realtime server",
  "server_time": "2026-06-23T10:01:00+08:00",
  "player_id": 2,
  "username": "player02",
  "online_players": 2
}
```

### 同一个玩家重复连接

如果 `player01` 又打开一个新的 WebSocket 连接：

```text
旧连接会被替换。
管理器里仍然只保留 player01 的最新连接。
```

今天暂时不做多端同时在线。

## 今日会学到什么

你今天会学到：

- 为什么实时服务需要连接管理器。
- Go 里为什么多个连接同时操作 `map` 时必须加锁。
- `sync.RWMutex` 的基本作用。
- 为什么连接管理逻辑应该从 `handler/ws.go` 拆出去。
- 为什么今天使用内存管理器，而不是直接上 Redis。
- 同一个玩家重复连接时为什么要处理旧连接。
- 断开连接时为什么必须注销。

一句话理解：

```text
Day 20 知道“谁连上来了”，Day 21 开始记住“这个玩家当前连着哪条连接”。
```

## 今日文件范围

今天主要涉及：

```text
E:\game-realtime-gm\backend\internal\ws\manager.go
E:\game-realtime-gm\backend\internal\handler\ws.go
E:\game-realtime-gm\backend\internal\router\router.go
E:\game-realtime-gm\docs\api-overview.md
E:\game-realtime-gm\docs\day21-plan.md
```

文件职责说明：

| 文件 | 作用 |
| --- | --- |
| `internal/ws/manager.go` | 新增 WebSocket 连接管理器，维护玩家和连接关系 |
| `handler/ws.go` | WebSocket 握手、鉴权、注册连接、收发消息 |
| `router/router.go` | 创建一个全局连接管理器，并传给 `/ws` handler |
| `docs/api-overview.md` | 更新 `/ws` welcome 响应，补充 `online_players` |
| `docs/day21-plan.md` | 今天这份学习教程 |

今天不新增数据库表。

今天不写 Redis。

原因：

```text
今天先理解单进程内存连接管理。
Day 22 再把在线状态续期到 Redis。
```

## 任务 0：确认 Day 20 状态

### 你要做什么

先确认 Day 20 的 WebSocket token 鉴权代码能编译。

### 执行命令

```powershell
cd E:\game-realtime-gm\backend
go test ./...
```

成功时你应该看到：

```text
?       game-realtime-gm/backend/internal/handler [no test files]
?       game-realtime-gm/backend/internal/router  [no test files]
```

我当前检查到的状态是：

```text
go test ./... 已通过。
```

你自己动手做 Day 21 前，仍建议再跑一次。

## 任务 1：理解为什么要新增 internal/ws

### 你要做什么

先理解今天为什么不继续把所有代码都塞进：

```text
E:\game-realtime-gm\backend\internal\handler\ws.go
```

### 为什么要拆出 internal/ws

`handler` 的职责是：

```text
处理请求入口
解析参数
返回响应
```

但连接管理器的职责是：

```text
保存当前有哪些玩家在线
保存 player_id 对应哪个 WebSocket 连接
玩家断开时删除连接
未来给指定玩家推送消息
未来给房间内多个玩家广播消息
```

这些不是普通 HTTP handler 的职责。

所以今天新增：

```text
E:\game-realtime-gm\backend\internal\ws\
```

这个包以后可以继续扩展：

```text
internal/ws/manager.go     连接管理
internal/ws/client.go      单个连接读写封装
internal/ws/message.go     消息格式
internal/ws/hub.go         广播中心
```

今天只新增一个：

```text
manager.go
```

## 任务 2：新建连接管理器 manager.go

### 修改文件

```text
E:\game-realtime-gm\backend\internal\ws\manager.go
```

操作类型：

```text
新建文件。
如果 internal\ws 目录不存在，先创建目录。
```

### 创建目录

在 PowerShell 执行：

```powershell
cd E:\game-realtime-gm\backend
New-Item -ItemType Directory -Force .\internal\ws
```

### 完整代码

把下面内容写入：

```text
E:\game-realtime-gm\backend\internal\ws\manager.go
```

```go
package ws

import (
	"sort"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Client struct {
	PlayerID    int64
	Username    string
	Conn        *websocket.Conn
	ConnectedAt time.Time
}

type Manager struct {
	mu      sync.RWMutex
	clients map[int64]*Client
}

func NewManager() *Manager {
	return &Manager{
		clients: make(map[int64]*Client),
	}
}

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

func (m *Manager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return len(m.clients)
}

func (m *Manager) PlayerIDs() []int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	playerIDs := make([]int64, 0, len(m.clients))
	for playerID := range m.clients {
		playerIDs = append(playerIDs, playerID)
	}

	sort.Slice(playerIDs, func(i, j int) bool {
		return playerIDs[i] < playerIDs[j]
	})

	return playerIDs
}
```

## 任务 3：理解 manager.go 代码

### package

```go
package ws
```

表示这个文件属于 `ws` 包。

它的位置是：

```text
backend/internal/ws
```

所以其他包引用它时会写：

```go
import "game-realtime-gm/backend/internal/ws"
```

### Client

```go
type Client struct {
	PlayerID    int64
	Username    string
	Conn        *websocket.Conn
	ConnectedAt time.Time
}
```

`Client` 表示一个已经连上的玩家连接。

字段说明：

| 字段 | 作用 |
| --- | --- |
| `PlayerID` | 当前连接属于哪个玩家 |
| `Username` | 玩家用户名，方便日志和调试 |
| `Conn` | 真正的 WebSocket 连接对象 |
| `ConnectedAt` | 连接建立时间 |

注意：

```text
这里不保存 token。
```

原因：

```text
token 是敏感信息。
连接通过鉴权后，只需要保存 player_id 和 username。
```

### Manager

```go
type Manager struct {
	mu      sync.RWMutex
	clients map[int64]*Client
}
```

`Manager` 是连接管理器。

它内部维护：

```text
map[player_id]client
```

也就是：

```text
某个玩家 ID 当前对应哪个 WebSocket 连接。
```

### 为什么要 sync.RWMutex

WebSocket 服务里，多个玩家会同时连接和断开。

这些操作可能同时发生：

```text
玩家 A 注册连接
玩家 B 注册连接
玩家 A 断开
服务端统计在线人数
```

如果多个 goroutine 同时读写同一个 map，又没有加锁，Go 可能报：

```text
fatal error: concurrent map writes
```

所以这里用：

```go
sync.RWMutex
```

它的作用：

| 方法 | 作用 |
| --- | --- |
| `Lock` | 写锁，用于新增、删除、替换连接 |
| `Unlock` | 释放写锁 |
| `RLock` | 读锁，用于只读取在线人数或玩家列表 |
| `RUnlock` | 释放读锁 |

### Register

```go
func (m *Manager) Register(client *Client) *websocket.Conn
```

作用：

```text
把玩家连接注册进管理器。
```

如果同一个玩家已经有旧连接，它会返回旧连接。

返回旧连接的原因：

```text
handler 可以关闭旧连接，只保留最新连接。
```

今天的规则是：

```text
同一个玩家只保留一个最新 WebSocket 连接。
```

### Unregister

```go
func (m *Manager) Unregister(playerID int64, conn *websocket.Conn)
```

作用：

```text
连接断开时，从管理器删除这个玩家的连接。
```

为什么要传 `conn`：

```text
防止旧连接断开时，把新连接误删。
```

例如：

```text
player01 旧连接还没完全退出
player01 新连接已经注册成功
旧连接 defer 执行 Unregister
```

如果只按 `playerID` 删除，就会误删新连接。

所以这里判断：

```go
if currentClient.Conn == conn {
	delete(m.clients, playerID)
}
```

只有当前管理器里保存的连接就是这条连接时，才删除。

### Count

```go
func (m *Manager) Count() int
```

作用：

```text
返回当前在线玩家连接数量。
```

今天会把它放进 welcome 消息里，方便你测试。

### PlayerIDs

```go
func (m *Manager) PlayerIDs() []int64
```

作用：

```text
返回当前在线玩家 ID 列表。
```

今天主要用于后续扩展，不一定马上用。

为什么排序：

```text
调试时输出稳定，方便看。
```

## 任务 4：整文件替换 handler/ws.go

### 修改文件

```text
E:\game-realtime-gm\backend\internal\handler\ws.go
```

操作类型：

```text
整文件替换。
```

### 为什么整文件替换

今天要同时改：

- import
- `WSMessage` 字段
- `WebSocketEcho` 参数
- 注册连接
- 注销连接
- 同玩家旧连接替换
- welcome 中返回在线人数

文件仍然不算太长，整文件替换更稳。

### 完整代码

把下面完整内容替换到：

```text
E:\game-realtime-gm\backend\internal\handler\ws.go
```

```go
package handler

import (
	"log"
	"net/http"
	"strings"
	"time"

	tokenauth "game-realtime-gm/backend/internal/auth"
	realtimews "game-realtime-gm/backend/internal/ws"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type WSMessage struct {
	Type          string    `json:"type"`
	Content       string    `json:"content,omitempty"`
	ServerTime    time.Time `json:"server_time"`
	PlayerID      int64     `json:"player_id,omitempty"`
	Username      string    `json:"username,omitempty"`
	OnlinePlayers int       `json:"online_players,omitempty"`
}

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func WebSocketEcho(jwtSecret string, wsManager *realtimews.Manager) gin.HandlerFunc {
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
```

## 任务 5：理解 handler/ws.go 改动

### 新增 import

```go
realtimews "game-realtime-gm/backend/internal/ws"
```

这里给 `internal/ws` 包起了别名：

```text
realtimews
```

原因：

```text
当前文件里已经使用 gorilla/websocket。
如果再直接叫 ws，初学者容易看混。
```

### WSMessage 新增字段

```go
OnlinePlayers int `json:"online_players,omitempty"`
```

作用：

```text
连接成功时告诉客户端当前管理器里有多少个在线玩家连接。
```

这是 Day 21 的可见验证点。

### WebSocketEcho 参数变化

Day 20：

```go
func WebSocketEcho(jwtSecret string) gin.HandlerFunc
```

Day 21：

```go
func WebSocketEcho(jwtSecret string, wsManager *realtimews.Manager) gin.HandlerFunc
```

新增的：

```go
wsManager *realtimews.Manager
```

就是连接管理器。

### 创建 Client

```go
client := &realtimews.Client{
	PlayerID:    claims.PlayerID,
	Username:    claims.Username,
	Conn:        conn,
	ConnectedAt: time.Now(),
}
```

这一步把 JWT 里的玩家身份和真实 WebSocket 连接绑定起来。

### 注册连接

```go
oldConn := wsManager.Register(client)
```

作用：

```text
把 player_id -> 当前连接 保存到管理器。
```

如果同一个玩家之前已经连过，`Register` 会返回旧连接。

### 替换旧连接

```go
if oldConn != nil {
	_ = oldConn.Close()
}
```

作用：

```text
同一个玩家新连接进来时，关闭旧连接。
```

今天的规则是：

```text
一个玩家只保留一个最新连接。
```

### 注销连接

```go
defer wsManager.Unregister(claims.PlayerID, conn)
```

作用：

```text
当前 handler 结束时，把这条连接从管理器里移除。
```

`defer` 的好处是：

```text
无论是客户端主动断开、读取失败、写入失败，函数结束前都会执行注销。
```

## 任务 6：修改 router.go

### 修改文件

```text
E:\game-realtime-gm\backend\internal\router\router.go
```

操作类型：

```text
局部修改。
```

### 第一步：新增 import

搜索：

```go
"game-realtime-gm/backend/internal/middleware"
```

修改前：

```go
"game-realtime-gm/backend/internal/handler"
"game-realtime-gm/backend/internal/middleware"
```

修改后：

```go
"game-realtime-gm/backend/internal/handler"
"game-realtime-gm/backend/internal/middleware"
"game-realtime-gm/backend/internal/ws"
```

### 第二步：创建 wsManager

搜索：

```go
onlineHandler := handler.NewOnlineHandler(redisClient)
```

修改前：

```go
onlineHandler := handler.NewOnlineHandler(redisClient)
```

修改后：

```go
onlineHandler := handler.NewOnlineHandler(redisClient)
wsManager := ws.NewManager()
```

### 第三步：修改 /ws 路由

搜索：

```go
r.GET("/ws", handler.WebSocketEcho(cfg.JWTSecret))
```

修改前：

```go
r.GET("/ws", handler.WebSocketEcho(cfg.JWTSecret))
```

修改后：

```go
r.GET("/ws", handler.WebSocketEcho(cfg.JWTSecret, wsManager))
```

### 为什么 wsManager 在 router.New 里创建

`router.New` 在服务启动时只执行一次。

所以：

```go
wsManager := ws.NewManager()
```

创建出来的管理器会跟随整个服务生命周期存在。

所有 `/ws` 连接都会共享同一个管理器。

不要把 `ws.NewManager()` 写进 `WebSocketEcho` 里面。

错误思路：

```go
func WebSocketEcho(...) gin.HandlerFunc {
	wsManager := ws.NewManager()
	...
}
```

这样每个请求都会创建一个新管理器，连接之间就互相看不见了。

## 任务 7：格式化和编译验证

### 执行命令

```powershell
cd E:\game-realtime-gm\backend
gofmt -w .\internal\ws\manager.go .\internal\handler\ws.go .\internal\router\router.go
go test ./...
```

### 成功结果

应该看到类似：

```text
?       game-realtime-gm/backend/internal/ws      [no test files]
?       game-realtime-gm/backend/internal/handler [no test files]
?       game-realtime-gm/backend/internal/router  [no test files]
```

### 可选并发检查

今天新增了带锁的连接管理器。

如果你想额外检查数据竞争，可以执行：

```powershell
go test -race ./...
```

这条命令可能比普通 `go test` 慢。

如果通过，说明目前测试覆盖到的路径没有发现数据竞争。

## 任务 8：启动依赖和后端

### 启动 Docker Desktop 依赖

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

### 启动后端

```powershell
cd E:\game-realtime-gm\backend
go run .\cmd\server
```

后端窗口不要关闭。

## 任务 9：准备两个玩家 token

### 为什么需要两个玩家

Day 21 要验证：

```text
管理器里能同时保存多个玩家连接。
```

所以最好准备两个玩家：

```text
player01
player02
```

### 注册 player01

```http
POST http://localhost:8080/api/register
Content-Type: application/json

{
  "username": "player01",
  "password": "123456",
  "nickname": "玩家01"
}
```

如果已经存在，返回用户名冲突是正常的，直接登录即可。

### 登录 player01

```http
POST http://localhost:8080/api/login
Content-Type: application/json

{
  "username": "player01",
  "password": "123456"
}
```

复制 `data.token`。

### 注册 player02

```http
POST http://localhost:8080/api/register
Content-Type: application/json

{
  "username": "player02",
  "password": "123456",
  "nickname": "玩家02"
}
```

### 登录 player02

```http
POST http://localhost:8080/api/login
Content-Type: application/json

{
  "username": "player02",
  "password": "123456"
}
```

复制 `data.token`。

## 任务 10：用 Apifox 验证连接管理器

### 测试 1：连接 player01

打开一个 WebSocket 请求：

```text
ws://localhost:8080/ws?token=player01的token
```

预期 welcome：

```json
{
  "type": "welcome",
  "content": "connected to game realtime server",
  "player_id": 1,
  "username": "player01",
  "online_players": 1
}
```

`server_time` 会按实际时间变化。

### 测试 2：再连接 player02

不要关闭 player01 的连接。

再打开一个 WebSocket 请求：

```text
ws://localhost:8080/ws?token=player02的token
```

预期 welcome：

```json
{
  "type": "welcome",
  "content": "connected to game realtime server",
  "player_id": 2,
  "username": "player02",
  "online_players": 2
}
```

### 测试 3：同一个玩家重复连接

不要关闭 player02。

再打开一个新的 player01 连接：

```text
ws://localhost:8080/ws?token=player01的token
```

预期：

```text
新的 player01 连接成功。
旧的 player01 连接被关闭或失效。
online_players 仍然大概率是 2。
```

后端日志应出现：

```text
websocket replaced old connection: player_id=1 username=player01
```

### 测试 4：回显仍然可用

在新 player01 连接里发送：

```text
hello day21
```

应该收到：

```text
hello day21
```

## 任务 11：Redis 对照验证

### 今天 Redis 是否会新增在线状态

不会。

Day 21 的连接管理器是：

```text
Go 进程内存 map
```

不是：

```text
Redis
```

### 为什么今天不直接写 Redis

因为今天要先学会：

```text
连接对象在服务端怎么保存
并发访问 map 为什么要加锁
连接断开时怎么清理
```

Redis 在线状态放到 Day 22 更合适。

### 可选检查 Redis

```powershell
docker exec -it game_realtime_redis redis-cli
```

进入后执行：

```text
KEYS online:*
```

Day 21 的 `/ws` 不会因为连接管理器而新增 Redis key。

如果你之前手动调用过 `/api/online/heartbeat`，可能会看到旧的 `online:*` key，这是正常的。

## 任务 12：更新接口文档

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

把 welcome 示例从 Day 20 的：

```json
{
  "type": "welcome",
  "content": "connected to game realtime server",
  "server_time": "2026-06-22T10:00:00+08:00",
  "player_id": 1,
  "username": "player01"
}
```

更新为：

```json
{
  "type": "welcome",
  "content": "connected to game realtime server",
  "server_time": "2026-06-23T10:00:00+08:00",
  "player_id": 1,
  "username": "player01",
  "online_players": 1
}
```

说明里补充：

```text
Day 21 已将玩家连接注册到内存连接管理器。
online_players 表示当前 Go 进程内管理器记录的在线玩家连接数量。
同一个玩家重复连接时，旧连接会被新连接替换。
当前暂不写 Redis，服务重启后内存连接状态会清空。
```

## 任务 13：今日复盘

完成后用自己的话回答：

1. 为什么要有 `internal/ws/manager.go`？
2. `handler/ws.go` 和 `ws/manager.go` 的职责区别是什么？
3. 为什么 `clients map[int64]*Client` 要加锁？
4. `Register` 为什么要返回旧连接？
5. `Unregister` 为什么既传 `playerID` 又传 `conn`？
6. 为什么今天不把连接状态写入 Redis？
7. 服务重启后，内存连接管理器里的状态会怎样？
8. Day 21 和后续房间广播有什么关系？

参考表达：

```text
Day 21 我把 WebSocket 连接管理从 handler 中拆到了 internal/ws。
Manager 用 map 保存 player_id 到 Client 的关系，并用 sync.RWMutex 保证并发安全。
玩家连接成功后，handler 创建 Client 并注册到 Manager；连接结束时通过 defer 注销。
同一个玩家重复连接时，新连接会替换旧连接，避免一个 player_id 对应多条连接导致后续推送混乱。
```

## 今日验收清单

- [ ] 新增 `backend/internal/ws/manager.go`。
- [ ] `manager.go` 里有 `Client`、`Manager`、`Register`、`Unregister`、`Count`。
- [ ] `handler/ws.go` 已引入 `realtimews`。
- [ ] `WebSocketEcho` 已接收 `wsManager`。
- [ ] 连接成功后会注册到 `wsManager`。
- [ ] 连接结束时会执行 `Unregister`。
- [ ] welcome 消息包含 `online_players`。
- [ ] `router.go` 创建了 `wsManager := ws.NewManager()`。
- [ ] `/ws` 路由传入了 `wsManager`。
- [ ] `gofmt` 执行成功。
- [ ] `go test ./...` 通过。
- [ ] player01 连接时 `online_players` 为 1。
- [ ] player02 同时连接时 `online_players` 为 2。
- [ ] 同一个玩家重复连接时旧连接被替换。
- [ ] 日志没有打印完整 token。
- [ ] `docs/api-overview.md` 已更新。

## 常见问题

### 1. go test 报 package internal/ws is not in std

检查 import 是否写成了错误路径。

正确：

```go
realtimews "game-realtime-gm/backend/internal/ws"
```

错误：

```go
import "internal/ws"
```

### 2. go test 报 undefined: ws.NewManager

检查 `manager.go` 是否：

```go
package ws
```

并且有：

```go
func NewManager() *Manager
```

### 3. go test 报 not enough arguments in call to handler.WebSocketEcho

说明 `router.go` 还停留在 Day 20 写法。

修改成：

```go
r.GET("/ws", handler.WebSocketEcho(cfg.JWTSecret, wsManager))
```

### 4. go test 报 imported and not used

说明 import 加了但代码没用到。

执行：

```powershell
gofmt -w .\internal\ws\manager.go .\internal\handler\ws.go .\internal\router\router.go
go test ./...
```

如果还报错，就按报错文件删除未使用 import。

### 5. 运行时报 fatal error: concurrent map writes

说明你可能自己写的 Manager 没加锁，或者直接在 handler 里用了全局 map。

检查 `manager.go` 是否有：

```go
mu sync.RWMutex
```

并且写 map 时使用：

```go
m.mu.Lock()
defer m.mu.Unlock()
```

读 map 时使用：

```go
m.mu.RLock()
defer m.mu.RUnlock()
```

### 6. online_players 数字不符合预期

优先检查：

```text
旧 Apifox WebSocket 连接是否还开着
同一个玩家是不是重复连接导致旧连接被替换
后端是否重启过
不同 token 是否其实属于同一个玩家
```

### 7. 旧连接被关闭是不是错误

不是。

Day 21 规则就是：

```text
同一个 player_id 只保留最新连接。
```

这让后续“给某个玩家推送消息”更简单。

## 今日不要做什么

今天不要做：

- Redis 在线状态续期。
- ping/pong 心跳。
- 房间系统。
- 多人房间广播。
- 私聊消息协议。
- GM 后台在线连接列表接口。
- WebSocket 压测。
- 分布式多进程连接管理。

原因：

```text
今天只解决“服务端如何记住连接”的问题。
```

Day 21 先把内存连接管理学清楚。

Day 22 再把在线状态写入 Redis。

## 任务 14：提交并推送到 GitHub

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

### 选择性添加 Day 21 相关文件

```powershell
git add backend README.md docs/api-overview.md docs/day21-plan.md
```

如果 `README.md` 没有修改，可以不加。

不要使用：

```powershell
git add .
```

### 提交

```powershell
git commit -m "Complete day21 websocket manager"
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

Day 22 建议继续 WebSocket 主线：

```text
WebSocket 连接时写入 Redis 在线状态
```

目标：

```text
玩家 WebSocket 连接成功后，写入 online:player:<player_id>
连接期间续期 TTL
连接断开时删除或等待 TTL 自动过期
```

这会把 Day 21 的“进程内连接管理”和前面已有的 Redis 在线状态模块接起来。
