# Day 19 学习计划：WebSocket 最小连接接口

## 你今天从任务几开始

今天从任务 1 开始。

Day 18 已经把 GM 后台 dashboard 最近操作日志接口推进到文档阶段。Day 19 开始进入实时服务主线：

```text
WebSocket 最小连接接口
```

今天先不要做复杂房间、匹配、广播、鉴权。今天只做一个能跑通的最小闭环：

```text
客户端连接 ws://localhost:8080/ws
        ↓
后端把 HTTP 请求升级成 WebSocket 长连接
        ↓
后端发送欢迎消息
        ↓
客户端发送一条消息
        ↓
后端原样回显
        ↓
客户端断开
        ↓
后端打印断开日志
```

## 今日目标

今天要完成一个最小 WebSocket 接口：

```text
GET /ws
```

它的作用是：

- 让后端支持 WebSocket 长连接。
- 让你理解 HTTP 接口和 WebSocket 连接的区别。
- 为后续“玩家实时在线连接”“房间广播”“实时对战消息”打基础。

## 今日最终效果

后端启动后，你可以用 Apifox 的 WebSocket 功能连接：

```text
ws://localhost:8080/ws
```

连接成功后，应该收到类似消息：

```json
{
  "type": "welcome",
  "content": "connected to game realtime server",
  "server_time": "2026-06-22T10:00:00+08:00"
}
```

然后你在客户端发送：

```text
hello websocket
```

后端会原样回给你：

```text
hello websocket
```

这就是 Day 19 的成功标准。

## 今日会学到什么

你今天会学到：

- WebSocket 是什么。
- 为什么实时游戏服务需要长连接。
- Go 项目如何引入第三方依赖。
- `github.com/gorilla/websocket` 的基本用法。
- Gin 里如何把 HTTP 请求升级成 WebSocket。
- WebSocket 连接建立、读消息、写消息、断开的基本生命周期。
- 为什么今天先不做 JWT 鉴权和连接管理。

一句话理解：

```text
HTTP 更像“问一次答一次”，WebSocket 更像“电话接通后一直说话”。
```

游戏服务里很多场景不能只靠 HTTP：

- 房间内玩家移动。
- 对战操作同步。
- 匹配结果推送。
- GM 后台实时在线人数变化。
- 系统公告实时推送。

这些都适合 WebSocket。

## 今日文件范围

今天主要涉及这些文件：

```text
E:\game-realtime-gm\backend\go.mod
E:\game-realtime-gm\backend\go.sum
E:\game-realtime-gm\backend\internal\handler\ws.go
E:\game-realtime-gm\backend\internal\router\router.go
E:\game-realtime-gm\docs\api-overview.md
E:\game-realtime-gm\docs\day19-plan.md
```

文件职责说明：

| 文件 | 作用 |
| --- | --- |
| `go.mod` | Go 项目的依赖清单，新增 WebSocket 库后会变化 |
| `go.sum` | Go 依赖校验文件，执行 `go get` 或 `go mod tidy` 后可能变化 |
| `handler/ws.go` | 新增 WebSocket handler，负责升级连接、收发消息 |
| `router/router.go` | 注册 `/ws` 路由，让客户端能访问到 WebSocket handler |
| `docs/api-overview.md` | 更新接口文档，记录新的 WebSocket 接口 |
| `docs/day19-plan.md` | 今天这份学习教程 |

## 任务 0：确认上一天状态

### 你要做什么

先确认项目目前没有奇怪的未提交代码变更。

### 为什么要做

WebSocket 是新主线的开始。开始前确认工作区干净，可以避免你把 Day 18、Day 19 或内部文档混在一起提交。

### 执行命令

在 PowerShell 执行：

```powershell
cd E:\game-realtime-gm
git status -sb
```

理想情况：

```text
## main...origin/main
?? docs/codex-context.md
?? docs/conversation-handoff-gpt55.md
?? docs/mcp-adoption-plan.md
?? docs/skill-adoption-plan.md
```

如果你看到上面这几个内部文档未跟踪，是正常的。

