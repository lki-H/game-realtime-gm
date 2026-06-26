# Day 24 学习计划：统一 WebSocket 消息协议

## 你今天从任务几开始

今天从任务 1 开始。

Day 23 已经完成：

```text
WebSocket 玩家 token 鉴权
        ↓
建立玩家长连接
        ↓
写入 Redis 在线状态并续期
        ↓
服务端定时 ping
        ↓
客户端返回 pong
        ↓
服务端通过读超时发现死连接
```

Day 24 要继续 WebSocket 主线，但今天不做小队房间，也不做匹配。

今天只解决一个问题：

```text
WebSocket 消息不能继续只靠普通字符串 echo。
```

后续小队准备、加入小队、退出小队、匹配、错误返回、广播，都需要统一消息格式。

## 今日目标

今天要实现：

```text
统一 WebSocket JSON 消息协议。
```

客户端以后发送 WebSocket 消息时，统一使用：

```json
{
  "type": "debug.echo",
  "request_id": "req-001",
  "data": {
    "text": "hello day24"
  }
}
```

服务端统一返回：

```json
{
  "type": "debug.echo.result",
  "request_id": "req-001",
  "code": 0,
  "message": "ok",
  "data": {
    "received_type": "debug.echo",
    "received_data": {
      "text": "hello day24"
    }
  },
  "server_time": "2026-06-26T10:00:00+08:00"
}
```

一句话理解：

```text
Day 23 让连接更可靠，Day 24 让连接里的消息变得可扩展。
```

## 今日最终效果

你用 Apifox 连接：

```text
ws://localhost:8080/ws?token=玩家token
```

连接成功后，服务端 welcome 消息会从旧结构逐步改成统一结构。

发送合法 JSON：

```json
{
  "type": "debug.echo",
  "request_id": "req-001",
  "data": {
    "text": "hello day24"
  }
}
```

服务端返回统一 JSON。

发送非法 JSON：

```text
hello day24
```

服务端不再原样 echo，而是返回统一错误：

```json
{
  "type": "server.error",
  "code": 40024,
  "message": "invalid websocket message json",
  "server_time": "2026-06-26T10:00:00+08:00"
}
```

发送缺少 `type` 的 JSON：

```json
{
  "request_id": "req-002",
  "data": {}
}
```

服务端返回：

```json
{
  "type": "server.error",
  "request_id": "req-002",
  "code": 40025,
  "message": "websocket message type required",
  "server_time": "2026-06-26T10:00:00+08:00"
}
```

## 今日会学到什么

你今天会学到：

- 为什么 WebSocket 不能一直只传字符串。
- 什么是消息协议。
- `type` 字段为什么重要。
- `request_id` 用来解决什么问题。
- `data` 为什么要使用 `json.RawMessage`。
- 服务端为什么要有统一错误返回。
- 为什么今天只做 `debug.echo`，不急着做小队业务。
- 为什么消息结构适合放到 `internal/ws/message.go`，而不是继续塞在 `handler/ws.go`。

## 今日文件范围

今天主要涉及：

```text
E:\game-realtime-gm\backend\internal\ws\message.go
E:\game-realtime-gm\backend\internal\handler\ws.go
E:\game-realtime-gm\docs\api-overview.md
E:\game-realtime-gm\docs\day\day24-plan.md
```

文件职责说明：

| 文件 | 作用 |
| --- | --- |
| `internal/ws/message.go` | 新增统一 WebSocket 消息结构、消息类型常量和响应结构 |
| `internal/handler/ws.go` | WebSocket 连接入口，今天从字符串 echo 改成解析统一 JSON 消息 |
| `docs/api-overview.md` | 更新 `/ws` 文档，补充 Day24 消息协议 |
| `docs/day/day24-plan.md` | 今天这份学习教程 |

今天不新增数据库表。

今天不新增 Redis key。

今天不新增 Go 第三方依赖。

今天不改路由。

原因：

```text
WebSocket 路由仍然是 /ws。
今天只改变连接建立后的消息内容格式。
```

## 任务 0：确认当前状态

### 你要做什么

