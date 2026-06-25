# 项目全周期技术规范与架构参考

## 文档定位

这份文档是 `game-realtime-gm` 项目的长期技术规范。

它有两个作用：

1. 给学习者参考：你写代码、改目录、设计接口、排查问题时，优先看这里。
2. 给 AI 助手参考：以后生成 `dayxx-plan.md` 时，必须遵守这里的项目结构、命名、代码组织和学习节奏。

这份文档会贯穿整个项目周期，而不是某一天的任务计划。

## GitHub 日常提交规范

本项目已经上传到 GitHub：

```text
https://github.com/lki01/game-realtime-gm.git
```

当前主分支：

```text
main
```

以后每次生成 `docs/day/dayXX-plan.md` 时，文档末尾必须包含 GitHub 收尾流程。

默认节奏：

```text
当天任务完成
        ↓
go test ./... 通过
        ↓
git status 检查文件
        ↓
选择性 git add
        ↓
git commit
        ↓
git push
```

不要无脑使用：

```powershell
git add .
```

原因是项目里可能存在不适合上传 GitHub 的内部协作文档或隐私文件。

每次提交前必须检查不要上传：

```text
.env
.env.*
*.pem
*.key
*.dump
*.sql.gz
*.log
真实手机号
真实邮箱
真实数据库密码
真实 JWT_SECRET
Docker volume 数据
GoLand 临时文件
```

本项目默认不建议上传：

```text
docs/codex-context.md
docs/conversation-handoff-gpt55.md
docs/mcp-adoption-plan.md
docs/skill-adoption-plan.md
```

旧的 `claude.md` 已整合进项目根目录 `AGENTS.md`，后续不再单独维护。

完整流程见：

```text
docs/github-workflow.md
```

## 参考项目与参考方式

这些项目和平台不要求照抄，只用于观察成熟项目如何组织工程、命名业务对象、表达多人 PVE 后端链路。

参考时分成两类：

```text
开源架构参考：看代码结构、抽象方式、实时连接和房间/匹配模型。
真实业务模型参考：看商业游戏后台平台如何命名 Party、Session、Matchmaking、Game Session。
```

### 开源架构参考