今天仍然不要提交这些文件：

```text
docs/codex-context.md
docs/conversation-handoff-gpt55.md
docs/mcp-adoption-plan.md
docs/skill-adoption-plan.md
```

### 再确认后端能编译

```powershell
cd E:\game-realtime-gm\backend
go test ./...
```

成功时可能看到：

```text
?       game-realtime-gm/backend/cmd/server     [no test files]
?       game-realtime-gm/backend/internal/auth  [no test files]
...
```

说明：

```text
[no test files] 不是错误。
它表示这个包目前没有测试文件，但代码可以编译通过。
```

## 任务 1：理解今天为什么要引入 WebSocket

### 你要做什么

先理解今天功能的业务位置，不急着写代码。

### 为什么要做

这个项目叫 `game-realtime-gm`，其中 `realtime` 就是“实时”。前面 Day 01 到 Day 18 主要做的是 HTTP API 和 GM 后台能力：

```text
注册
登录
玩家查询
在线心跳
管理员登录
玩家封禁解封
操作日志
dashboard
```

这些多数是“请求一次，返回一次”的 HTTP 接口。

但游戏服务还需要“实时通信”：

```text
玩家 A 移动后，玩家 B 要立刻看到
玩家进入房间后，房间内其他人要立刻收到通知
匹配成功后，服务端要主动通知玩家
```

这类场景如果只用 HTTP 轮询，会比较低效。

### HTTP 和 WebSocket 的区别

HTTP：

```text
客户端发请求
服务端返回响应
一次请求结束
```

WebSocket：

```text
客户端先发起一个 HTTP 请求
服务端同意升级协议
连接保持打开
双方可以持续互相发消息
```

今天先做最小能力：

```text
连接建立
发送欢迎消息
读取客户端消息
回显客户端消息
断开时打印日志
```

后续再加：

```text
token 鉴权
player_id 和连接绑定
心跳
房间
广播
匹配
```

## 任务 2：引入 WebSocket 依赖

### 你要做什么

给 Go 项目安装：

```text
github.com/gorilla/websocket
```

### 为什么要做

Go 标准库有 HTTP 能力，但没有直接提供我们今天要用的高级 WebSocket server 封装。

`gorilla/websocket` 是 Go 里常见的 WebSocket 库，可以帮我们完成：

- HTTP 升级 WebSocket。
- 读取 WebSocket 消息。
- 写入 WebSocket 消息。
- 识别连接断开错误。

### 执行目录

注意必须进入后端模块目录：

```powershell
cd E:\game-realtime-gm\backend
```

### 执行命令

```powershell
go get github.com/gorilla/websocket
```

### 成功后会发生什么

这两个文件可能变化：

```text
E:\game-realtime-gm\backend\go.mod
E:\game-realtime-gm\backend\go.sum
```

你可以检查：

```powershell
git status -sb
```

可能看到：

```text
 M go.mod
 M go.sum
```

### 如果 go get 失败

常见原因：

```text
网络连接 GitHub 或 Go proxy 失败
```

可以稍后重试：

```powershell
go get github.com/gorilla/websocket
```

如果一直失败，先不要乱改 `go.mod`，把完整报错复制出来再处理。

## 任务 3：新建 WebSocket handler 文件

### 你要做什么

新增一个文件：

```text
E:\game-realtime-gm\backend\internal\handler\ws.go
```

操作类型：

```text
新建文件。
如果文件已经存在但为空，直接填入下面完整内容。
如果文件已经存在且有内容，先不要覆盖，先对比是否已经实现过 WebSocket。
```

### 为什么放在 handler 目录

当前项目结构里：

```text
internal/router
```

负责登记路由。

```text
internal/handler
```

负责处理请求和返回响应。

WebSocket 的第一步本质上也是一个请求入口：

```text
客户端访问 /ws
        ↓
router 找到 handler.WebSocketEcho
        ↓
handler 把 HTTP 请求升级成 WebSocket
```

所以今天先放在 `handler` 目录。

后续当连接管理变复杂时，再按项目规范迁移到：