先确认 Day23 已经提交，当前代码干净。

### 执行命令

```powershell
cd E:\game-realtime-gm
git status -sb
```

理想情况：

```text
## main...origin/main
```

如果仍然看到下面这些未跟踪文件，是正常的，不要提交：

```text
docs/codex-context.md
docs/conversation-handoff-gpt55.md
docs/mcp-adoption-plan.md
docs/skill-adoption-plan.md
```

### 再跑后端测试

```powershell
cd E:\game-realtime-gm\backend
go test ./...
```

成功时会看到多个包显示：

```text
[no test files]
```

这不是错误，表示当前包还没有测试文件，但能正常编译。

## 任务 1：理解为什么需要统一 WebSocket 消息协议

### 当前 Day23 的问题

Day23 的 WebSocket 收到客户端消息后，还是：

```text
客户端发什么字符串
        ↓
服务端原样回显
```

这适合测试连接是否通，但不适合真实业务。

后续如果要支持：

```text
玩家准备
创建小队
加入小队
退出小队
进入匹配
取消匹配
任务副本状态广播
```

服务端必须知道客户端发来的消息想做什么。

所以需要统一结构：

```json
{
  "type": "squad.ready",
  "request_id": "req-001",
  "data": {}
}
```

### 字段解释

| 字段 | 作用 |
| --- | --- |
| `type` | 表示这条消息是什么业务动作，例如 `debug.echo`、`squad.ready` |
| `request_id` | 客户端生成的请求编号，服务端响应时带回，方便客户端对应请求和响应 |
| `data` | 具体业务数据，不同 type 里面结构不同 |

### 为什么今天先做 debug.echo

今天不要一上来做小队。

原因：

```text
先证明消息协议能解析、能返回、能报错。
再把小队、匹配、任务副本挂到这个协议上。
```

这就像先修路，再跑车。

## 任务 2：新增 WebSocket 消息结构文件

### 修改文件

```text
E:\game-realtime-gm\backend\internal\ws\message.go
```

操作类型：

```text
新建文件。
```

### 为什么放在 internal/ws

当前已有：

```text
E:\game-realtime-gm\backend\internal\ws\manager.go
```

`manager.go` 管连接。

今天新增的 `message.go` 管消息格式。

这样后续 `handler/ws.go` 不会越来越乱。

### 完整代码

新建文件：

```text
E:\game-realtime-gm\backend\internal\ws\message.go
```

填入：

```go
package ws

import (
	"encoding/json"
	"time"
)

const (
	MessageTypeServerWelcome = "server.welcome"
	MessageTypeServerError   = "server.error"

	MessageTypeDebugEcho       = "debug.echo"
	MessageTypeDebugEchoResult = "debug.echo.result"
)

type ClientMessage struct {
	Type      string          `json:"type"`
	RequestID string          `json:"request_id,omitempty"`
	Data      json.RawMessage `json:"data,omitempty"`
}

type ServerMessage struct {
	Type       string    `json:"type"`
	RequestID  string    `json:"request_id,omitempty"`
	Code       int       `json:"code"`
	Message    string    `json:"message"`
	Data       any       `json:"data,omitempty"`
	ServerTime time.Time `json:"server_time"`
}

type WelcomeData struct {
	PlayerID      int64  `json:"player_id"`
	Username      string `json:"username"`
	OnlinePlayers int    `json:"online_players"`
	OnlineTTL     int    `json:"online_ttl_seconds"`
}

type EchoData struct {
	ReceivedType string          `json:"received_type"`
	ReceivedData json.RawMessage `json:"received_data,omitempty"`
}

func NewServerMessage(messageType string, requestID string, data any) ServerMessage {
	return ServerMessage{
		Type:       messageType,
		RequestID:  requestID,
		Code:       0,
		Message:    "ok",
		Data:       data,
		ServerTime: time.Now(),
	}
}

func NewErrorMessage(requestID string, code int, message string) ServerMessage {
	return ServerMessage{
		Type:       MessageTypeServerError,
		RequestID:  requestID,
		Code:       code,
		Message:    message,
		ServerTime: time.Now(),
	}
}
```

