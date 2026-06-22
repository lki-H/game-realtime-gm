# Day 20 学习计划：WebSocket 玩家 Token 鉴权

## 你今天从任务几开始

今天从任务 1 开始。

Day 19 已经完成了 WebSocket 最小连接：

```text
ws://localhost:8080/ws
```

Day 20 要把它升级成：

```text
ws://localhost:8080/ws?token=玩家token
```

今天的核心目标是：

```text
只有玩家 token 可以建立 WebSocket 连接。
没有 token、错误 token、管理员 token 都不能连接成功。
```

## 今日目标

今天要完成 WebSocket 玩家身份校验。

最终连接流程变成：

```text
玩家登录拿到 token
        ↓
客户端连接 ws://localhost:8080/ws?token=xxx
        ↓
后端读取 query token
        ↓
调用 auth.ParseToken 校验 JWT
        ↓
确认 subject_type 是 player
        ↓
确认 player_id > 0
        ↓
升级成 WebSocket
        ↓
返回带 player_id 和 username 的 welcome 消息
```

## 今日最终效果

### 1. 不带 token 连接

连接：

```text
ws://localhost:8080/ws
```

结果：

```text
连接失败，HTTP 401
```

### 2. 带管理员 token 连接

连接：

```text
ws://localhost:8080/ws?token=管理员token
```

结果：

```text
连接失败，HTTP 403
```

### 3. 带玩家 token 连接

连接：

```text
ws://localhost:8080/ws?token=玩家token
```

结果：连接成功，并收到欢迎消息：

```json
{
  "type": "welcome",
  "content": "connected to game realtime server",
  "server_time": "2026-06-22T10:00:00+08:00",
  "player_id": 1,
  "username": "player01"
}
```

然后继续保留 Day 19 的回显能力：

```text
客户端发送 hello
服务端回显 hello
```

## 今日会学到什么

你今天会学到：

- WebSocket 握手前如何做鉴权。
- 为什么 WebSocket 不能直接复用普通 HTTP 中间件。
- JWT token 里的 `subject_type` 有什么用。
- 为什么管理员 token 不能当玩家 token 用。
- 为什么不能把完整 token 打印到日志。
- 为什么今天只验证 token，不做连接池、房间和广播。

一句话理解：

```text
Day 19 是“先接通电话”，Day 20 是“接电话前先确认对方是不是玩家”。
```

## 今日文件范围

今天主要涉及：

```text
E:\game-realtime-gm\backend\internal\handler\ws.go
E:\game-realtime-gm\backend\internal\router\router.go
E:\game-realtime-gm\docs\api-overview.md
E:\game-realtime-gm\docs\day20-plan.md
```

文件职责说明：

| 文件 | 作用 |
| --- | --- |
| `handler/ws.go` | WebSocket 连接入口，今天加入玩家 token 校验 |
| `router/router.go` | 注册 `/ws`，今天把 `JWTSecret` 传给 WebSocket handler |
| `docs/api-overview.md` | 更新 `/ws` 接口说明，从“不需要 token”改成“需要玩家 token” |
| `docs/day20-plan.md` | 今天这份学习教程 |

今天不需要新增数据库表。

今天不需要新增 Redis key。

## 任务 0：确认 Day 19 状态

### 你要做什么

先确认 Day 19 的 WebSocket 最小连接代码已经能编译。

### 为什么要做

Day 20 是在 Day 19 上加鉴权。如果 Day 19 本身还没跑通，今天加 token 后会更难排查。

### 执行命令

```powershell
cd E:\game-realtime-gm\backend
gofmt -w .\internal\handler\ws.go .\internal\router\router.go
go test ./...
```

成功时你应该看到所有包通过，可能是：

```text
?       game-realtime-gm/backend/internal/handler [no test files]
?       game-realtime-gm/backend/internal/router  [no test files]
```

如果这里失败，先不要继续 Day 20。

### 特别检查 ws.go

打开：

```text
E:\game-realtime-gm\backend\internal\handler\ws.go
```

确认整个文件里只有一个：

```go
package handler
```

如果出现两个 `package handler`，说明又重复粘贴了。

## 任务 1：理解为什么 WebSocket 需要单独鉴权

### 你要做什么

