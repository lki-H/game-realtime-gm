# Day 26 学习计划：小队房间基础

## 你今天从任务几开始

今天从任务 0 开始。

Day 23 到 Day 25 已经把 WebSocket 主线打好了：

```text
玩家 token 鉴权
        ↓
建立 WebSocket 长连接
        ↓
ping/pong 心跳和读写超时
        ↓
统一 JSON 消息协议
        ↓
connection_id 区分同一玩家的新旧连接
```

Day26 开始进入真正的共斗 PVE 业务：

```text
小队房间基础
```

今天只做“小队数据和单人操作返回”，不做小队内广播。广播留到 Day27。

## 今日目标

今天要通过 WebSocket 消息实现：

```text
创建小队
加入小队
离开小队
设置准备状态
查询我的小队
```

客户端发送：

```json
{
  "type": "squad.create",
  "request_id": "req-001",
  "data": {}
}
```

服务端返回：

```json
{
  "type": "squad.create.result",
  "request_id": "req-001",
  "code": 0,
  "message": "ok",
  "data": {
    "squad": {
      "id": "squad_1",
      "leader_id": 1,
      "max_members": 4,
      "members": [
        {
          "player_id": 1,
          "username": "player01",
          "ready": true
        }
      ]
    }
  },
  "server_time": "2026-07-01T10:00:00+08:00"
}
```

一句话理解：

```text
Day26 让 WebSocket 从“能连、能收发”进入“能表达多人 PVE 小队业务”。
```

## 今日最终效果

你可以打开两个 Apifox WebSocket 连接，分别使用两个玩家 token。

玩家 1 发送：

```json
{
  "type": "squad.create",
  "request_id": "req-create-001",
  "data": {}
}
```

得到小队 id，例如：

```text
squad_1
```

玩家 2 发送：

```json
{
  "type": "squad.join",
  "request_id": "req-join-001",
  "data": {
    "squad_id": "squad_1"
  }
}
```

服务端返回的小队成员中包含两个玩家。

玩家 2 再发送：

```json
{
  "type": "squad.ready",
  "request_id": "req-ready-001",
  "data": {
    "ready": true
  }
}
```

服务端返回小队状态，玩家 2 的 `ready` 变成 `true`。

## 今日会学到什么

你今天会学到：

- 小队房间为什么是共斗 PVE 的核心业务对象。
- 为什么 Day26 先用内存保存小队状态。
- `squad.Manager` 为什么要用 `sync.RWMutex`。
- 为什么要维护 `squads` 和 `playerSquad` 两个 map。
- 队长、成员、准备状态分别表示什么。
- WebSocket 的 `type` 怎么从 `debug.echo` 扩展到真实业务。
- 为什么今天不做广播。
- 为什么 Day26 不新增数据库表。

## 今日文件范围

今天主要涉及：

```text
E:\game-realtime-gm\backend\internal\squad\manager.go
E:\game-realtime-gm\backend\internal\ws\message.go
E:\game-realtime-gm\backend\internal\handler\ws.go
E:\game-realtime-gm\backend\internal\router\router.go
E:\game-realtime-gm\docs\api-overview.md
E:\game-realtime-gm\docs\day\day26-plan.md
```

文件职责说明：

| 文件 | 作用 |
| --- | --- |
| `internal/squad/manager.go` | 新增小队房间内存管理器，负责创建、加入、离开、准备状态 |
| `internal/ws/message.go` | 增加小队相关 WebSocket 消息类型和请求/响应数据结构 |
| `internal/handler/ws.go` | 在 WebSocket 消息分发中处理 `squad.*` 消息 |
| `internal/router/router.go` | 创建 `squad.Manager` 并传给 WebSocket handler |
| `docs/api-overview.md` | 更新 `/ws` 文档，补充 Day26 小队消息 |
| `docs/day/day26-plan.md` | 今天这份学习教程 |

今天不新增 PostgreSQL 表。

今天不新增 Redis key。

原因：

```text
Day26 先做单进程内存小队，验证业务模型和消息协议。
等小队流程跑通后，再考虑 Redis 缓存、数据库记录和广播。
```

## 任务 0：确认当前状态

### 执行命令

```powershell
cd E:\game-realtime-gm
git status -sb
```

理想结果：

```text
## main...origin/main
```

再执行：

```powershell
cd E:\game-realtime-gm\backend
go test ./...
```

成功时没有 `FAIL`。

## 任务 1：理解小队房间的业务含义

小队房间不是普通聊天室。