### 代码分析

```go
package ws
```

表示这个文件属于 `internal/ws` 包。

`manager.go` 也是 `package ws`，所以它们在同一个包里。

```go
import "encoding/json"
```

用于 `json.RawMessage`。

`json.RawMessage` 的意思是：

```text
先把 data 当成原始 JSON 保存下来。
等后续知道 type 是什么，再决定把 data 解析成哪种具体结构。
```

今天 `debug.echo` 只把它原样返回。

```go
type ClientMessage struct
```

表示客户端发给服务端的消息。

```go
type ServerMessage struct
```

表示服务端发给客户端的统一响应。

```go
NewServerMessage
NewErrorMessage
```

是两个小 helper：

- 正常响应用 `NewServerMessage`
- 错误响应用 `NewErrorMessage`

这样以后不会每个地方都手写 `server_time`、`code`、`message`。

## 任务 3：修改 handler/ws.go 的 import

### 修改文件

```text
E:\game-realtime-gm\backend\internal\handler\ws.go
```

操作类型：

```text
局部修改。
```

### 修改位置

搜索：

```go
import (
	"context"
	"log"
```

### 修改前

```go
import (
	"context"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
```

### 修改后

```go
import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
```

### 为什么要增加 encoding/json

Day24 要把客户端发来的文本消息解析成：

```go
realtimews.ClientMessage
```

所以需要：

```go
json.Unmarshal(message, &clientMessage)
```

## 任务 4：删除旧 WSMessage 类型

### 修改文件

```text
E:\game-realtime-gm\backend\internal\handler\ws.go
```

操作类型：

```text
删除旧类型。
```

### 搜索关键词

```go
type WSMessage struct
```

### 删除这整段

```go
type WSMessage struct {
	Type          string    `json:"type"`
	Content       string    `json:"content,omitempty"`
	ServerTime    time.Time `json:"server_time"`
	PlayerID      int64     `json:"player_id,omitempty"`
	Username      string    `json:"username,omitempty"`
	OnlinePlayers int       `json:"online_players,omitempty"`
	OnlineTTL     int       `json:"online_ttl_seconds,omitempty"`
}
```

### 为什么删除

Day23 的 `WSMessage` 只适合 welcome 和简单 error。

Day24 要统一所有消息格式，所以改用：

```go
realtimews.ServerMessage
```

消息结构放到 `internal/ws/message.go` 后，后续小队模块、匹配模块也可以复用。

## 任务 5：修改 Redis 在线状态失败时的错误返回

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
update online status failed
```

### 修改前

```go
_ = writeWebSocketJSON(conn, writeMu, WSMessage{
	Type:       "error",
	Content:    "update online status failed",
	ServerTime: time.Now(),
	PlayerID:   claims.PlayerID,
	Username:   claims.Username,
})
```

### 修改后

```go
_ = writeWebSocketJSON(conn, writeMu, realtimews.NewErrorMessage("", 50024, "update online status failed"))
```

### 为什么这样改

这里连接刚建立，还没有客户端业务请求，所以 `request_id` 为空字符串。

错误码 `50024` 表示：

```text
Day24 WebSocket 服务端内部错误：更新在线状态失败。
```

## 任务 6：修改 welcome 消息

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
welcome := WSMessage{
```

### 修改前

```go
welcome := WSMessage{
	Type:          "welcome",
	Content:       "connected to game realtime server",
	ServerTime:    time.Now(),
	PlayerID:      claims.PlayerID,
	Username:      claims.Username,
	OnlinePlayers: wsManager.Count(),
	OnlineTTL:     int(onlineTTL.Seconds()),
}
```

### 修改后

```go
welcome := realtimews.NewServerMessage(
	realtimews.MessageTypeServerWelcome,
	"",
	realtimews.WelcomeData{
		PlayerID:      claims.PlayerID,
		Username:      claims.Username,
		OnlinePlayers: wsManager.Count(),
		OnlineTTL:     int(onlineTTL.Seconds()),
	},
)
```

### 新 welcome 格式

改完后连接成功返回：