理解今天为什么不是简单给 `/ws` 加：

```go
protected.Use(middleware.Auth(cfg.JWTSecret))
```

### 为什么不直接复用 HTTP Authorization 头

普通 HTTP 接口现在这样鉴权：

```text
Authorization: Bearer 玩家token
```

但是很多浏览器原生 WebSocket API 不能方便地自定义 `Authorization` 请求头。

例如浏览器里通常这样连接：

```javascript
const ws = new WebSocket("ws://localhost:8080/ws");
```

它不像 `fetch` 那样容易传：

```text
Authorization: Bearer xxx
```

所以今天本地学习阶段使用：

```text
ws://localhost:8080/ws?token=玩家token
```

### 安全提醒

URL query 里带 token 有一个风险：

```text
token 可能出现在浏览器历史、代理日志、网关日志或服务访问日志里。
```

所以今天必须做到：

```text
后端日志不能打印完整 token。
```

本项目当前是本地学习项目，用 query token 帮助你理解 WebSocket 鉴权流程是可以的。

以后如果部署公网，可以进一步优化：

- 使用短期 WebSocket 专用 token。
- 使用 `Sec-WebSocket-Protocol` 携带 token。
- 使用前端先换取一次性连接票据。
- 通过 HTTPS/WSS 部署，避免明文传输。

今天先不做这些复杂方案。

## 任务 2：整文件替换 ws.go

### 你要做什么

修改：

```text
E:\game-realtime-gm\backend\internal\handler\ws.go
```

操作类型：

```text
整文件替换。
```

### 为什么整文件替换

Day 19 的 `ws.go` 文件还很短。

今天要同时改：

- import
- `WSMessage` 字段
- `WebSocketEcho` 函数签名
- 连接前 token 校验
- welcome 消息内容
- 新增 helper 函数