在共斗 PVE 游戏里，小队通常表示：

```text
几个玩家准备一起进入一个任务副本。
```

它至少需要这些信息：

| 信息 | 作用 |
| --- | --- |
| `squad_id` | 小队唯一编号 |
| `leader_id` | 队长玩家 ID |
| `members` | 当前成员列表 |
| `ready` | 每个成员是否准备 |
| `max_members` | 最大人数，例如 4 人 |
| `created_at` | 小队创建时间 |
| `updated_at` | 小队最后更新时间 |

今天先做：

```text
一个玩家只能在一个小队里。
一个小队最多 4 人。
创建者自动成为队长并自动 ready=true。
普通成员加入时 ready=false。
队长离开后转移给最早加入的成员。
最后一个人离开时小队解散。
```

## 任务 2：新增 squad 管理器

### 修改文件

```text
E:\game-realtime-gm\backend\internal\squad\manager.go
```

操作类型：

```text
新建目录和新建文件。
```

如果 `internal\squad` 目录不存在，先创建目录：

```powershell
cd E:\game-realtime-gm\backend
mkdir .\internal\squad
```

### 完整代码

新建文件：

```text
E:\game-realtime-gm\backend\internal\squad\manager.go
```

填入：

```go
package squad

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

const MaxMembers = 4

var (
	ErrPlayerAlreadyInSquad = errors.New("player already in squad")
	ErrPlayerNotInSquad     = errors.New("player not in squad")
	ErrSquadNotFound        = errors.New("squad not found")
	ErrSquadFull            = errors.New("squad is full")
)

type Member struct {
	PlayerID int64     `json:"player_id"`
	Username string    `json:"username"`
	Ready    bool      `json:"ready"`
	JoinedAt time.Time `json:"joined_at"`
}

type Squad struct {
	ID         string    `json:"id"`
	LeaderID   int64     `json:"leader_id"`
	MaxMembers int       `json:"max_members"`
	Members    []Member  `json:"members"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type Manager struct {
	mu          sync.RWMutex
	nextID      int64
	squads      map[string]*Squad
	playerSquad map[int64]string
}

func NewManager() *Manager {
	return &Manager{
		nextID:      1,
		squads:      make(map[string]*Squad),
		playerSquad: make(map[int64]string),
	}
}