```text
E:\game-realtime-gm\backend\internal\ws\
```

例如后续可能拆成：

```text
internal/ws/hub.go
internal/ws/client.go
internal/ws/message.go
internal/ws/handler.go
```

今天先不拆，避免一开始就复杂化。

### 完整代码

把下面内容粘贴到：

```text
E:\game-realtime-gm\backend\internal\handler\ws.go
```

```go
package handler

import (
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type WSMessage struct {
	Type       string    `json:"type"`
	Content    string    `json:"content,omitempty"`
	ServerTime time.Time `json:"server_time"`
}

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func WebSocketEcho(c *gin.Context) {
	conn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("websocket upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	remoteAddr := conn.RemoteAddr().String()
	log.Printf("websocket connected: %s", remoteAddr)
	defer log.Printf("websocket disconnected: %s", remoteAddr)

	welcome := WSMessage{
		Type:       "welcome",
		Content:    "connected to game realtime server",
		ServerTime: time.Now(),
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

		log.Printf("websocket received from %s: %s", remoteAddr, string(message))

		if err := conn.WriteMessage(messageType, message); err != nil {
			log.Printf("websocket write echo failed: %v", err)
			return
		}
	}
}
```

### 代码分析

#### package

```go
package handler
```

表示这个文件属于 `handler` 包。

它和下面这些文件是同一个包：

```text
backend/internal/handler/auth.go
backend/internal/handler/admin.go
backend/internal/handler/player.go
backend/internal/handler/online.go
```

同一个包里不能有两个同名的类型或函数，所以今天使用：

```go
WebSocketEcho
WSMessage
wsUpgrader
```

这些名字目前不会和已有 handler 冲突。

#### import

```go
import (
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)
```

每个 import 的作用：

| import | 作用 |
| --- | --- |
| `log` | 打印连接建立、断开、错误日志 |
| `net/http` | `CheckOrigin` 需要使用 `*http.Request` |
| `time` | 给欢迎消息加服务端时间 |
| `gin` | 当前项目使用 Gin，handler 参数是 `*gin.Context` |
| `websocket` | 提供 WebSocket 升级、读取、写入能力 |

#### WSMessage

```go
type WSMessage struct {
	Type       string    `json:"type"`
	Content    string    `json:"content,omitempty"`
	ServerTime time.Time `json:"server_time"`
}
```

这个结构体表示服务端主动发给客户端的欢迎消息。

字段说明：

| 字段 | JSON 名 | 作用 |
| --- | --- | --- |
| `Type` | `type` | 消息类型，例如 `welcome` |
| `Content` | `content` | 消息内容 |
| `ServerTime` | `server_time` | 服务端发送消息的时间 |

为什么要有 `type`：

```text
后续 WebSocket 消息会有很多种，比如 join_room、leave_room、chat、match_success。
客户端需要根据 type 判断这条消息是什么意思。
```

#### wsUpgrader

```go
var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}
```

`Upgrader` 的作用：

```text
把普通 HTTP 请求升级成 WebSocket 连接。
```

字段说明：

| 字段 | 作用 |
| --- | --- |
| `ReadBufferSize` | 读取消息时的缓冲区大小 |
| `WriteBufferSize` | 写消息时的缓冲区大小 |
| `CheckOrigin` | 判断是否允许这个来源的页面连接 WebSocket |

今天 `CheckOrigin` 返回 `true`，表示本地学习阶段允许连接。

注意：

```text
正式部署时不能随便 return true。
后续部署到公网后，要限制允许的前端域名。
```

#### WebSocketEcho

```go
func WebSocketEcho(c *gin.Context) {
```

这是今天的核心 handler。

它会被 `router.go` 注册到：

```text
GET /ws
```

#### Upgrade

```go
conn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
```

这行代码把 Gin 的 HTTP 请求升级成 WebSocket。

如果成功：

```text
conn 就代表这个客户端的长连接。
```

如果失败：

```text
说明客户端不是用 WebSocket 方式连接，或者握手请求不正确。
```

#### defer conn.Close