```json
{
  "type": "server.welcome",
  "code": 0,
  "message": "ok",
  "data": {
    "player_id": 1,
    "username": "player01",
    "online_players": 1,
    "online_ttl_seconds": 120
  },
  "server_time": "2026-06-26T10:00:00+08:00"
}
```

### 为什么 welcome 也要统一

如果 welcome 仍然使用旧结构，而业务消息使用新结构，前端处理会很麻烦。

统一后，客户端可以固定按：

```text
type
code
message
data
server_time
```

来处理服务端所有消息。

## 任务 7：把 echo 逻辑改成解析统一 JSON 消息

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
log.Printf("websocket received from player_id=%d: %s", claims.PlayerID, string(message))
```

### 修改前

```go
log.Printf("websocket received from player_id=%d: %s", claims.PlayerID, string(message))

if err := writeWebSocketMessage(conn, writeMu, messageType, message); err != nil {
	log.Printf("websocket write echo failed: %v", err)
	return
}
```

### 修改后

```go
log.Printf("websocket received from player_id=%d: %s", claims.PlayerID, string(message))

	if messageType != websocket.TextMessage {
		errMsg := realtimews.NewErrorMessage("", 40026, "websocket only supports text json messages")
		if err := writeWebSocketJSON(conn, writeMu, errMsg); err != nil {
			log.Printf("websocket write message type error failed: %v", err)
			return
		}
		continue
	}

	var clientMessage realtimews.ClientMessage
	if err := json.Unmarshal(message, &clientMessage); err != nil {
		errMsg := realtimews.NewErrorMessage("", 40024, "invalid websocket message json")
		if err := writeWebSocketJSON(conn, writeMu, errMsg); err != nil {
			log.Printf("websocket write invalid json error failed: %v", err)
			return
		}
		continue
	}

	if strings.TrimSpace(clientMessage.Type) == "" {
		errMsg := realtimews.NewErrorMessage(clientMessage.RequestID, 40025, "websocket message type required")
		if err := writeWebSocketJSON(conn, writeMu, errMsg); err != nil {
			log.Printf("websocket write missing type error failed: %v", err)
			return
		}
		continue
	}

	switch clientMessage.Type {
	case realtimews.MessageTypeDebugEcho:
		response := realtimews.NewServerMessage(
			realtimews.MessageTypeDebugEchoResult,
			clientMessage.RequestID,
			realtimews.EchoData{
				ReceivedType: clientMessage.Type,
				ReceivedData: clientMessage.Data,
			},
		)
		if err := writeWebSocketJSON(conn, writeMu, response); err != nil {
			log.Printf("websocket write echo response failed: %v", err)
			return
		}
	default:
		errMsg := realtimews.NewErrorMessage(clientMessage.RequestID, 40424, "unsupported websocket message type")
		if err := writeWebSocketJSON(conn, writeMu, errMsg); err != nil {
			log.Printf("websocket write unsupported type error failed: %v", err)
			return
		}
	}