func (m *Manager) Create(playerID int64, username string) (*Squad, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.playerSquad[playerID]; exists {
		return nil, ErrPlayerAlreadyInSquad
	}

	now := time.Now()
	squadID := fmt.Sprintf("squad_%d", m.nextID)
	m.nextID++

	s := &Squad{
		ID:         squadID,
		LeaderID:   playerID,
		MaxMembers: MaxMembers,
		Members: []Member{
			{
				PlayerID: playerID,
				Username: username,
				Ready:    true,
				JoinedAt: now,
			},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	m.squads[squadID] = s
	m.playerSquad[playerID] = squadID

	return cloneSquad(s), nil
}

func (m *Manager) Join(squadID string, playerID int64, username string) (*Squad, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.playerSquad[playerID]; exists {
		return nil, ErrPlayerAlreadyInSquad
	}

	s, exists := m.squads[squadID]
	if !exists {
		return nil, ErrSquadNotFound
	}

	if len(s.Members) >= s.MaxMembers {
		return nil, ErrSquadFull
	}

	now := time.Now()
	s.Members = append(s.Members, Member{
		PlayerID: playerID,
		Username: username,
		Ready:    false,
		JoinedAt: now,
	})
	s.UpdatedAt = now
	m.playerSquad[playerID] = squadID

	return cloneSquad(s), nil
}

func (m *Manager) Leave(playerID int64) (*Squad, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	squadID, exists := m.playerSquad[playerID]
	if !exists {
		return nil, false, ErrPlayerNotInSquad
	}

	s, exists := m.squads[squadID]
	if !exists {
		delete(m.playerSquad, playerID)
		return nil, false, ErrSquadNotFound
	}

	nextMembers := make([]Member, 0, len(s.Members)-1)
	for _, member := range s.Members {
		if member.PlayerID != playerID {
			nextMembers = append(nextMembers, member)
		}
	}

	delete(m.playerSquad, playerID)

	if len(nextMembers) == 0 {
		delete(m.squads, squadID)
		return nil, true, nil
	}

	s.Members = nextMembers
	if s.LeaderID == playerID {
		s.LeaderID = nextMembers[0].PlayerID
	}
	s.UpdatedAt = time.Now()

	return cloneSquad(s), false, nil
}

func (m *Manager) SetReady(playerID int64, ready bool) (*Squad, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	squadID, exists := m.playerSquad[playerID]
	if !exists {
		return nil, ErrPlayerNotInSquad
	}

	s, exists := m.squads[squadID]
	if !exists {
		delete(m.playerSquad, playerID)
		return nil, ErrSquadNotFound
	}

	for i := range s.Members {
		if s.Members[i].PlayerID == playerID {
			s.Members[i].Ready = ready
			s.UpdatedAt = time.Now()
			return cloneSquad(s), nil
		}
	}

	delete(m.playerSquad, playerID)
	return nil, ErrPlayerNotInSquad
}

func (m *Manager) GetByPlayer(playerID int64) (*Squad, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	squadID, exists := m.playerSquad[playerID]
	if !exists {
		return nil, ErrPlayerNotInSquad
	}

	s, exists := m.squads[squadID]
	if !exists {
		return nil, ErrSquadNotFound
	}

	return cloneSquad(s), nil
}

func cloneSquad(s *Squad) *Squad {
	if s == nil {
		return nil
	}

	members := make([]Member, len(s.Members))
	copy(members, s.Members)

	return &Squad{
		ID:         s.ID,
		LeaderID:   s.LeaderID,
		MaxMembers: s.MaxMembers,
		Members:    members,
		CreatedAt:  s.CreatedAt,
		UpdatedAt:  s.UpdatedAt,
	}
}
```

### 代码分析

`package squad` 表示这是一个新的业务包，专门管理小队。

`Manager` 里有两个 map：

```go
squads map[string]*Squad
```

表示：

```text
squad_id -> 小队对象
```

```go
playerSquad map[int64]string
```

表示：

```text
player_id -> squad_id
```

为什么需要两个 map：

```text
查询某个小队时，用 squads。
判断玩家是否已经在小队里，用 playerSquad。
```

`sync.RWMutex` 用来保护 map。

原因：

```text
多个 WebSocket 连接可能同时创建、加入、离开小队。
Go 的 map 不能在并发情况下随便读写。
```

`cloneSquad` 的作用：

```text
返回小队副本，不把内部指针直接暴露给 handler。
```

这是一个小但很重要的工程习惯。

## 任务 3：新增 WebSocket 小队消息类型

### 修改文件

```text
E:\game-realtime-gm\backend\internal\ws\message.go
```

操作类型：

```text
局部修改。
```

### 第一步：修改 import

搜索：

```go
import (
	"encoding/json"
	"time"
)
```

改成：

```go
import (
	"encoding/json"
	"time"

	"game-realtime-gm/backend/internal/squad"
)
```

### 第二步：新增消息类型常量

搜索：

```go
MessageTypeDebugEchoResult = "debug.echo.result"
```

在它后面新增：

```go
	MessageTypeSquadCreate       = "squad.create"
	MessageTypeSquadCreateResult = "squad.create.result"
	MessageTypeSquadJoin         = "squad.join"
	MessageTypeSquadJoinResult   = "squad.join.result"
	MessageTypeSquadLeave        = "squad.leave"
	MessageTypeSquadLeaveResult  = "squad.leave.result"
	MessageTypeSquadReady        = "squad.ready"
	MessageTypeSquadReadyResult  = "squad.ready.result"
	MessageTypeSquadMe           = "squad.me"
	MessageTypeSquadMeResult     = "squad.me.result"
```

### 第三步：在 EchoData 后新增数据结构

搜索：

```go
type EchoData struct {
```

在 `EchoData` 结构体结束后新增：

```go
type SquadJoinRequest struct {
	SquadID string `json:"squad_id"`
}

type SquadReadyRequest struct {
	Ready bool `json:"ready"`
}

type SquadData struct {
	Squad *squad.Squad `json:"squad,omitempty"`
}

type SquadLeaveData struct {
	Squad    *squad.Squad `json:"squad,omitempty"`
	Disbanded bool        `json:"disbanded"`
}
```

### 代码分析

`SquadJoinRequest` 表示客户端加入小队时要传：

```json
{
  "squad_id": "squad_1"
}
```

`SquadReadyRequest` 表示客户端设置准备状态时要传：

```json
{
  "ready": true
}
```

`SquadData` 是服务端返回小队状态的统一结构。

`SquadLeaveData` 多了一个：

```text
disbanded
```

用于表示小队是否因为最后一个成员离开而解散。

## 任务 4：让 WebSocket handler 接收 squadManager

### 修改文件

```text
E:\game-realtime-gm\backend\internal\handler\ws.go
```

操作类型：

```text
局部修改。
```

### 第一步：新增 import

搜索：

```go
tokenauth "game-realtime-gm/backend/internal/auth"
realtimews "game-realtime-gm/backend/internal/ws"
```

改成：

```go
tokenauth "game-realtime-gm/backend/internal/auth"
gamesquad "game-realtime-gm/backend/internal/squad"
realtimews "game-realtime-gm/backend/internal/ws"
```

### 第二步：修改函数签名

搜索：

```go
func WebSocketEcho(jwtSecret string, wsManager *realtimews.Manager, redisClient *redis.Client) gin.HandlerFunc {
```

改成：

```go
func WebSocketEcho(jwtSecret string, wsManager *realtimews.Manager, redisClient *redis.Client, squadManager *gamesquad.Manager) gin.HandlerFunc {
```

### 为什么要传 squadManager

`wsManager` 管 WebSocket 连接。

`squadManager` 管小队业务状态。

它们职责不同：

```text
wsManager：谁在线、哪条连接。
squadManager：谁在哪个小队、谁是队长、谁准备了。
```

## 任务 5：处理 squad.create

### 修改文件

```text
E:\game-realtime-gm\backend\internal\handler\ws.go
```

操作类型：

```text
在 switch clientMessage.Type 中新增 case。
```

### 插入位置

搜索：

```go
case realtimews.MessageTypeDebugEcho:
```

在 debug echo 这个 case 结束后、`default:` 前面新增：

```go
			case realtimews.MessageTypeSquadCreate:
				createdSquad, err := squadManager.Create(claims.PlayerID, claims.Username)
				if err != nil {
					errMsg := squadErrorMessage(clientMessage.RequestID, err)
					if err := writeWebSocketJSON(conn, writeMu, errMsg); err != nil {
						log.Printf("websocket write squad create error failed: %v", err)
						return
					}
					continue
				}

				response := realtimews.NewServerMessage(
					realtimews.MessageTypeSquadCreateResult,
					clientMessage.RequestID,
					realtimews.SquadData{Squad: createdSquad},
				)
				if err := writeWebSocketJSON(conn, writeMu, response); err != nil {
					log.Printf("websocket write squad create response failed: %v", err)
					return
				}
```

### 为什么创建者自动 ready=true

创建小队的人通常已经表达了“我要组队”的意图。

所以 Day26 里创建者自动：

```text
ready=true
leader_id=自己
```

## 任务 6：处理 squad.join

### 修改文件

```text
E:\game-realtime-gm\backend\internal\handler\ws.go
```

操作类型：

```text
继续在 switch 中新增 case。
```

### 代码

在 `squad.create` case 后新增：

```go
			case realtimews.MessageTypeSquadJoin:
				var request realtimews.SquadJoinRequest
				if err := json.Unmarshal(clientMessage.Data, &request); err != nil {
					errMsg := realtimews.NewErrorMessage(clientMessage.RequestID, 40027, "invalid squad join data")
					if err := writeWebSocketJSON(conn, writeMu, errMsg); err != nil {
						log.Printf("websocket write squad join invalid data failed: %v", err)
						return
					}
					continue
				}

				if strings.TrimSpace(request.SquadID) == "" {
					errMsg := realtimews.NewErrorMessage(clientMessage.RequestID, 40028, "squad_id required")
					if err := writeWebSocketJSON(conn, writeMu, errMsg); err != nil {
						log.Printf("websocket write squad join missing squad_id failed: %v", err)
						return
					}
					continue
				}

				joinedSquad, err := squadManager.Join(request.SquadID, claims.PlayerID, claims.Username)
				if err != nil {
					errMsg := squadErrorMessage(clientMessage.RequestID, err)
					if err := writeWebSocketJSON(conn, writeMu, errMsg); err != nil {
						log.Printf("websocket write squad join error failed: %v", err)
						return
					}
					continue
				}

				response := realtimews.NewServerMessage(
					realtimews.MessageTypeSquadJoinResult,
					clientMessage.RequestID,
					realtimews.SquadData{Squad: joinedSquad},
				)
				if err := writeWebSocketJSON(conn, writeMu, response); err != nil {
					log.Printf("websocket write squad join response failed: %v", err)
					return
				}
```

### 错误解释

`40027 invalid squad join data`：

```text
data 不是合法的加入小队 JSON。
```

`40028 squad_id required`：

```text
data 里没有 squad_id。
```

## 任务 7：处理 squad.leave

### 修改文件

```text
E:\game-realtime-gm\backend\internal\handler\ws.go
```

### 代码

在 `squad.join` case 后新增：

```go
			case realtimews.MessageTypeSquadLeave:
				leftSquad, disbanded, err := squadManager.Leave(claims.PlayerID)
				if err != nil {
					errMsg := squadErrorMessage(clientMessage.RequestID, err)
					if err := writeWebSocketJSON(conn, writeMu, errMsg); err != nil {
						log.Printf("websocket write squad leave error failed: %v", err)
						return
					}
					continue
				}

				response := realtimews.NewServerMessage(
					realtimews.MessageTypeSquadLeaveResult,
					clientMessage.RequestID,
					realtimews.SquadLeaveData{
						Squad:    leftSquad,
						Disbanded: disbanded,
					},
				)
				if err := writeWebSocketJSON(conn, writeMu, response); err != nil {
					log.Printf("websocket write squad leave response failed: %v", err)
					return
				}
```

### 业务规则

如果小队有多个人：

```text
离开的人从成员列表移除。
```

如果离开的是队长：

```text
队长转给剩余成员中最早加入的人。
```

如果最后一个成员离开：

```text
小队解散，disbanded=true。
```

## 任务 8：处理 squad.ready

### 修改文件

```text
E:\game-realtime-gm\backend\internal\handler\ws.go
```

### 代码

在 `squad.leave` case 后新增：

```go
			case realtimews.MessageTypeSquadReady:
				var request realtimews.SquadReadyRequest
				if err := json.Unmarshal(clientMessage.Data, &request); err != nil {
					errMsg := realtimews.NewErrorMessage(clientMessage.RequestID, 40029, "invalid squad ready data")
					if err := writeWebSocketJSON(conn, writeMu, errMsg); err != nil {
						log.Printf("websocket write squad ready invalid data failed: %v", err)
						return
					}
					continue
				}

				updatedSquad, err := squadManager.SetReady(claims.PlayerID, request.Ready)
				if err != nil {
					errMsg := squadErrorMessage(clientMessage.RequestID, err)
					if err := writeWebSocketJSON(conn, writeMu, errMsg); err != nil {
						log.Printf("websocket write squad ready error failed: %v", err)
						return
					}
					continue
				}

				response := realtimews.NewServerMessage(
					realtimews.MessageTypeSquadReadyResult,
					clientMessage.RequestID,
					realtimews.SquadData{Squad: updatedSquad},
				)
				if err := writeWebSocketJSON(conn, writeMu, response); err != nil {
					log.Printf("websocket write squad ready response failed: %v", err)
					return
				}
```

### 为什么 ready 用 data

客户端可以设置：

```json
{
  "ready": true
}
```

也可以取消准备：

```json
{
  "ready": false
}
```

## 任务 9：处理 squad.me

### 修改文件

```text
E:\game-realtime-gm\backend\internal\handler\ws.go
```

### 代码

在 `squad.ready` case 后新增：

```go
			case realtimews.MessageTypeSquadMe:
				currentSquad, err := squadManager.GetByPlayer(claims.PlayerID)
				if err != nil {
					errMsg := squadErrorMessage(clientMessage.RequestID, err)
					if err := writeWebSocketJSON(conn, writeMu, errMsg); err != nil {
						log.Printf("websocket write squad me error failed: %v", err)
						return
					}
					continue
				}

				response := realtimews.NewServerMessage(
					realtimews.MessageTypeSquadMeResult,
					clientMessage.RequestID,
					realtimews.SquadData{Squad: currentSquad},
				)
				if err := writeWebSocketJSON(conn, writeMu, response); err != nil {
					log.Printf("websocket write squad me response failed: %v", err)
					return
				}
```

### 为什么需要 squad.me

它用于查询：

```text
我当前在哪个小队里。
```

后续前端 GM 或客户端调试时很有用。

## 任务 10：新增 squad 错误映射函数

### 修改文件

```text
E:\game-realtime-gm\backend\internal\handler\ws.go
```

操作类型：

```text
在 websocketPlayerClaims 函数前新增 helper。
```

### 插入代码

搜索：

```go
func websocketPlayerClaims
```

在它前面新增：

```go
func squadErrorMessage(requestID string, err error) realtimews.ServerMessage {
	switch err {
	case gamesquad.ErrPlayerAlreadyInSquad:
		return realtimews.NewErrorMessage(requestID, 40926, "player already in squad")
	case gamesquad.ErrPlayerNotInSquad:
		return realtimews.NewErrorMessage(requestID, 40426, "player not in squad")
	case gamesquad.ErrSquadNotFound:
		return realtimews.NewErrorMessage(requestID, 40427, "squad not found")
	case gamesquad.ErrSquadFull:
		return realtimews.NewErrorMessage(requestID, 40927, "squad is full")
	default:
		return realtimews.NewErrorMessage(requestID, 50026, "squad operation failed")
	}
}
```

### 错误码说明

| code | message | 场景 |
| --- | --- | --- |
| `40426` | `player not in squad` | 玩家还没有小队却执行离开、准备、查询我的小队 |
| `40427` | `squad not found` | 加入的小队不存在 |
| `40926` | `player already in squad` | 玩家已经在小队中，又尝试创建或加入 |
| `40927` | `squad is full` | 小队人数已满 |
| `50026` | `squad operation failed` | 未预期的小队操作错误 |

## 任务 11：修改 router.go 创建 squadManager

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
"game-realtime-gm/backend/internal/ws"
```

改成：

```go
"game-realtime-gm/backend/internal/middleware"
"game-realtime-gm/backend/internal/squad"
"game-realtime-gm/backend/internal/ws"
```

### 第二步：创建 squadManager

搜索：

```go
wsManager := ws.NewManager()
```

改成：

```go
wsManager := ws.NewManager()
squadManager := squad.NewManager()
```

### 第三步：修改 /ws 路由

搜索：

```go
r.GET("/ws", handler.WebSocketEcho(cfg.JWTSecret, wsManager, redisClient))
```

改成：

```go
r.GET("/ws", handler.WebSocketEcho(cfg.JWTSecret, wsManager, redisClient, squadManager))
```

### 为什么在 router.go 创建

`router.New` 是服务启动时组装依赖的地方。

当前项目已经在这里创建：

```go
wsManager := ws.NewManager()
```

所以今天也在这里创建：

```go
squadManager := squad.NewManager()
```

这样所有 WebSocket 连接共享同一个小队管理器。

## 任务 12：执行 gofmt 和测试

### 执行命令

```powershell
cd E:\game-realtime-gm\backend
gofmt -w .\internal\squad\manager.go .\internal\ws\message.go .\internal\handler\ws.go .\internal\router\router.go
go test ./...
```

### 成功结果

你应该看到：

```text
?       game-realtime-gm/backend/internal/squad [no test files]
```

并且没有 `FAIL`。

### 常见编译错误

如果看到：

```text
undefined: gamesquad
```

说明 `handler/ws.go` 没有加 import：

```go
gamesquad "game-realtime-gm/backend/internal/squad"
```

如果看到：

```text
not enough arguments in call to handler.WebSocketEcho
```

说明 `router.go` 还没有把 `squadManager` 传进去。

## 任务 13：启动服务

### 启动 Docker

```powershell
cd E:\game-realtime-gm\deploy
docker compose up -d
docker ps
```

### 启动后端

```powershell
cd E:\game-realtime-gm\backend
go run .\cmd\server
```

如果 8080 端口不能用：

```powershell
$env:APP_PORT="18080"
go run .\cmd\server
```

## 任务 14：准备两个玩家 token

至少准备两个玩家账号，例如：

```text
player01 / 123456
player02 / 123456
```

如果没有 `player02`，先注册：

```http
POST http://localhost:8080/api/register
Content-Type: application/json

{
  "username": "player02",
  "password": "123456",
  "nickname": "玩家02"
}
```

分别登录拿 token：

```http
POST http://localhost:8080/api/login
Content-Type: application/json

{
  "username": "player01",
  "password": "123456"
}
```

```http
POST http://localhost:8080/api/login
Content-Type: application/json

{
  "username": "player02",
  "password": "123456"
}
```

复制两个响应里的：

```text
data.token
```

## 任务 15：Apifox 测试 squad.create

### 玩家 1 建立 WebSocket

连接地址：

```text
ws://localhost:8080/ws?token=玩家1token
```

### 发送

```json
{
  "type": "squad.create",
  "request_id": "req-create-001",
  "data": {}
}
```

### 预期响应

```json
{
  "type": "squad.create.result",
  "request_id": "req-create-001",
  "code": 0,
  "message": "ok",
  "data": {
    "squad": {
      "id": "squad_1",
      "leader_id": 1,
      "max_members": 4,
      "members": [
        {
          "player_id": 1,
          "username": "player01",
          "ready": true
        }
      ]
    }
  }
}
```

记录：

```text
squad_1
```

后面玩家 2 加入要用。

## 任务 16：测试重复创建错误

玩家 1 再发送一次：

```json
{
  "type": "squad.create",
  "request_id": "req-create-002",
  "data": {}
}
```

预期：

```json
{
  "type": "server.error",
  "request_id": "req-create-002",
  "code": 40926,
  "message": "player already in squad"
}
```

这说明：

```text
一个玩家不能同时在两个小队里。
```

## 任务 17：测试 squad.join

### 玩家 2 建立 WebSocket

连接地址：

```text
ws://localhost:8080/ws?token=玩家2token
```

### 发送

把 `squad_id` 换成玩家 1 创建出来的小队 id：

```json
{
  "type": "squad.join",
  "request_id": "req-join-001",
  "data": {
    "squad_id": "squad_1"
  }
}
```

### 预期响应

```json
{
  "type": "squad.join.result",
  "request_id": "req-join-001",
  "code": 0,
  "message": "ok",
  "data": {
    "squad": {
      "id": "squad_1",
      "leader_id": 1,
      "max_members": 4,
      "members": [
        {
          "player_id": 1,
          "username": "player01",
          "ready": true
        },
        {
          "player_id": 2,
          "username": "player02",
          "ready": false
        }
      ]
    }
  }
}
```

重点看：

```text
玩家 2 已加入。
玩家 2 初始 ready=false。
```

## 任务 18：测试 squad.ready

玩家 2 发送：

```json
{
  "type": "squad.ready",
  "request_id": "req-ready-001",
  "data": {
    "ready": true
  }
}
```

预期：

```text
玩家 2 的 ready 变成 true。
```

再发送：

```json
{
  "type": "squad.ready",
  "request_id": "req-ready-002",
  "data": {
    "ready": false
  }
}
```

预期：

```text
玩家 2 的 ready 变回 false。
```

## 任务 19：测试 squad.me

玩家 1 或玩家 2 发送：

```json
{
  "type": "squad.me",
  "request_id": "req-me-001",
  "data": {}
}
```

预期返回：

```text
当前玩家所在小队。
```

如果玩家不在小队里，返回：

```json
{
  "type": "server.error",
  "request_id": "req-me-001",
  "code": 40426,
  "message": "player not in squad"
}
```

## 任务 20：测试 squad.leave

玩家 2 发送：

```json
{
  "type": "squad.leave",
  "request_id": "req-leave-001",
  "data": {}
}
```

预期：

```json
{
  "type": "squad.leave.result",
  "request_id": "req-leave-001",
  "code": 0,
  "message": "ok",
  "data": {
    "squad": {
      "id": "squad_1",
      "leader_id": 1,
      "members": [
        {
          "player_id": 1,
          "username": "player01",
          "ready": true
        }
      ]
    },
    "disbanded": false
  }
}
```

然后玩家 1 发送：

```json
{
  "type": "squad.leave",
  "request_id": "req-leave-002",
  "data": {}
}
```

预期：

```json
{
  "type": "squad.leave.result",
  "request_id": "req-leave-002",
  "code": 0,
  "message": "ok",
  "data": {
    "disbanded": true
  }
}
```

表示最后一个成员离开，小队解散。

## 任务 21：更新接口文档

### 修改文件

```text
E:\game-realtime-gm\docs\api-overview.md
```

操作类型：

```text
局部修改。
```

### 建议补充位置

搜索：

```markdown
当前支持的业务消息：
```

在 `debug.echo` 后补充：

| type | 说明 |
| --- | --- |
| `squad.create` | 创建小队，创建者成为队长并自动 ready=true |
| `squad.join` | 加入指定小队，需要 `data.squad_id` |
| `squad.leave` | 离开当前小队；最后一人离开时小队解散 |
| `squad.ready` | 设置当前玩家准备状态，需要 `data.ready` |
| `squad.me` | 查询当前玩家所在小队 |

### 错误码补充

在 WebSocket 错误表里补充：

| code | message | 场景 |
| --- | --- | --- |
| `40027` | `invalid squad join data` | `squad.join` 的 data 不是合法 JSON |
| `40028` | `squad_id required` | `squad.join` 缺少 `squad_id` |
| `40029` | `invalid squad ready data` | `squad.ready` 的 data 不是合法 JSON |
| `40426` | `player not in squad` | 玩家不在小队中 |
| `40427` | `squad not found` | 小队不存在 |
| `40926` | `player already in squad` | 玩家已经在小队中 |
| `40927` | `squad is full` | 小队已满 |
| `50026` | `squad operation failed` | 小队操作服务端错误 |

## 任务 22：今日复盘

完成后用自己的话回答：

1. 小队房间和普通聊天室有什么区别？
2. 为什么今天用内存保存小队？
3. `squads` 和 `playerSquad` 两个 map 分别解决什么问题？
4. 为什么 `Manager` 要加锁？
5. 为什么创建者自动成为队长？
6. 为什么普通成员加入时 `ready=false`？
7. 队长离开时怎么处理？
8. 为什么 Day26 不做广播？

参考表达：

```text
Day26 我在 WebSocket 统一消息协议上实现了小队房间基础能力，包括创建小队、加入小队、离开小队、设置准备状态和查询我的小队。小队状态先保存在进程内存中，用 mutex 保证并发安全。这个功能让项目从单个玩家连接进入多人 PVE 业务，为 Day27 小队状态广播和后续 PVE 匹配打基础。
```

## 今日验收清单

- [ ] 已新增 `backend/internal/squad/manager.go`。
- [ ] `squad.Manager` 能创建小队。
- [ ] `squad.Manager` 能加入小队。
- [ ] `squad.Manager` 能离开小队。
- [ ] `squad.Manager` 能设置准备状态。
- [ ] `squad.Manager` 能查询玩家所在小队。
- [ ] `ws/message.go` 已新增 `squad.*` 消息类型。
- [ ] `handler/ws.go` 已处理 `squad.create`。
- [ ] `handler/ws.go` 已处理 `squad.join`。
- [ ] `handler/ws.go` 已处理 `squad.leave`。
- [ ] `handler/ws.go` 已处理 `squad.ready`。
- [ ] `handler/ws.go` 已处理 `squad.me`。
- [ ] `router.go` 已创建并传入 `squadManager`。
- [ ] `gofmt` 执行成功。
- [ ] `go test ./...` 通过。
- [ ] Apifox 能创建小队。
- [ ] Apifox 能让第二个玩家加入小队。
- [ ] Apifox 能设置 ready。
- [ ] Apifox 能离开小队。
- [ ] `docs/api-overview.md` 已更新。

## 常见问题

### 1. go test 报 package squad is not in std

检查 import 是否写成了：

```go
"game-realtime-gm/backend/internal/squad"
```

不要只写：

```go
"squad"
```

### 2. go test 报 undefined: SquadData

检查 `backend/internal/ws/message.go` 是否已经新增：

```go
type SquadData struct
```

### 3. squad.join 返回 squad not found

检查 `squad_id` 是否和创建小队返回的一致。

例如创建返回：

```text
squad_1
```

加入时也必须写：

```json
{
  "squad_id": "squad_1"
}
```

### 4. 玩家 2 加入后玩家 1 没收到消息

这是正常的。

Day26 只返回给当前操作玩家。

小队广播是 Day27 的任务。

### 5. 服务重启后小队没了

这是正常的。

Day26 小队状态只存在 Go 进程内存。

服务重启后内存清空。

后续可以再考虑 Redis 或 PostgreSQL。

## 今日不要做什么

今天不要做：

- 小队广播。
- GM 查询小队。
- Redis 保存小队状态。
- PostgreSQL 保存小队记录。
- PVE 匹配。
- 任务副本生命周期。
- 结算记录。
- UDP / KCP / QUIC。
- 前端页面。

原因：

```text
今天只做小队房间的最小业务闭环。
先让创建、加入、离开、准备能跑通，再做广播和匹配。
```

## 任务 23：提交并推送到 GitHub

### 先检查状态

```powershell
cd E:\game-realtime-gm
git status
```

确认不要提交：

```text
.env
*.log
真实数据库密码
真实 JWT_SECRET
```

### 选择性添加 Day26 文件

```powershell
git add backend/internal/squad/manager.go backend/internal/ws/message.go backend/internal/handler/ws.go backend/internal/router/router.go docs/api-overview.md docs/day/day26-plan.md
```

不要使用：

```powershell
git add .
```

### 提交

```powershell
git commit -m "Complete day26 squad room basics"
```

### 推送

```powershell
git push
```

### 推送后检查

```powershell
git status -sb
```

理想结果：

```text
## main...origin/main
```

## 明日预告

Day27 建议继续小队主线：

```text
小队状态广播
```

目标：

```text
成员加入广播
成员离开广播
准备状态广播
队长变化广播
```

Day27 会让小队从“只有当前操作人知道结果”升级为：

```text
小队内所有在线成员都能收到状态变化。
```