```go
defer conn.Close()
```

作用：

```text
函数结束时关闭 WebSocket 连接。
```

这可以避免连接资源泄漏。

#### 欢迎消息

```go
welcome := WSMessage{
	Type:       "welcome",
	Content:    "connected to game realtime server",
	ServerTime: time.Now(),
}
```

连接成功后，服务端主动发一条消息给客户端。

这能帮助你测试：

```text
连接是否真的建立成功。
```

#### 消息循环

```go
for {
	messageType, message, err := conn.ReadMessage()
	...
	conn.WriteMessage(messageType, message)
}
```

这个循环表示：

```text
只要连接还在，服务端就一直等客户端发消息。
```

收到消息后，服务端用同样的 `messageType` 和 `message` 原样发回去。

这就是“echo 回显”。

今天做 echo 的原因：

```text
它是验证 WebSocket 收发能力的最小闭环。
```

## 任务 4：在 router.go 注册 /ws 路由

### 你要做什么

修改：

```text
E:\game-realtime-gm\backend\internal\router\router.go
```

操作类型：

```text
局部修改。
```

### 搜索关键词

在 `router.go` 里搜索：

```go
r.GET("/health", handler.Health)
```

### 修改前

```go
r.GET("/health", handler.Health)
```

### 修改后

在它下面新增一行：

```go
r.GET("/health", handler.Health)
r.GET("/ws", handler.WebSocketEcho)
```

注意：这两行应该是同一层级，不要把 `/ws` 放进 `api := r.Group("/api")` 下面。

```go
r.GET("/health", handler.Health)
r.GET("/ws", handler.WebSocketEcho)
```

如果你粘贴后发现缩进不齐，不用手动纠结，后面执行 `gofmt` 会自动格式化。

### 为什么 /ws 不放进 /api

今天使用：

```text
/ws
```

而不是：

```text
/api/ws
```

原因：

```text
WebSocket 不是普通 REST API。
它是一个长期连接入口，后续可能会单独接入网关、鉴权、连接管理。
```

当然，真实项目也可以设计成：

```text
/api/ws
/ws/game
/ws/admin
```

但本项目 Day 19 先按最简单方式推进。

### 当前 router.go 的关系

现在项目里大致是：

```text
/health                 健康检查
/ws                     WebSocket 最小连接
/api/register           玩家注册
/api/login              玩家登录
/api/admin/login        管理员登录
/api/admin/...          GM 后台接口
```

## 任务 5：执行 gofmt、go mod tidy、go test

### 你要做什么

格式化代码，整理依赖，确认项目能编译。

### 执行目录

```powershell
cd E:\game-realtime-gm\backend
```

### 执行命令

```powershell
gofmt -w .\internal\handler\ws.go .\internal\router\router.go
go mod tidy
go test ./...
```

### 每条命令的作用

#### gofmt

```powershell
gofmt -w .\internal\handler\ws.go .\internal\router\router.go
```

作用：

```text
自动格式化 Go 代码。
```

Go 项目不要手动争论缩进，统一交给 `gofmt`。

#### go mod tidy

```powershell
go mod tidy
```

作用：

```text
整理 go.mod 和 go.sum。
```

它会确保：

- 用到的依赖被保留。
- 没用到的依赖被移除。
- 校验信息写入 `go.sum`。

#### go test

```powershell
go test ./...
```

作用：

```text
编译并测试所有包。
```

成功时可能看到：

```text
?       game-realtime-gm/backend/internal/handler [no test files]
?       game-realtime-gm/backend/internal/router  [no test files]
```

这表示编译通过。

### 常见错误

#### 1. cannot find package github.com/gorilla/websocket

说明依赖没安装成功。

处理：

```powershell
cd E:\game-realtime-gm\backend
go get github.com/gorilla/websocket
go mod tidy
```

#### 2. undefined: handler.WebSocketEcho

说明：

```text
router.go 已经注册了 WebSocketEcho，但 handler/ws.go 没有正确创建，或者函数名写错。
```

检查：