```

注意：粘贴后如果缩进不整齐，不要手动慢慢调，后面执行 `gofmt` 会自动整理。

### 代码分析

```go
if messageType != websocket.TextMessage
```

表示今天只支持文本 JSON。

二进制消息暂时不支持。

```go
json.Unmarshal(message, &clientMessage)
```

把客户端发来的 JSON 文本解析成 Go 结构体。

如果客户端发的是：

```text
hello day24
```

它不是 JSON，所以会返回 `40024`。

```go
strings.TrimSpace(clientMessage.Type) == ""
```

检查 `type` 是否为空。

`type` 是消息协议最重要的字段，没有它服务端不知道该做什么。

```go
switch clientMessage.Type
```

根据消息类型分发处理。

今天只支持：

```text
debug.echo
```

以后会继续加：

```text
squad.create
squad.join
squad.ready
matchmaking.join
mission.start
```

## 任务 8：保留或删除 writeWebSocketMessage

### 修改文件

```text
E:\game-realtime-gm\backend\internal\handler\ws.go
```

操作类型：

```text
删除不再使用的 helper。
```

### 搜索关键词

```go
func writeWebSocketMessage
```

### 删除这整个函数

```go
func writeWebSocketMessage(conn *websocket.Conn, writeMu *sync.Mutex, messageType int, message []byte) error {
	writeMu.Lock()
	defer writeMu.Unlock()

	if err := conn.SetWriteDeadline(time.Now().Add(webSocketWriteWait)); err != nil {
		return err
	}
	return conn.WriteMessage(messageType, message)
}
```

### 为什么删除

Day24 后普通业务消息都通过：

```go
writeWebSocketJSON
```

返回统一 JSON。

`writeWebSocketMessage` 只适合原始 echo，现在不需要了。

## 任务 9：完成后执行 gofmt 和测试

### 执行命令

```powershell
cd E:\game-realtime-gm\backend
gofmt -w .\internal\ws\message.go .\internal\handler\ws.go
go test ./...
```

### 命令解释

`gofmt`：

```text
格式化 Go 文件，自动整理 import、缩进和空行。
```

`go test ./...`：

```text
编译并测试 backend 下所有 Go 包。
```

### 成功结果

你应该看到类似：

```text
?       game-realtime-gm/backend/internal/handler [no test files]
?       game-realtime-gm/backend/internal/ws      [no test files]
```

如果看到：

```text
imported and not used
```

说明某个 import 没有用。

今天重点检查：

```text
encoding/json
time
```

如果你已经删除 `WSMessage`，但还保留 `time` 的使用，是正常的，因为心跳常量和 deadline 仍然使用 `time`。

## 任务 10：启动 Docker Desktop 依赖

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

虽然今天改的是 WebSocket 消息协议，但 `/ws` 建连时仍然会写：

```text
online:player:<player_id>
```

Redis 没启动时，连接会返回在线状态更新失败。

## 任务 11：启动后端

### 默认启动

```powershell
cd E:\game-realtime-gm\backend
go run .\cmd\server
```

### 如果 8080 端口不能用

你之前遇到过 Windows 端口排除范围问题。

如果又看到：

```text
listen tcp :8080: bind
```

临时改端口：

```powershell
cd E:\game-realtime-gm\backend
$env:APP_PORT="18080"
go run .\cmd\server
```

测试地址也改成：

```text
http://localhost:18080
ws://localhost:18080/ws?token=玩家token
```

## 任务 12：准备玩家 token

如果已有玩家账号，登录：

```http
POST http://localhost:8080/api/login
Content-Type: application/json