对初学者来说，整文件替换比一点点插入更不容易漏。

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

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type WSMessage struct {
	Type       string    `json:"type"`
	Content    string    `json:"content,omitempty"`
	ServerTime time.Time `json:"server_time"`
	PlayerID   int64     `json:"player_id,omitempty"`
	Username   string    `json:"username,omitempty"`
}

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func WebSocketEcho(jwtSecret string) gin.HandlerFunc {
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

		remoteAddr := conn.RemoteAddr().String()
		log.Printf("websocket connected: player_id=%d username=%s remote=%s", claims.PlayerID, claims.Username, remoteAddr)
		defer log.Printf("websocket disconnected: player_id=%d username=%s remote=%s", claims.PlayerID, claims.Username, remoteAddr)

		welcome := WSMessage{
			Type:       "welcome",
			Content:    "connected to game realtime server",
			ServerTime: time.Now(),
			PlayerID:   claims.PlayerID,
			Username:   claims.Username,
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

## 任务 3：理解 ws.go 代码

### package

```go
package handler
```

表示这个文件属于 `handler` 包。

它和 `auth.go`、`player.go`、`admin.go` 是同一个包。

同一个包里不能出现重复函数名，所以今天新增的 helper 叫：

```go
websocketPlayerClaims
```

这个名字目前不会和其他文件冲突。

### import

新增了两个重点 import：

```go
"strings"

tokenauth "game-realtime-gm/backend/internal/auth"
```

`strings` 用来处理 query token 前后的空格。

`tokenauth` 是给 `internal/auth` 包起别名，避免和标准概念里的 auth 混淆。

今天复用已有函数：

```go
tokenauth.ParseToken(jwtSecret, tokenString)
```

这比重新写一套 JWT 解析逻辑更好。

### WSMessage

Day 19 的消息只有：

```go
Type
Content
ServerTime
```

Day 20 加了：

```go
PlayerID
Username
```

作用是让客户端确认：

```text
当前 WebSocket 连接属于哪个玩家。
```

### WebSocketEcho 函数签名变化

Day 19：

```go
func WebSocketEcho(c *gin.Context)
```

Day 20：

```go
func WebSocketEcho(jwtSecret string) gin.HandlerFunc
```

为什么要这样改？

因为解析 JWT 必须知道：

```text
JWT_SECRET
```

`JWT_SECRET` 在项目配置里：

```text
cfg.JWTSecret
```

`handler/ws.go` 自己拿不到 `cfg`，所以由 `router.go` 注册路由时传进来。

### 连接前先校验 token

```go
claims, ok := websocketPlayerClaims(c, jwtSecret)
if !ok {
	return
}
```

注意它放在：

```go
wsUpgrader.Upgrade(...)
```

之前。

原因：

```text
如果 token 不合法，就不要升级成 WebSocket。
```

这样失败时仍然可以返回普通 HTTP 状态码：

```text
401
403
```

### 不打印完整 token

日志里只打印：

```go
player_id
username
remote
```

不打印：

```text
token
JWT_SECRET
```

这是安全习惯。

### websocketPlayerClaims

这个 helper 做三件事：

1. 从 query 里取 token。
2. 调用已有 `ParseToken`。
3. 确认 token 是玩家 token。

判断玩家 token 的关键代码：

```go
if claims.SubjectType != tokenauth.SubjectTypePlayer || claims.PlayerID <= 0 {
```

这可以拦住管理员 token。

管理员 token 里是：

```text
subject_type = admin
admin_id > 0
```

玩家 token 里是：

```text
subject_type = player
player_id > 0
```

这就是为什么 Day 20 要测管理员 token。

## 任务 4：修改 router.go

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

搜索：

```go
r.GET("/ws", handler.WebSocketEcho)
```

### 修改前

```go
r.GET("/health", handler.Health)
r.GET("/ws", handler.WebSocketEcho)
```

### 修改后

```go
r.GET("/health", handler.Health)
r.GET("/ws", handler.WebSocketEcho(cfg.JWTSecret))
```

### 为什么这样改

Day 20 的 `WebSocketEcho` 需要 `JWTSecret` 才能解析 token。

`router.New` 函数已经有：

```go
cfg config.Config
```

所以这里可以拿到：

```go
cfg.JWTSecret
```

这条链路是：

```text
cmd/server/main.go
        ↓
config.Load()
        ↓
router.New(db, redisClient, cfg)
        ↓
handler.WebSocketEcho(cfg.JWTSecret)
        ↓
auth.ParseToken(jwtSecret, token)
```

## 任务 5：格式化和编译验证

### 执行目录

```powershell
cd E:\game-realtime-gm\backend
```

### 执行命令

```powershell
gofmt -w .\internal\handler\ws.go .\internal\router\router.go
go test ./...
```

### 成功结果

看到类似：

```text
?       game-realtime-gm/backend/internal/handler [no test files]
?       game-realtime-gm/backend/internal/router  [no test files]
```

说明编译通过。

### 常见错误

#### 1. cannot use handler.WebSocketEcho as gin.HandlerFunc

说明 `router.go` 还停留在 Day 19 写法。

检查是否改成：

```go
r.GET("/ws", handler.WebSocketEcho(cfg.JWTSecret))
```

#### 2. undefined: tokenauth

说明 `ws.go` import 没有加：

```go
tokenauth "game-realtime-gm/backend/internal/auth"
```

#### 3. imported and not used: strings

说明代码里没有使用：

```go
strings.TrimSpace
```

对照完整 `ws.go` 检查是否漏粘了 `websocketPlayerClaims`。

#### 4. expected declaration, found package

说明 `ws.go` 又重复粘贴了两份 `package handler`。

整个文件只能有一个：

```go
package handler
```

## 任务 6：启动 Docker Desktop 依赖

### 为什么 Day 20 仍然需要 Docker

今天 WebSocket 鉴权需要先通过玩家登录拿 token。

玩家登录会查询 PostgreSQL：

```text
players 表
```

所以 PostgreSQL 必须启动。

后端启动时也会初始化 Redis，所以 Redis 也要启动。

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

## 任务 7：启动后端服务

### 执行命令

```powershell
cd E:\game-realtime-gm\backend
go run .\cmd\server
```

成功后不要关闭这个窗口。

如果看到：

```text
listen tcp :8080: bind
```

说明 8080 被占用。

先停掉 GoLand 或另一个 PowerShell 里正在运行的后端。

## 任务 8：获取玩家 token

### 你要做什么

今天测试 WebSocket 需要玩家 token。

### 如果你已经有玩家账号

用 Apifox 请求：

```http
POST http://localhost:8080/api/login
Content-Type: application/json

{
  "username": "player01",
  "password": "123456"
}
```

响应里复制：

```json
{
  "data": {
    "token": "这里就是玩家token"
  }
}
```

### 如果你没有玩家账号

先注册：

```http
POST http://localhost:8080/api/register
Content-Type: application/json

{
  "username": "player01",
  "password": "123456",
  "nickname": "玩家01"
}
```

再登录：

```http
POST http://localhost:8080/api/login
Content-Type: application/json

{
  "username": "player01",
  "password": "123456"
}
```

### 注意

复制 token 时不要带双引号。

正确：

```text
eyJhbGciOiJIUzI1NiIs...
```

错误：

```text
"eyJhbGciOiJIUzI1NiIs..."
```

## 任务 9：测试不带 token 的 WebSocket

### 请求地址

用 Apifox WebSocket 连接：

```text
ws://localhost:8080/ws
```

### 预期结果

应该连接失败。

可能显示：

```text
Unexpected server response: 401
```

或能看到响应：

```json
{
  "code": 40121,
  "message": "websocket token missing"
}
```

### 为什么这样才对

Day 20 以后 `/ws` 不再允许匿名连接。

如果不带 token 还能连接成功，说明鉴权没有生效。

## 任务 10：测试管理员 token 不能连接

### 先获取管理员 token

请求：

```http
POST http://localhost:8080/api/admin/login
Content-Type: application/json

{
  "username": "admin",
  "password": "admin123456"
}
```

复制响应里的管理员 token。

### WebSocket 连接

```text
ws://localhost:8080/ws?token=管理员token
```

### 预期结果

应该连接失败，HTTP 403。

可能看到：

```json
{
  "code": 40331,
  "message": "player token required"
}
```

### 为什么管理员 token 不能连

因为这个 `/ws` 是玩家实时连接入口，不是 GM 后台连接入口。

当前规则：

```text
玩家 token：可以连接 /ws
管理员 token：不可以连接 /ws
```

后续如果要做 GM 后台实时监控，可以另开：

```text
/admin/ws
```

但今天不做。

## 任务 11：测试玩家 token 可以连接

### 请求地址

把 `玩家token` 替换成你登录拿到的 token：

```text
ws://localhost:8080/ws?token=玩家token
```

### 预期结果

连接成功，并收到：

```json
{
  "type": "welcome",
  "content": "connected to game realtime server",
  "server_time": "2026-06-22T10:00:00+08:00",
  "player_id": 1,
  "username": "player01"
}
```

### 继续测试回显

发送：

```text
hello day20
```

应该收到：

```text
hello day20
```

### 后端日志应该类似

```text
websocket connected: player_id=1 username=player01 remote=127.0.0.1:xxxxx
websocket received from player_id=1: hello day20
```

注意日志里不应该出现完整 token。

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

把 Day 19 的：

```text
鉴权：Day 19 暂不需要 token。
```

改成：

```text
鉴权：需要玩家 token，通过 query 参数传入。
```

连接地址从：

```text
ws://localhost:8080/ws
```

改成：

```text
ws://localhost:8080/ws?token=玩家token
```

welcome 响应示例增加：

```json
{
  "type": "welcome",
  "content": "connected to game realtime server",
  "server_time": "2026-06-22T10:00:00+08:00",
  "player_id": 1,
  "username": "player01"
}
```

新增主要错误：

```text
40121 websocket token missing
40122 invalid websocket token
40331 player token required
```

### 为什么要更新接口文档

Day 20 以后 `/ws` 行为变了。

Day 19：

```text
不需要 token
```

Day 20：

```text
必须带玩家 token
```

如果文档不更新，以后你自己测试也会被旧文档误导。

## 任务 13：今日复盘

完成后用自己的话回答：

1. 为什么 WebSocket 不能直接照搬普通 HTTP Authorization 头？
2. 为什么 token 校验要放在 `Upgrade` 之前？
3. `auth.ParseToken` 返回的 `Claims` 里有哪些重要字段？
4. `SubjectTypePlayer` 是怎么拦住管理员 token 的？
5. 为什么日志里不能打印完整 token？
6. 为什么今天不查数据库确认玩家是否被封禁？
7. `/ws?token=xxx` 从请求进入到连接成功，中间经过哪些代码？

参考表达：

```text
Day 20 我给 WebSocket 入口增加了玩家 token 鉴权。
客户端连接 /ws 时需要在 query 参数里携带 token。
后端在升级 WebSocket 之前先调用 auth.ParseToken 校验 JWT，再检查 subject_type 必须是 player 且 player_id 大于 0。
如果 token 缺失或无效，直接返回 401；如果是管理员 token，返回 403。
校验成功后才升级连接，并在 welcome 消息中返回 player_id 和 username。
```

## 今日验收清单

- [ ] `ws.go` 已整文件替换。
- [ ] `router.go` 已改成 `handler.WebSocketEcho(cfg.JWTSecret)`。
- [ ] `gofmt` 执行成功。
- [ ] `go test ./...` 通过。
- [ ] Docker Desktop 中 PostgreSQL 和 Redis 正常运行。
- [ ] 后端服务能启动。
- [ ] 不带 token 连接 `/ws` 会失败。
- [ ] 管理员 token 连接 `/ws` 会失败。
- [ ] 玩家 token 连接 `/ws` 会成功。
- [ ] welcome 消息包含 `player_id` 和 `username`。
- [ ] 发送文本消息仍然可以回显。
- [ ] 后端日志没有打印完整 token。
- [ ] `docs/api-overview.md` 已更新。

## 常见问题

### 1. no token 时 Apifox 只显示连接失败，看不到 JSON

这是正常的。

WebSocket 客户端在握手失败时，不一定展示后端返回的 JSON body。

重点看状态码是不是：

```text
401
```

### 2. 玩家 token 连接也失败

优先检查：

```text
token 有没有复制完整
URL 里有没有多余双引号
token 有没有过期
是不是误用了管理员 token
后端 JWT_SECRET 是否和签发 token 时一致
```

### 3. 管理员 token 居然能连接

检查 `ws.go` 是否有这段：

```go
if claims.SubjectType != tokenauth.SubjectTypePlayer || claims.PlayerID <= 0 {
```

如果没有，说明漏了权限判断。

### 4. go test 报 undefined: handler.WebSocketEcho

检查 `ws.go` 函数名是否是：

```go
func WebSocketEcho(jwtSecret string) gin.HandlerFunc
```

不是：

```go
func WebsocketEcho(...)
func WebSocketHandler(...)
```

Go 大小写敏感。

### 5. go test 报 cannot use handler.WebSocketEcho

检查 `router.go` 是否写成：

```go
r.GET("/ws", handler.WebSocketEcho(cfg.JWTSecret))
```

不是：

```go
r.GET("/ws", handler.WebSocketEcho)
```

### 6. 日志里出现了完整 token

立刻删掉相关日志。

不应该写：

```go
log.Printf("token=%s", tokenString)
```

可以写：

```go
log.Printf("websocket connected: player_id=%d username=%s", claims.PlayerID, claims.Username)
```

## 今日不要做什么

今天不要做：

- WebSocket 连接池。
- `player_id -> connection` 映射。
- Redis 在线状态续期。
- ping/pong 心跳。
- 房间系统。
- 多人广播。
- 匹配系统。
- GM 后台 WebSocket。
- WebSocket 压测。

原因：

```text
今天只解决“谁能连接”的问题。
```

如果今天同时做连接管理和心跳，出错时你会分不清是 token 问题、连接问题、并发问题还是 Redis 问题。

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

### 选择性添加 Day 20 相关文件

```powershell
git add backend README.md docs/api-overview.md docs/day20-plan.md
```

如果 `README.md` 没有修改，可以不加。

不要使用：

```powershell
git add .
```

### 提交

```powershell
git commit -m "Complete day20 websocket auth"
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

Day 21 建议继续 WebSocket 主线：

```text
WebSocket 连接与 player_id 绑定
```

目标：

```text
玩家连接成功后，把 player_id 和连接关系记录到内存管理器里。
玩家断开时，从管理器移除。
```

这会为后续能力做准备：

```text
查看当前在线连接数
给指定玩家推送消息
房间广播
匹配成功通知
```