```text
E:\game-realtime-gm\backend\internal\handler\ws.go
```

确认函数名是：

```go
func WebSocketEcho(c *gin.Context) {
```

#### 3. imported and not used

说明某个 import 没有被使用。

处理：

```powershell
gofmt -w .\internal\handler\ws.go .\internal\router\router.go
go test ./...
```

如果仍然报错，打开报错文件，把未使用的 import 删除。

#### 4. listen tcp :8080: bind

如果启动服务时报：

```text
listen tcp :8080: bind: Only one usage of each socket address is normally permitted.
```

说明 8080 端口已经被占用。

处理方式：

先在 GoLand 或终端里停止之前运行的后端。

也可以在 PowerShell 查端口：

```powershell
netstat -ano | findstr :8080
```

找到 PID 后再判断是否要结束进程。

## 任务 6：启动 Docker Desktop 依赖服务

### 你要做什么

确认 PostgreSQL 和 Redis 容器在运行。

虽然 Day 19 的 `/ws` 最小连接暂时不查数据库和 Redis，但当前后端启动时仍会初始化数据库和 Redis，所以依赖服务要启动。

### 执行命令

```powershell
cd E:\game-realtime-gm\deploy
docker compose up -d
docker ps
```

正常应该看到：

```text
game_realtime_postgres
game_realtime_redis
```

端口应该类似：

```text
0.0.0.0:15432->5432/tcp
0.0.0.0:6379->6379/tcp
```

### 如果 Redis 6379 端口又报错

先不要改代码。

先检查：

```powershell
netsh interface ipv4 show excludedportrange protocol=tcp
```

如果 `6379` 落在排除范围里，需要按之前 Docker 端口问题的处理流程来解决。

## 任务 7：启动后端服务

### 执行目录

```powershell
cd E:\game-realtime-gm\backend
```

### 启动命令

```powershell
go run .\cmd\server
```

成功后应该看到类似：

```text
server listening on :8080
```

不要关闭这个窗口。

WebSocket 测试时，后端必须保持运行。

## 任务 8：用 Apifox 测试 WebSocket

### 你要做什么

用 Apifox 新建一个 WebSocket 请求。

### 为什么不用浏览器直接打开

浏览器地址栏直接输入：

```text
ws://localhost:8080/ws
```

通常不会像 HTTP 页面一样展示结果。

WebSocket 需要专门的客户端工具，例如：

- Apifox WebSocket
- Postman WebSocket
- 浏览器控制台 JavaScript
- wscat

你目前优先用 Apifox。

### Apifox 操作步骤

1. 打开 Apifox。
2. 新建接口。
3. 类型选择 WebSocket。
4. 地址填写：

```text
ws://localhost:8080/ws
```

5. 点击连接。

连接成功后，你应该看到服务端发来的欢迎消息：

```json
{
  "type": "welcome",
  "content": "connected to game realtime server",
  "server_time": "2026-06-22T10:00:00+08:00"
}
```

### 发送测试消息

在 Apifox 发送：

```text
hello websocket
```

正常情况下，会收到同样内容：

```text
hello websocket
```

### 后端终端应该看到什么

后端运行窗口可能会打印：

```text
websocket connected: 127.0.0.1:xxxxx
websocket received from 127.0.0.1:xxxxx: hello websocket
```

断开连接时可能看到：

```text
websocket disconnected: 127.0.0.1:xxxxx
```

## 任务 9：用浏览器控制台测试 WebSocket

这个任务可选。

如果 Apifox 能测试成功，可以先跳过。

### 操作步骤

打开浏览器，按 `F12`，进入 Console，粘贴：

```javascript
const ws = new WebSocket("ws://localhost:8080/ws");

ws.onopen = () => {
  console.log("connected");
  ws.send("hello from browser");
};

ws.onmessage = (event) => {
  console.log("message:", event.data);
};

ws.onclose = () => {
  console.log("closed");
};

ws.onerror = (error) => {
  console.log("error:", error);
};
```

正常会看到：

```text
connected
message: {"type":"welcome",...}
message: hello from browser
```

