# 项目方向说明：共斗 PVE 游戏后台与实时服务基础

## 这份文档的作用

这份文档用于说明 `game-realtime-gm` 后续的项目方向，避免项目定位过于泛化。

项目不会直接伪装成大型商业战斗服，也不会在当前阶段强行堆 UDP、KCP、QUIC、微服务、跨服、滚服等高级概念。

更合适的定位是：

```text
基于 Go 的游戏后台与共斗 PVE 实时服务基础项目。
```

这个定位同时服务两个求职方向：

- 普通 Go 后端实习：展示 Go、Gin、PostgreSQL、Redis、JWT、WebSocket、Docker、接口文档和工程结构。
- 游戏服务端实习：展示玩家服务、GM 后台、在线状态、小队房间、匹配队列、任务副本生命周期和结算记录。

## 为什么不直接改成 UDP/KCP 战斗服

当前项目仍处于学习和求职展示阶段。

如果现在直接引入 UDP、KCP、QUIC、帧同步、状态同步和战斗服拆分，会带来几个问题：

- 项目复杂度会明显上升。
- 很多基础后端能力还没有完全打牢。
- 普通 Go 后端岗位可能看不懂项目重点。
- 简历容易变成堆技术名词，但面试时讲不清楚链路。

因此当前阶段先坚持：

```text
单体 Go 服务
        ↓
模块清晰
        ↓
接口可测
        ↓
数据可验证
        ↓
业务链路能讲清楚
```

UDP/KCP/QUIC、状态同步和战斗服架构会作为二期研究方向，而不是当前主线。

## 当前项目已经覆盖的能力

当前项目已经覆盖或正在推进：

- 玩家注册和登录
- bcrypt 密码哈希
- JWT 玩家鉴权
- 管理员登录
- 管理员鉴权
- 玩家查询
- 封禁和解封
- GM 操作日志
- dashboard 统计接口
- Redis 在线状态
- WebSocket 玩家长连接
- WebSocket 玩家 token 鉴权
- 进程内连接管理器
- WebSocket 驱动 Redis 在线状态续期
- Docker Compose 本地环境
- 接口文档和每日任务文档
- GitHub 持续提交记录

这些能力对应真实游戏项目中的：

```text
账号服务
玩家基础服务
GM 运营后台
在线状态服务
实时连接入口
日志审计
本地开发环境
```

## 后续业务语义调整

后续不要把房间、匹配、结算写成过于抽象的 demo。

建议统一调整成共斗 PVE 语义：

| 原说法 | 后续项目说法 | 目的 |
| --- | --- | --- |
| 房间系统 | 小队房间 | 更贴近 4 人共斗 PVE |
| 房间广播 | 小队状态广播 | 表达玩家加入、退出、准备、队长变更 |
| 匹配队列 | PVE 任务匹配 | 不只是排队，而是按任务模式组队 |
| 对局记录 | 任务副本记录 | 更贴近关卡/副本制游戏 |
| 结算 | 任务结算 | 表达耗时、奖励、结果、积分 |
| 排行榜 | 战绩/通关/积分排行榜 | 更贴近运营展示和玩家成长 |

## 参考项目与吸收方式

后续参考项目分成两类。

第一类是开源架构参考，用来理解游戏后端、实时连接、房间、匹配、排行榜、后台和工程结构。

第二类是真实业务模型参考，用来理解商业游戏后台平台里常见的 Party、Lobby、Session、Matchmaking、Game Session、Player Session 等概念。

当前阶段只吸收概念、命名、状态流转和简化模型，不引入完整框架、云服务、Kubernetes、微服务集群或商业平台 SDK。

### 开源架构参考