{
  "username": "player01",
  "password": "123456"
}
```

如果使用 18080：

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

## 任务 13：用 Apifox 测试连接成功 welcome

### 连接地址

```text
ws://localhost:8080/ws?token=玩家token
```

如果使用 18080：

```text
ws://localhost:18080/ws?token=玩家token
```

### 预期 welcome

连接成功后，welcome 应该类似：

```json
{
  "type": "server.welcome",
  "code": 0,
  "message": "ok",
  "data": {
    "player_id": 1,
    "username": "player01",
    "online_players": 1,
    "online_ttl_seconds": 120
  },
  "server_time": "2026-06-26T10:00:00+08:00"
}
```

如果你仍然看到旧格式：

```json
{
  "type": "welcome",
  "content": "connected to game realtime server"
}
```

说明 `welcome := ...` 那一段还没改成功。

## 任务 14：测试合法 debug.echo 消息

### 发送消息

在 Apifox WebSocket 消息框里发送：

```json
{
  "type": "debug.echo",
  "request_id": "req-001",
  "data": {
    "text": "hello day24"
  }
}
```

### 预期响应

```json
{
  "type": "debug.echo.result",
  "request_id": "req-001",
  "code": 0,
  "message": "ok",
  "data": {
    "received_type": "debug.echo",
    "received_data": {
      "text": "hello day24"
    }
  },
  "server_time": "2026-06-26T10:00:00+08:00"
}
```

### 验证重点

重点看：

```text
request_id 是否原样返回
type 是否变成 debug.echo.result
data.received_data 是否包含你发的内容
```

这说明服务端已经能理解统一消息格式。

## 任务 15：测试非法 JSON

### 发送消息

```text
hello day24
```

### 预期响应

```json
{
  "type": "server.error",
  "code": 40024,
  "message": "invalid websocket message json",
  "server_time": "2026-06-26T10:00:00+08:00"
}
```

### 为什么要测试这个

真实项目里，客户端可能因为 bug 发错消息。

服务端不能直接崩溃，也不能静默不回。

统一错误响应可以让客户端知道哪里错了。

## 任务 16：测试缺少 type

### 发送消息

```json
{
  "request_id": "req-002",
  "data": {}
}
```

### 预期响应

```json
{
  "type": "server.error",
  "request_id": "req-002",
  "code": 40025,
  "message": "websocket message type required",
  "server_time": "2026-06-26T10:00:00+08:00"
}
```

### 验证重点

重点看：

```text
request_id 是否仍然返回 req-002
```

即使请求错了，只要服务端能读到 request_id，也应该带回去。

这样客户端能知道是哪次请求错了。

## 任务 17：测试不支持的 type

### 发送消息

```json
{
  "type": "squad.ready",
  "request_id": "req-003",
  "data": {}
}
```

### 预期响应

```json
{
  "type": "server.error",
  "request_id": "req-003",
  "code": 40424,
  "message": "unsupported websocket message type",
  "server_time": "2026-06-26T10:00:00+08:00"
}
```

### 为什么 squad.ready 现在还不支持

`squad.ready` 是后续小队系统会用的消息类型。

今天先让服务端知道：

```text
这个 type 目前不支持，但消息协议已经能识别它。
```

这为 Day26 小队房间做准备。

## 任务 18：验证 Day23 心跳没有被破坏

保持 Apifox WebSocket 连接 30 秒以上。

观察后端日志，应该仍然能看到类似：

```text
websocket pong received: player_id=1 username=player01 data=ping
```

然后进入 Redis：

```powershell
docker exec -it game_realtime_redis redis-cli
```

查看：

```text
TTL online:player:1
```

连接保持时 TTL 应该会续期。

这证明：

```text
Day24 消息协议没有破坏 Day23 心跳和 Day22 在线状态。
```

## 任务 19：更新接口文档

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
### GET /ws
```

### 建议补充内容

在 `/ws` 说明中补充下面内容。注意这里是要粘贴到 Markdown 文档里的正文，不要再额外套一层大的代码块。

Day 24 起，WebSocket 业务消息使用统一 JSON 协议。

客户端消息格式：

```json
{
  "type": "debug.echo",
  "request_id": "req-001",
  "data": {}
}
```

服务端消息格式：

```json
{
  "type": "debug.echo.result",
  "request_id": "req-001",
  "code": 0,
  "message": "ok",
  "data": {},
  "server_time": "2026-06-26T10:00:00+08:00"
}
```

当前支持的业务消息：

| type | 说明 |
| --- | --- |
| `debug.echo` | 调试用 echo 消息，服务端会按统一格式返回 `debug.echo.result` |

当前错误：

| code | message | 场景 |
| --- | --- | --- |
| `40024` | `invalid websocket message json` | 客户端发送的不是合法 JSON |
| `40025` | `websocket message type required` | JSON 中缺少 type |
| `40026` | `websocket only supports text json messages` | 客户端发送了非文本消息 |
| `40424` | `unsupported websocket message type` | type 暂未支持 |
| `50024` | `update online status failed` | 连接建立后更新 Redis 在线状态失败 |

复制时从“Day 24 起”这一行开始，到错误表格结束即可。

## 任务 20：今日复盘

完成后用自己的话回答：

1. 为什么 WebSocket 不能一直只做字符串 echo？
2. `type` 字段解决什么问题？
3. `request_id` 的作用是什么？
4. `data` 为什么不固定成某一种结构？
5. `json.RawMessage` 是什么？
6. 为什么服务端要返回统一错误结构？
7. 今天为什么只做 `debug.echo`，不直接做 `squad.ready`？
8. Day24 和 Day23 的关系是什么？

参考表达：