关闭连接：

```javascript
ws.close();
```

## 任务 10：更新接口文档

### 你要做什么

修改：

```text
E:\game-realtime-gm\docs\api-overview.md
```

操作类型：

```text
局部新增。
```

### 搜索关键词

搜索：

```markdown
## 在线状态模块
```

建议在“在线状态模块”前面新增一个模块：

```markdown
## WebSocket 实时连接模块
```

### 建议新增内容

把下面这一段复制到 `docs/api-overview.md` 的“在线状态模块”前面：

~~~markdown
## WebSocket 实时连接模块

### GET /ws

用途：建立最小 WebSocket 长连接，用于验证实时通信入口。

连接地址：

```text
ws://localhost:8080/ws
```

鉴权：Day 19 暂不需要 token。

连接成功后，服务端会主动发送欢迎消息：

```json
{
  "type": "welcome",
  "content": "connected to game realtime server",
  "server_time": "2026-06-22T10:00:00+08:00"
}
```

客户端发送文本消息后，服务端会原样回显。

说明：

```text
该接口是实时服务主线的最小连接验证。
Day 19 暂不做 JWT 鉴权、玩家连接绑定、房间广播和心跳。
```
~~~

### 为什么今天要更新 api-overview

`docs/api-overview.md` 是项目对外展示的接口总览。

如果代码里有 `/ws`，但接口文档没有，别人看仓库时会不知道这个实时入口已经开始实现。

## 任务 11：数据库和 Redis 对照验证

### 今天是否需要查数据库

Day 19 的 `/ws` 最小接口暂时不写 PostgreSQL。

所以你不需要新增表，也不需要写 SQL。

### 今天是否需要查 Redis

Day 19 的 `/ws` 最小接口也暂时不写 Redis。

所以 Redis 里不会新增 `online:*` key。

### 那为什么还要启动数据库和 Redis

因为当前服务启动流程会初始化：

```text
PostgreSQL
Redis
```

服务依赖能正常连接，后端才能按现有方式启动。

### 可选检查

如果你想确认 Redis 没被今天的 `/ws` 改动，可以执行：

```powershell
docker exec -it game_realtime_redis redis-cli
```

进入后执行：

```text
KEYS online:*
```

如果之前没有在线心跳，可能返回：

```text
(empty array)
```

这正常。

Day 19 不负责写在线状态。

## 任务 12：今日复盘

完成 Day 19 后，用自己的话回答这些问题：

1. WebSocket 和 HTTP 最大区别是什么？
2. 为什么实时游戏服务需要 WebSocket？
3. `wsUpgrader.Upgrade` 做了什么？
4. `conn.ReadMessage()` 是阻塞等待还是立刻返回？
5. 为什么今天先做 echo 回显？
6. 为什么今天不做 JWT 鉴权？
7. `/ws` 请求从哪个文件进入，最终执行哪个函数？
8. 如果连接失败，你会先检查哪些地方？

参考回答方向：

```text
/ws 在 router.go 里注册，指向 handler.WebSocketEcho。
客户端连接时，WebSocketEcho 使用 gorilla/websocket 的 Upgrader 把 HTTP 请求升级成长连接。
升级成功后，服务端先发送 welcome JSON，然后进入 for 循环不断读取客户端消息。
收到消息后，服务端用 WriteMessage 原样回显。
断开或读取失败时，函数 return，并通过 defer 关闭连接。
```

## 今日验收清单

完成后逐项检查：

- [ ] `go get github.com/gorilla/websocket` 执行成功。
- [ ] 新增了 `backend/internal/handler/ws.go`。
- [ ] `router.go` 注册了 `GET /ws`。
- [ ] `gofmt` 执行成功。
- [ ] `go mod tidy` 执行成功。
- [ ] `go test ./...` 通过。
- [ ] Docker Desktop 中 PostgreSQL 和 Redis 容器正常运行。
- [ ] `go run .\cmd\server` 能启动后端。
- [ ] Apifox 能连接 `ws://localhost:8080/ws`。
- [ ] 连接后能收到 welcome 消息。
- [ ] 发送 `hello websocket` 后能收到回显。
- [ ] `docs/api-overview.md` 已记录 `/ws`。
- [ ] 没有提交内部交接文档。