| 项目 | 适合学习 | 本项目怎么吸收 | 当前不要照抄 |
| --- | --- | --- | --- |
| [Nakama](https://github.com/heroiclabs/nakama) / [docs](https://heroiclabs.com/docs/nakama/) | 账号、实时连接、队伍、匹配、排行榜、后台控制台 | 学习游戏后端功能边界，理解 Socket、Party、Matchmaker、Leaderboard、Console 如何组合 | 不照抄完整平台架构、多语言 runtime、复杂 API 网关 |
| [Pitaya](https://github.com/topfreegames/pitaya) / [docs](https://pitaya.readthedocs.io/) | Go 游戏服务器、TCP/WebSocket、Group 广播、Session、Route、Push | 学习 Group 如何表达小队/房间广播，学习 Session 绑定玩家和服务端主动推送 | 不引入 etcd、NATS、RPC 集群、frontend/backend server 拆分 |
| [Colyseus](https://github.com/colyseus/colyseus) / [docs](https://docs.colyseus.io/) | Room 生命周期、房间状态、状态同步、匹配 | 学习 Room、onCreate、onJoin、onLeave、onDispose、maxClients 思路 | 不换成 Node/TypeScript，不做完整状态同步框架 |
| [Open Match](https://github.com/googleforgames/open-match) / [docs](https://open-match.dev/site/docs/) | 匹配票据、匹配池、匹配函数、Director、Assignment | 学习“匹配流程模型”，将 Ticket、Pool、MatchProfile 简化成 Redis 匹配队列 | 不部署 Open Match，不拆 Director/Evaluator/MatchFunction 服务 |
| [Agones](https://github.com/googleforgames/agones) / [docs](https://agones.dev/site/docs/) | GameServer 生命周期、分配、玩家容量、Fleet | 学习“任务副本生命周期”和 PlayerCapacity 概念 | 不上 Kubernetes，不使用 CRD、Fleet、GameServerAllocation |
| [Gin-Vue-Admin](https://gin-vue-admin.com/guide/introduce/project.html) | GM 后台、JWT、RBAC、菜单、API 权限、Swagger、Redis | 学习 GM 后台展示、权限组织和接口管理思路 | 不照搬大后台框架，不引入不理解的代码生成体系 |
| [go-clean-template](https://github.com/evrone/go-clean-template) | Go 项目分层、配置、测试、Docker、工程结构 | 学习目录职责、配置隔离、测试和 Docker 组织方式 | 不过早上复杂 Clean Architecture |
| [go-zero](https://github.com/zeromicro/go-zero) | 工程化、配置、超时、限流、监控、服务治理 | 后期学习工程实践和服务治理概念 | 当前阶段不引入微服务框架 |
| [go-redis](https://github.com/redis/go-redis) | Redis Go 客户端、连接、命令、连接池 | 本项目 Redis 客户端选型和用法参考 | 不把 Redis 当长期可靠数据库 |
| [gin-gonic/gin](https://github.com/gin-gonic/gin) | Gin 路由、中间件、JSON、请求绑定 | 本项目 HTTP API 框架基础 | 不为简单接口引入复杂框架封装 |

### 真实业务模型参考

这些平台更接近真实工作中的游戏后台业务语言。当前只学习业务模型，不引入它们的 SDK、云服务或部署体系。

| 平台/项目 | 适合学习 | 本项目怎么吸收 | 当前不要照抄 |
| --- | --- | --- | --- |
| [AccelByte Gaming Services](https://docs.accelbyte.io/gaming-services/modules/multiplayer/parties-presence/) | Party、Presence、Session、Matchmaking、Dedicated Server Manager | 学习“小队 -> 匹配 -> 会话/任务副本 -> 分配服务器”的完整业务链 | 不接入商业平台，不复制复杂跨平台账号和会话体系 |
| [PlayFab Multiplayer](https://learn.microsoft.com/en-us/gaming/playfab/multiplayer/matchmaking/) | Matchmaking Queue、Match Size、Server Allocation | 学习匹配队列、最小/最大人数、匹配后分配服务器的表达方式 | 不接入 PlayFab，不做云服务器自动分配 |
| [AWS GameLift FlexMatch](https://docs.aws.amazon.com/gameliftservers/latest/flexmatchguide/match-intro.html) | 匹配规则、Game Session、Player Session、延迟/属性匹配 | 学习成熟匹配系统如何描述规则和玩家会话 | 不上 AWS GameLift，不做复杂规则引擎 |
| [Epic Online Services](https://dev.epicgames.com/docs/game-services/lobbies-and-sessions) | Lobby、Session、Presence、跨平台联机概念 | 学习 Lobby 和 Session 的区别 | 不接入 EOS，不做跨平台账号体系 |
| [Hathora](https://hathora.dev/docs) | Room、server process、区域部署、进程生命周期 | 学习“一个房间/任务实例对应一个服务进程”的部署概念 | 当前不拆独立战斗服进程 |
| [Pragma Engine](https://pragma.gg/) | Player Data、Party、Matchmaking、Game Instance | 学习玩家数据、队伍、游戏实例之间的业务关系 | 不照搬商业后端平台架构 |
| [Centrifugo](https://centrifugal.dev/) / [presence](https://centrifugal.dev/docs/server/presence) | WebSocket、Channel、Presence、Pub/Sub、实时推送 | 学习 `channel -> squad:<id>`、presence、join/leave、小队广播模型 | 当前不引入 Centrifugo 服务 |
| [Casbin](https://casbin.org/docs/overview) | RBAC/ABAC 权限模型 | 后期优化 GM 权限，不只依赖简单 role 字符串 | 当前不急着引入复杂权限策略语言 |
| [Prometheus Go Client](https://prometheus.io/docs/guides/go-application/) | Go 指标暴露、连接数、接口耗时 | Day 46 以后为在线人数、WebSocket 连接数、接口耗时加 `/metrics` | 当前不提前做完整监控体系 |
| [OpenTelemetry Go](https://opentelemetry.io/docs/languages/go/) | tracing、请求链路、Gin/DB/Redis 调用观察 | 后期理解请求从 router 到 DB/Redis 的链路 | 当前不引入复杂 trace 基建 |

### 参考原则

- 只吸收和当前阶段有关的结构，不提前照搬大型框架。
- 先做单体服务，后期再理解微服务、服务发现、网关、集群。
- 参考项目优先用于“命名、边界、数据流、状态流转”，不要直接搬代码。
- 商业平台文档优先用于理解真实工作里的业务语言，例如 Party、Session、Matchmaking、Game Session。
- 优先保持项目可运行、可解释、可演示。

### 概念映射到本项目

| 外部概念 | 本项目当前/后续概念 |
| --- | --- |
| Party / Lobby / Group | 小队房间 `squad` |
| Presence | 在线状态、连接状态 |
| Channel | 小队广播频道 `squad:<squad_id>` |
| Ticket | 匹配票据 `matchmaking_ticket` |
| Matchmaking Queue / Pool | PVE 任务匹配队列 |
| Match / Game Session / Room | 任务副本实例 `mission_instance` |
| Player Session | 玩家在任务副本内的参与记录 |
| GameServer | 二期战斗服/任务服概念，当前只用文档理解 |
| Fleet / Allocation | 二期部署和调度概念，当前不实现 |

### 技术文档映射规则

以后把参考项目或新业务方向写进文档时，按下面优先级处理：

| 文档 | 是否必须同步 | 同步内容 |
| --- | --- | --- |
| `README.md` | 必须 | 项目定位、当前进度、对外展示口径 |
| `docs/project-direction-pve.md` | 必须 | 业务方向、参考项目、阶段边界、面试表达 |
| `docs/project-technical-standards.md` | 必须 | 命名规范、目录规范、接口规范、Redis/PostgreSQL 规则 |
| `docs/api-overview.md` | 按功能同步 | 只记录已经实现或正在当天验证的接口行为 |
| `docs/learning-roadmap.md` | 按阶段同步 | 学习路线、阶段目标、需要补的基础知识 |
| `docs/day/dayXX-plan.md` | 必须 | 从新增 Day 开始按最新方向生成，不回头重写旧 Day |
| 旧 Day 文档 | 不主动重写 | 作为学习轨迹保留，除非内容会明显误导当前开发 |
| 内部交接文档 | 不上传、不主动重写 | 只用于对话续接和协作上下文 |

判断标准：

```text
会影响以后怎么写代码、怎么测试接口、怎么给 HR 解释项目的文档，要同步。
只记录历史学习过程、内部上下文或旧对话压缩的文档，不要为了统一口径全部改掉。
```

## 当前项目技术定位

项目名称：

```text
Go 实时游戏服务与 GM 运营后台
```

当前阶段是学习型单体后端项目。

项目目标不是直接做成大型商业游戏服务器，而是逐步掌握：

- Go 后端项目结构
- Gin HTTP API
- PostgreSQL 持久化
- Redis 实时状态
- JWT 鉴权
- WebSocket 长连接
- 共斗 PVE 小队房间
- PVE 任务匹配队列
- 任务副本生命周期
- 任务结算与排行榜
- GM 后台
- Docker Compose
- 测试、日志、压测、pprof

后续业务语义优先向“共斗 PVE 游戏后台与实时服务基础”收束。项目仍然保持普通 Go 后端工程能力为底座，不在当前阶段强行引入 UDP/KCP/QUIC、战斗服拆分、跨服、滚服或灰度发布。

## 总体架构原则

### 1. 先单体，后拆分

当前不做微服务。

正确路线：

```text
单体 Gin 服务
        ↓
模块清晰
        ↓
能测试、能演示
        ↓
后期再理解服务拆分
```

不要提前引入：

- 微服务
- Kubernetes
- 服务网格
- 消息队列
- 分布式锁
- 复杂 CQRS

除非当前功能真的需要。

### 2. 一条请求链路必须能讲清楚

每个接口都要能画出：

```text
客户端 / Apifox
        ↓
router
        ↓
middleware
        ↓
handler
        ↓
database / redis
        ↓
JSON response
```

如果一个功能讲不清楚链路，就先不要继续堆功能。

### 3. 目录结构随着复杂度自然演进

不要一开始就做很深的 Clean Architecture。

但是也不能所有代码都塞进一个文件。

当前推荐结构：

```text
backend/
├── cmd/
│   └── server/              程序入口
├── internal/
│   ├── auth/                JWT 生成、解析等认证工具
│   ├── cache/               Redis 初始化
│   ├── config/              配置读取
│   ├── database/            PostgreSQL 初始化、schema.sql
│   ├── handler/             HTTP handler
│   ├── middleware/          Gin 中间件
│   ├── model/               数据模型
│   ├── router/              路由注册
│   └── service/             后期业务逻辑层，可按需要新增
├── go.mod
└── go.sum
```

当前阶段可以先让 `handler` 直接访问数据库。

当某个 handler 变复杂，或多个 handler 共享同一段业务逻辑时，再提取 `service`。

## Go 包与文件规范

### 1. 同一目录就是同一个包

Go 的包按目录组织。

例如：

```text
backend/internal/handler/
├── auth.go
├── online.go
└── player.go
```

如果这三个文件开头都是：

```go
package handler
```

那么它们属于同一个包。

同一个包里：

- 可以互相调用小写函数
- 不能有两个同名函数
- 不能有两个同名类型
- import 必须各文件自己写

### 2. 避免同包函数重名

错误示例：

```go
// online.go
func requireCurrentPlayerID(c *gin.Context) (int64, bool) { ... }

// player.go
func requireCurrentPlayerID(c *gin.Context) (int64, bool) { ... }
```

这会报：

```text
redeclared in this block
```

因为它们都在 `handler` 包。

### 3. helper 函数放置规则

如果 helper 只被一个文件使用：

```text
放在当前文件底部
```

如果 helper 被同一个包里的多个文件使用：

```text
优先放到同包的 helper 文件
```

例如：

```text
backend/internal/handler/context.go
```

放：

```go
func requireCurrentPlayerID(c *gin.Context) (int64, bool) { ... }
```

如果 helper 已经属于通用中间件能力：

```text
放到 middleware 包
```

例如：

```go
middleware.CurrentPlayerID(c)
```

### 4. 是否新建文件的判断

不要因为一个小函数就立刻建很多文件。

判断标准：

| 情况 | 推荐做法 |
| --- | --- |
| 只被当前文件使用 | 放当前文件底部 |
| 被同包 2 个以上文件使用 | 放同包 `context.go`、`helper.go` 或更明确的文件 |
| 被多个包使用 | 放到职责更明确的包，例如 `middleware`、`auth`、`model` |
| 逻辑已经涉及业务规则 | 考虑提到 `service` |

当前项目推荐：

```text
handler 内部共享当前玩家 ID：
handler/context.go

JWT 生成解析：
internal/auth/jwt.go

Gin 鉴权中间件：
internal/middleware/auth.go
```

### 5. import 规范

每次删除代码后，要检查 import。

Go 不允许未使用的 import。

如果看到：

```text
imported and not used
```

说明某个 import 已经没用了，要删掉。

每次改完 Go 文件，执行：

```powershell
gofmt -w 文件路径
go test ./...
```

## 分层职责规范

### cmd/server

职责：

- 加载配置
- 初始化 PostgreSQL
- 初始化 Redis
- 创建路由
- 启动 HTTP 服务

不要放：

- 注册逻辑
- 登录逻辑
- SQL 查询细节
- Redis key 业务逻辑

### config

职责：

- 从环境变量读取配置
- 提供默认值
- 管理数据库、Redis、JWT、服务端口等配置

不要放：

- 数据库连接代码
- Redis 操作代码
- 业务逻辑

### database

职责：

- 创建 PostgreSQL 连接池
- Ping 数据库
- 保存 schema.sql
- 后期可管理 migration

不要放：

- HTTP handler
- 玩家注册业务
- Redis 逻辑

### cache

职责：

- 创建 Redis 客户端
- Ping Redis

不要放：

- 在线状态 key 的业务规则
- 排行榜业务逻辑

这些应该放 handler 或 service。

### auth

职责：

- JWT 生成
- JWT 解析
- Claims 定义

不要放：

- Gin 中间件响应 JSON
- 数据库查询玩家

### middleware

职责：

- Gin 请求前置处理
- JWT 鉴权
- 把 player_id 写入 gin.Context
- 后期可放日志、CORS、限流

不要放：

- 具体业务查询
- SQL
- Redis SET/GET

### handler

职责：

- 解析 HTTP 请求
- 调用数据库或 Redis
- 返回 JSON
- 处理 HTTP 状态码

当前学习阶段允许 handler 直接访问 DB/Redis。

后期如果 handler 太长，再引入：

```text
internal/service/
```

### model

职责：

- 定义业务数据结构
- 对应数据库表或接口响应中的核心对象

不要放：

- 查询数据库的方法
- 业务处理函数

## 接口设计规范

### URL 命名

使用资源名复数：

```text
/api/players
/api/squads
/api/missions
```

当前用户使用：

```text
/api/me
```

动作类接口可以使用明确动词：

```text
/api/online/heartbeat
/api/matchmaking/join
/api/matchmaking/leave
```

### HTTP 方法

| 方法 | 用途 | 示例 |
| --- | --- | --- |
| GET | 查询 | `GET /api/me` |
| POST | 创建或动作 | `POST /api/register` |
| PATCH | 局部更新 | `PATCH /api/me/nickname` |
| DELETE | 删除或退出 | `DELETE /api/squads/:id/players/me` |

### 响应格式

统一使用：

```json
{
  "code": 0,
  "message": "ok",
  "data": {}
}
```

失败时：

```json
{
  "code": 40001,
  "message": "invalid request"
}
```

### HTTP 状态码

| 状态码 | 场景 |
| --- | --- |
| 200 | 查询成功、更新成功 |
| 201 | 创建成功，例如注册 |
| 400 | 请求参数错误 |
| 401 | 未登录、token 错误 |
| 403 | 已登录但无权限 |
| 404 | 资源不存在 |
| 409 | 冲突，例如用户名重复 |
| 500 | 服务端内部错误 |

### 错误信息规范

登录失败不要区分：

```text
用户名不存在
密码错误
```

统一返回：

```text
username or password is wrong
```

原因：避免账号枚举。

## 数据库规范

### PostgreSQL 用途

PostgreSQL 保存长期可靠数据：

- 玩家账号
- 玩家资料
- GM 管理员
- 小队房间记录
- 任务副本记录
- 任务结算记录
- 战绩
- 操作日志

### SQL 写法

必须使用参数化 SQL。

正确：

```sql
WHERE username = $1
```

错误：

```go
"WHERE username = '" + username + "'"
```

原因：字符串拼接 SQL 有 SQL 注入风险。

### 表字段规范

每张业务表默认考虑：

```text
id
created_at
updated_at
```

账号表密码字段必须是：

```text
password_hash
```

不要使用：

```text
password
```

数据库中不能保存明文密码。

### schema.sql 规范

数据库里手动执行过的建表 SQL，必须同步写入：

```text
backend/internal/database/schema.sql
```

原因：

- 方便重建数据库
- 方便别人运行项目
- 方便写 README
- 方便后期迁移到 migration 工具

## Redis 规范

### Redis 用途

Redis 保存高频、临时、实时状态：

- 玩家在线状态
- WebSocket 连接状态
- 匹配队列
- 小队临时状态
- 任务副本临时状态
- 排行榜
- 验证码
- 短期 token 黑名单

不要把长期可靠数据只存在 Redis。

### key 命名

使用冒号分层：

```text
online:player:<player_id>
matchmaking:queue:<mode>
squad:<squad_id>:state
mission:<mission_id>:state
rank:score
```

### TTL 规范

在线状态必须设置 TTL。

正确：

```text
SET online:player:1 1 EX 120
```

错误：

```text
SET online:player:1 1
```

原因：没有 TTL 时，玩家可能永远显示在线。

### Redis 命令学习阶段

本地学习可以用：

```text
KEYS online:*
```

但线上不要随便用 `KEYS`。

线上使用：

```text
SCAN
```

## JWT 与鉴权规范

### JWT Claims

当前 token 至少包含：

```text
player_id
username
exp
iat
sub
```

### Authorization 头格式

必须使用：

```text
Authorization: Bearer <token>
```

### 中间件职责

JWT 中间件只做：

- 读取 Authorization
- 解析 token
- 校验 token
- 把 player_id 写入 gin.Context
- 失败时返回 401

不做：

- 查询数据库
- 执行业务逻辑
- 判断 GM 权限

GM 权限后期单独做 RBAC。

## WebSocket 规划规范

项目已经进入 WebSocket 实时服务阶段。

后续结构建议：

```text
internal/ws/
├── hub.go          连接管理
├── client.go       单个连接
├── message.go      消息格式
└── handler.go      升级 HTTP 到 WebSocket
```

WebSocket 连接建立流程：

```text
客户端带 token
        ↓
后端校验 token
        ↓
建立连接
        ↓
记录 player_id -> connection
        ↓
心跳续期 Redis 在线状态
```

不要在一开始就做复杂分布式网关。

## 共斗 PVE 小队与匹配规划规范

后期小队房间和任务匹配模块建议：

```text
internal/squad/
internal/matchmaking/
internal/mission/
```

小队房间状态可以先内存维护，再同步 Redis。

小队房间优先覆盖：

```text
创建小队
加入小队
退出小队
队长
成员列表
准备状态
小队状态广播
```

匹配队列优先使用 Redis：

```text
matchmaking:queue:normal
```

任务副本生命周期优先使用简单状态：

```text
waiting -> ready -> running -> finished
```

排行榜使用 Redis ZSet：

```text
rank:score
```

任务记录、结算记录和长期战绩必须落 PostgreSQL。

## 日志规范

当前阶段可以使用标准库：

```go
log.Println()
```

后期再引入结构化日志，例如 zap。

日志至少记录：

- 服务启动
- 数据库连接成功/失败
- Redis 连接成功/失败
- 关键业务错误
- WebSocket 连接建立/断开
- 小队创建/退出
- 任务副本开始/结算

不要在日志里打印：

- 明文密码
- JWT 完整 token
- 数据库密码
- 用户敏感信息

## 测试规范

每次改代码后必须执行：

```powershell
cd E:\game-realtime-gm\backend
gofmt -w 修改过的文件
go mod tidy
go test ./...
```

`[no test files]` 不是错误。

它表示目前没有测试文件，但包可以编译。

后期需要逐步补：

- handler 测试
- service 测试
- Redis key 逻辑测试
- WebSocket 消息测试

## Git 规范

不要把无关变更混在一个提交里。

推荐提交粒度：

```text
feat: add jwt login
feat: add redis online status
feat: add player profile APIs
docs: add day05 learning plan
fix: avoid duplicate handler helper
```

不要提交：

- `.env`
- 本地日志
- IDE 临时文件
- 编译产物
- 数据库真实数据文件

## 每日计划生成规范

以后生成 `dayxx-plan.md` 时，必须统一创建或更新到：

```text
E:\game-realtime-gm\docs\day\dayXX-plan.md
```

不要再生成到：

```text
E:\game-realtime-gm\docs\dayXX-plan.md
```

必须遵守：

1. 先检查当前代码状态。
2. 先跑或建议跑 `go test ./...`。
3. 不跳过未完成的前置任务。
4. 每个任务都写清楚文件路径和操作类型。
5. 涉及已有文件修改时，说明“整文件替换”还是“局部修改”。
6. 如果新增 helper，必须说明为什么不放在已有文件里。
7. 如果多个文件属于同一个包，必须检查函数和类型是否重名。
8. 每次引入新依赖，必须说明在哪个目录执行 `go get`。
9. 每天都要有验证方式。
10. 每天都要有“今日不要做”。

尤其注意：

```text
同一个目录 + 同一个 package = 同一个包
同一个包里不能有两个同名函数
```

这是后续生成计划时必须主动检查的规则。

## 决策记录规范

遇到架构选择时，应该记录为什么这样做。

例如：

```text
为什么在线状态用 Redis？
因为在线状态变化频繁，可以过期，不适合频繁写 PostgreSQL。
```

```text
为什么当前不引入 service 层？
因为当前业务还简单，handler 直接访问 DB 更适合学习；当业务逻辑变复杂时再提取。
```

这些记录可以放在：

```text
docs/learning-roadmap.md
docs/day/dayxx-plan.md
```

或后期新增：

```text
docs/adr/
```

## 当前阶段推荐优先级

### 必须坚持

- 能跑通
- 能解释
- 结构清楚
- 错误可排查
- 数据可验证
- 文档可复现

### 暂时不追求

- 大型微服务架构
- 完整 Clean Architecture
- Kubernetes
- 高并发压测指标
- 完整监控体系
- 游戏服务器集群
- UDP / KCP / QUIC
- 商业级战斗状态同步
- 跨服、滚服、灰度发布

## 项目演进路线

推荐路线：

```text
Day 01-05：
Go API、PostgreSQL、Redis、JWT、玩家基础模块

Day 06-10：
GM 管理员、基础权限、操作日志、接口文档

Day 11-18：
WebSocket、心跳、在线状态、消息格式

Day 19-28：
WebSocket 连接管理、小队房间、小队状态广播

Day 29-35：
PVE 任务匹配、任务副本生命周期、结算、防重复、排行榜

Day 36-45：
React GM 后台

Day 46-56：
测试、压测、pprof、README、架构图、简历和面试复盘
```

## 最重要的一条

这个项目的目标不是把所有高级技术堆上去。

真正目标是：

```text
每一个功能都能运行
每一个文件都有职责
每一条数据流都能讲清楚
每一个错误都知道怎么排查
```

只要坚持这个标准，这个项目就能真正训练出后端工程能力。