```text
Day24 我把 WebSocket 消息从普通字符串 echo 改成统一 JSON 协议。
客户端消息包含 type、request_id 和 data，服务端根据 type 分发处理，并统一返回 type、request_id、code、message、data、server_time。
今天只实现 debug.echo，是为了先验证协议解析、正常响应和错误响应，为后续小队房间、匹配和任务副本消息打基础。
```

## 今日验收清单

- [ ] 已新增 `backend/internal/ws/message.go`。
- [ ] `message.go` 中有 `ClientMessage`。
- [ ] `message.go` 中有 `ServerMessage`。
- [ ] `message.go` 中有 `WelcomeData`。
- [ ] `message.go` 中有 `EchoData`。
- [ ] `message.go` 中有 `NewServerMessage`。
- [ ] `message.go` 中有 `NewErrorMessage`。
- [ ] `handler/ws.go` 已新增 `encoding/json` import。
- [ ] `handler/ws.go` 已删除旧 `WSMessage`。
- [ ] welcome 消息已改成 `server.welcome`。
- [ ] 客户端发送 `debug.echo` 能收到 `debug.echo.result`。
- [ ] 非法 JSON 能返回 `40024`。
- [ ] 缺少 `type` 能返回 `40025`。
- [ ] 不支持的 `type` 能返回 `40424`。
- [ ] Day23 ping/pong 心跳仍然正常。
- [ ] Redis 在线 TTL 仍然能续期。
- [ ] `gofmt` 执行成功。
- [ ] `go test ./...` 通过。
- [ ] `docs/api-overview.md` 已更新。

## 常见问题

### 1. go test 报 undefined: realtimews.ClientMessage

检查是否已经新建：

```text
E:\game-realtime-gm\backend\internal\ws\message.go
```

并且文件开头是：

```go
package ws
```

不要写成：

```go
package handler
```

### 2. go test 报 undefined: json

检查 `handler/ws.go` import 中是否增加：

```go
"encoding/json"
```

### 3. go test 报 WSMessage 未定义

说明你删除了 `WSMessage`，但还有地方在用它。

搜索：

```text
WSMessage
```

所有旧用法都要改成：

```go
realtimews.NewServerMessage(...)
realtimews.NewErrorMessage(...)
```

### 4. go test 报 writeWebSocketMessage 未定义

说明你删除了 `writeWebSocketMessage`，但 read loop 里还在调用它。

搜索：

```text
writeWebSocketMessage
```

Day24 后不应该再调用它。

### 5. 发送 debug.echo 没响应

优先检查：

```text
客户端发的是不是合法 JSON
type 是否正好是 debug.echo
request_id 是否写成了 requestId
后端日志是否有 websocket received
```

字段名必须是：

```text
request_id
```

不是：

```text
requestId
```

### 6. 连接后 welcome 还是旧格式

检查 `handler/ws.go` 中是否还有：

```go
Type: "welcome"
Content: "connected to game realtime server"
```

如果还有，说明任务 6 没改完。

## 今日不要做什么

今天不要做：

- 小队房间创建。
- 小队加入退出。
- 小队准备状态。
- 小队广播。
- PVE 匹配队列。
- 任务副本生命周期。
- 断线重连恢复。
- UDP / KCP / QUIC。
- 前端页面。

原因：

```text
今天只做 WebSocket 消息协议层。
协议稳定后，后续业务消息才能挂上去。
```

## 任务 21：提交并推送到 GitHub

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

### 选择性添加 Day24 相关文件

```powershell
git add backend/internal/ws/message.go backend/internal/handler/ws.go docs/api-overview.md docs/day/day24-plan.md
```

不要使用：

```powershell
git add .
```

### 提交

```powershell
git commit -m "Complete day24 websocket message protocol"
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

Day 25 建议继续 WebSocket 主线：

```text
连接会话 ID
```

目标：

```text
给每次 WebSocket 连接生成 connection_id。
```

它会解决：

```text
同一个玩家重复连接时，怎么区分旧连接和新连接。
后续断线重连时，怎么更安全地判断当前连接是谁。
```