## 常见问题

### 1. Apifox 连接不上 ws://localhost:8080/ws

优先检查：

```text
后端是否正在运行
地址是不是 ws:// 不是 http://
端口是不是 8080
router.go 是否注册了 /ws
go test ./... 是否通过
```

### 2. 连接后没有 welcome 消息

检查：

```text
handler/ws.go 是否有 conn.WriteJSON(welcome)
后端终端是否有 websocket write welcome failed
```

### 3. 发送消息后没有回显

检查：

```text
handler/ws.go 是否有 conn.ReadMessage
handler/ws.go 是否有 conn.WriteMessage(messageType, message)
后端终端是否打印 websocket received
```

### 4. go test 报 undefined: websocket

说明依赖没有进入 `go.mod`。

处理：

```powershell
cd E:\game-realtime-gm\backend
go get github.com/gorilla/websocket
go mod tidy
go test ./...
```

### 5. WebSocket 连接一下就断

可能原因：

- 后端代码执行到错误后 `return`。
- 客户端工具自动断开。
- `WriteJSON(welcome)` 失败。
- 服务被你在 GoLand 里停止了。

先看后端终端日志。

### 6. 为什么没有 Authorization token

这是今天故意不做的。

Day 19 先证明：

```text
项目能建立 WebSocket 长连接。
```

Day 20 或后续再做：

```text
ws://localhost:8080/ws?token=玩家token
```

然后服务端解析 token，把连接和玩家 ID 绑定。

## 今日不要做什么

今天不要做：

- WebSocket JWT 鉴权。
- 玩家 ID 和连接绑定。
- 多玩家广播。
- 房间系统。
- 匹配系统。
- 心跳 ping/pong。
- Redis 保存 WebSocket 在线状态。
- 前端页面。
- WebSocket 压测。

原因：

```text
今天目标只是跑通实时通信入口。
```

如果今天一口气加鉴权、房间、心跳，你很难判断出错时到底是哪一层错了。

正确节奏是：

```text
Day 19：最小连接
Day 20：WebSocket token 鉴权
Day 21：连接和 player_id 绑定
Day 22：心跳和在线状态续期
Day 23：房间内广播
```

## 任务 13：提交并推送到 GitHub

### 先检查状态

```powershell
cd E:\game-realtime-gm
git status
```

确认不要提交这些内部文件：

```text
docs/codex-context.md
docs/conversation-handoff-gpt55.md
docs/mcp-adoption-plan.md
docs/skill-adoption-plan.md
```

### 选择性添加 Day 19 相关文件

如果今天完成了代码和文档修改，执行：

```powershell
git add backend README.md docs/api-overview.md docs/day19-plan.md
```

说明：

- `backend` 包含 `go.mod`、`go.sum`、`handler/ws.go`、`router.go`。
- `docs/day19-plan.md` 是今天的学习文档。
- `docs/api-overview.md` 如果你更新了 `/ws` 文档，就一起提交。
- `README.md` 如果你没有改，可以不加。

不要使用：

```powershell
git add .
```

### 提交

```powershell
git commit -m "Complete day19 websocket echo"
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

如果仍然看到这些未跟踪文件，是正常的：

```text
?? docs/codex-context.md
?? docs/conversation-handoff-gpt55.md
?? docs/mcp-adoption-plan.md
?? docs/skill-adoption-plan.md
```

## 明日预告

Day 20 建议继续 WebSocket 主线：

```text
WebSocket 玩家 token 鉴权
```

目标从：

```text
ws://localhost:8080/ws
```

升级为：

```text
ws://localhost:8080/ws?token=玩家token
```

后端要完成：

```text
读取 token
解析 JWT
确认 player_id
连接成功后返回当前玩家身份
token 错误时拒绝连接
```

这样 WebSocket 就会从“任何人都能连”升级成“登录玩家才能连”。