| 项目 | 适合学习 | 本项目怎么吸收 | 当前不要照抄 |
| --- | --- | --- | --- |
| [Nakama](https://heroiclabs.com/docs/nakama/) | 账号、实时连接、Party、Matchmaker、排行榜、Console | 学习游戏后端功能边界；将 Party 映射为小队，将 Matchmaker 映射为 PVE 匹配 | 不照抄完整平台架构、多语言 runtime、复杂 API 网关 |
| [Pitaya](https://pitaya.readthedocs.io/) | Go 游戏服务器、TCP/WebSocket、Session、Route、Group、Push | 学习 Group 如何表达小队广播，学习 Session 绑定玩家和服务端主动推送 | 不引入 etcd、NATS、RPC 集群 |
| [Colyseus](https://docs.colyseus.io/) | Room 生命周期、状态同步、匹配、maxClients | 学习 onCreate、onJoin、onLeave、onDispose；用来设计小队和任务副本生命周期 | 不换成 Node/TypeScript，不做完整状态同步框架 |
| [Open Match](https://open-match.dev/site/docs/) | Ticket、Pool、MatchProfile、MatchFunction、Director | 学习匹配流程模型；把 Ticket 简化为 Redis 匹配队列中的玩家或小队请求 | 不部署 Open Match，不拆多个匹配服务 |
| [Agones](https://agones.dev/site/docs/) | GameServer 生命周期、分配、玩家容量、Fleet | 学习任务副本/战斗服的生命周期概念 | 不上 Kubernetes，不使用 CRD/Fleet/GameServerAllocation |
| [Gin-Vue-Admin](https://gin-vue-admin.com/guide/introduce/project.html) | GM 后台、菜单、RBAC、API 权限 | 后期做 React GM 后台和权限管理时参考 | 不照搬大后台框架 |
| [go-clean-template](https://github.com/evrone/go-clean-template) | Go 分层、配置、测试、Docker | 学习工程结构和职责拆分 | 不过早上复杂 Clean Architecture |

### 真实业务模型参考

| 平台/项目 | 适合学习 | 本项目怎么吸收 | 当前不要照抄 |
| --- | --- | --- | --- |
| [AccelByte Gaming Services](https://docs.accelbyte.io/gaming-services/modules/multiplayer/parties-presence/) | Party、Presence、Session、Matchmaking、Dedicated Server Manager | 学习“小队 -> 匹配 -> 会话/任务副本 -> 分配服务器”的业务链 | 不接入商业平台 |
| [PlayFab Multiplayer](https://learn.microsoft.com/en-us/gaming/playfab/multiplayer/matchmaking/) | Matchmaking Queue、Match Size、Server Allocation | 学习匹配队列、最小/最大人数、匹配后分配服务器的表达方式 | 不接入 PlayFab，不做云服务器自动分配 |
| [AWS GameLift FlexMatch](https://docs.aws.amazon.com/gameliftservers/latest/flexmatchguide/match-intro.html) | 匹配规则、Game Session、Player Session | 学习成熟匹配系统怎么描述规则和玩家会话 | 不上 AWS GameLift，不做复杂规则引擎 |
| [Epic Online Services](https://dev.epicgames.com/docs/game-services/lobbies-and-sessions) | Lobby、Session、Presence | 学习 Lobby 和 Session 的区别 | 不接入 EOS，不做跨平台账号体系 |
| [Hathora](https://hathora.dev/docs) | Room、server process、区域部署 | 学习“一个房间/任务实例对应一个服务进程”的部署概念 | 当前不拆独立战斗服进程 |
| [Pragma Engine](https://pragma.gg/) | Player Data、Party、Matchmaking、Game Instance | 学习玩家数据、队伍、游戏实例之间的业务关系 | 不照搬商业后端平台架构 |
| [Centrifugo](https://centrifugal.dev/) | Channel、Presence、Pub/Sub、实时推送 | 学习 `channel -> squad:<id>`、presence、join/leave、小队广播模型 | 当前不引入 Centrifugo 服务 |
| [Casbin](https://casbin.org/docs/overview) | RBAC/ABAC 权限模型 | 后期优化 GM 权限模型 | 当前不急着引入复杂权限策略语言 |
| [Prometheus Go Client](https://prometheus.io/docs/guides/go-application/) / [OpenTelemetry Go](https://opentelemetry.io/docs/languages/go/) | 指标、链路追踪、可观测性 | Day 46 以后为在线人数、WebSocket 连接数、接口耗时补工程化展示 | 当前不提前做完整监控体系 |

### 概念映射

| 外部概念 | 本项目概念 |
| --- | --- |
| Party / Lobby / Group | 小队房间 |
| Presence | 在线状态、连接状态 |
| Channel | 小队广播频道 |
| Ticket | 匹配票据 |
| Matchmaking Queue / Pool | PVE 任务匹配队列 |
| Match / Game Session / Room | 任务副本实例 |
| Player Session | 玩家在任务副本内的参与记录 |
| GameServer | 二期战斗服/任务服概念 |
| Fleet / Allocation | 二期部署和调度概念 |

### 参考项目落地规则

这些参考项目后续主要落到 4 个方向：

| 后续功能 | 优先参考 | 本项目落地方式 |
| --- | --- | --- |
| WebSocket 连接生命周期 | Pitaya、Centrifugo | 学习 Session、Channel、Presence 的表达方式，当前只保留单体连接管理、ping/pong 和在线状态 |
| 小队房间 | Nakama Party、Colyseus Room、Centrifugo Channel | 设计 `squad`、成员、队长、准备状态和 `squad:<id>` 广播语义 |
| PVE 任务匹配 | Open Match、PlayFab、GameLift FlexMatch | 设计简化版 `matchmaking_ticket`、匹配队列和最小/最大人数规则 |
| 任务副本生命周期 | Colyseus Room、Agones GameServer、Hathora Room | 设计 `mission_instance` 的 waiting、ready、running、finished 状态 |

当前阶段每次只吸收一个概念并落到可运行的小功能里。

不要把参考项目当成“要照搬的目标架构”。它们更像是命名词典和业务边界样本，用来帮助你把自己的项目讲得更像真实游戏后端。

## 推荐后续 Day 路线

### Day 23：WebSocket ping/pong 心跳与读写超时

目标：

```text
服务端定时发送 ping，客户端返回 pong，服务端能发现死连接。
```

价值：

- 提升长连接可靠性。
- 为断线重连、小队广播打基础。

### Day 24：统一 WebSocket 消息协议

目标：

```json
{
  "type": "squad.ready",
  "request_id": "req-001",
  "data": {}
}
```

价值：

- 后续小队、匹配、广播、错误返回都使用统一格式。
- 避免 WebSocket 只停留在 echo demo。

### Day 25：连接会话 ID

目标：

```text
connection_id
player_id
connected_at
last_pong_at
```

价值：

- 区分同一个玩家的新旧连接。
- 为安全删除 Redis 在线 key 和断线重连做准备。

### Day 26：小队房间基础

目标：

```text
创建小队
加入小队
离开小队
队长
成员列表
准备状态
```

价值：

- 从普通 WebSocket 连接进入多人 PVE 业务。

### Day 27：小队状态广播

目标：

```text
成员加入广播
成员退出广播
准备状态广播
队长切换任务广播
```

价值：

- 训练服务端主动推送能力。
- 让 WebSocket 真正服务业务。

### Day 28：任务副本生命周期

目标：

```text
waiting -> ready -> running -> finished
```

价值：

- 理解多人 PVE 任务实例如何从创建到结束。

### Day 29：PVE 任务匹配队列

目标：

```text
玩家进入匹配
Redis 记录匹配队列
凑够人数后创建小队或任务实例
```

价值：

- 训练 Redis 在实时业务中的使用。

### Day 30：任务结算记录

目标：

```text
任务开始记录
任务结束记录
耗时
结果
奖励
积分
```

价值：

- PostgreSQL 保存长期可靠战绩。
- 为 GM 查询和排行榜打基础。

## 二期研究方向

当前项目主线完成后，再单独研究：

- UDP 基础
- KCP
- QUIC
- 状态同步
- 快照同步
- 客户端预测
- 服务端校验
- 断线重连恢复战斗
- 战斗服拆分
- 跨服和滚服架构
- 灰度发布

这些内容可以作为新项目或实验目录，不建议直接塞进当前主线。

建议后续新增：

```text
realtime-sync-lab
```

或在当前项目中新增：

```text
docs/realtime-sync-research.md
```

## 面试表达

投普通 Go 后端实习时，可以这样说：

```text
我做了一个 Go 游戏业务后台项目，用 Gin 提供 HTTP API，用 PostgreSQL 保存玩家和操作日志，用 Redis 保存在线状态，用 JWT 做玩家和管理员鉴权，并通过 WebSocket 实现玩家长连接。项目重点是后端工程结构、接口设计、数据持久化、缓存状态、权限隔离和本地 Docker 环境。
```

投游戏服务端实习时，可以这样说：

```text
这个项目以 Go 游戏后台为基础，正在向共斗 PVE 服务端基础演进。当前已经完成账号、GM、操作日志、在线状态和 WebSocket 连接管理，后续会扩展小队房间、PVE 任务匹配、任务副本生命周期、结算记录和排行榜，为进一步学习战斗服和实时同步打基础。
```

## 当前边界

当前项目不是：

- 完整商业游戏服务器
- 动作战斗服
- UDP/KCP 同步项目
- 微服务集群项目
- 跨服/滚服系统

当前项目是：

```text
可运行、可解释、可展示的 Go 游戏后台与共斗 PVE 实时服务基础项目。
```
