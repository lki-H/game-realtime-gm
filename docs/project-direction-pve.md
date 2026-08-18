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

如果参考公开大厂项目、招聘信息或对话资料，必须区分：

```text
可核验事实
合理推断
二期研究
当前不做
```

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
- WebSocket 统一 JSON 消息协议
- WebSocket connection_id 连接会话 ID
- 小队房间基础操作：创建、加入、离开、准备、查询当前小队
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
小队房间基础
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

对于 Varsapura、米哈游/HoYoverse、散爆等外部项目，只能把公开演示、公开招聘和行业常见架构作为参考材料。除非有官方技术文章或岗位原文明确说明，否则不要把“Go 微服务集群、全球同服、AI 调度层、Reactor 网关”等判断写成确定事实。

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

## GM 后台与游戏 Demo 的关系

GM 后台是后端能力的观察窗，负责观察、管理和审计。

游戏 Demo 是后端能力的验证器，负责验证玩家登录、WebSocket、小队、任务、位置同步、结算和排行榜链路。

两者只接入已经实现或 Day27-Day35 明确规划的后端能力，不反向定义后端需求。

详细规划见：

```text
docs/presentation-plan.md
docs/frontend-plan.md
docs/demo-plan.md
docs/ws-protocol.md
```

### 工业化建议分层

| 层级 | 可以吸收什么 | 本项目处理方式 |
| --- | --- | --- |
| 当前主线 | 小队、匹配、任务副本、结算、排行榜、基础 GM 观察 | 写入 Day 计划并实现最小闭环 |
| 工程增强 | 限流、日志 trace id、压测、指标、基础幂等、防重复提交 | 随 Day27-Day35 主线穿插加入 |
| 二期研究 | Protobuf、KCP/QUIC、战斗同步、网关拆分、微服务、Agones | 单独研究文档或实验项目 |
| 暂不采纳 | 无锁环形缓冲区、完整 Reactor、Write-Back 大宽表、复杂混沌工程 | 不进入当前主线 |

### 已完成部分也可以优化

已完成的 Day1-Day26 不是冻结状态。后续可以在新增 Day 任务中安排回头优化，但不批量重写旧 Day 文档。

优先优化会影响后续 PVE 主线的旧能力：

- 登录鉴权和封禁检查。
- GM 操作日志 action/detail 规范。
- Redis 在线状态 key/value 和 TTL 规则。
- WebSocket 错误响应、消息大小限制、连接替换逻辑。
- 小队房间错误分支、队长转移、广播和并发安全。

## Day27-Day35 可执行路线

Day23-Day26 已经完成 WebSocket 心跳、统一消息协议、connection_id 和小队房间基础。下一段路线不再继续堆底层名词，而是把共斗 PVE 闭环跑通。

### 总路线

| Day | 主线任务 | 今日回头优化项 |
| --- | --- | --- |
| Day27 | 小队状态广播 | WebSocket 错误响应、消息大小限制 |
| Day28 | 任务副本状态机 | 小队断线策略、队长转移 |
| Day29 | 匹配 ticket、Redis 队列、匹配超时、取消匹配 | Redis key 命名和残留清理 |
| Day30 | 任务结算记录、奖励记录、nonce 防重放 | `schema.sql` 补齐新增表 |
| Day31 | 结算幂等、核心资产强事务、奖励流水 | GM 日志 detail 结构化 |
| Day32 | 排行榜 ZSet、同分排序、战绩查询 | 排行榜计算单元测试 |
| Day33 | GM 实时观察在线、小队、匹配、任务、结算 | request_id / trace_id 串联 |
| Day34 | `ws_bot` 压测、`go test -race`、基础性能记录 | 慢日志和错误类型统计 |
| Day35 | README、架构图、数据流图、简历/面试表达 | GitHub 阶段提交和复盘 |

### Day27：小队状态广播

目标：

```text
成员加入广播
成员退出广播
准备状态广播
小队解散广播
```

回头优化：

```text
统一 WebSocket 错误响应
限制单条 WebSocket 消息大小
```

价值：

- 让 WebSocket 从“请求-响应”进入服务端主动推送。
- 为任务开始、匹配成功、GM 踢线打基础。

### Day28：任务副本状态机

目标：

```text
waiting -> ready -> running -> finished
waiting -> canceled
```

回头优化：

```text
玩家断线后小队如何处理
队长断线或离队后如何转移队长
```

价值：

- 训练游戏服务端最核心的状态流转。
- 避免任务副本状态乱跳。

### Day29：匹配 ticket 与 Redis 队列

目标：

```text
matchmaking_ticket
mission_id
player_id
squad_id
role
created_at
timeout_at
```

回头优化：

```text
Redis key 命名总表
匹配失败、取消、超时后的 Redis 残留清理
```

价值：

- 让匹配不只是“凑够人数”，而是开始具备真实游戏业务语义。
- 为后续 Open Match、Nakama、PlayFab 等参考概念建立映射。

### Day30：任务结算记录

目标：

```text
mission_records
reward_records
nonce
防重放
```

回头优化：

```text
schema.sql 补齐新增表结构
接口文档同步结算行为
```

价值：

- 让 PVE 任务从开始到结束形成可持久化闭环。
- 奖励和任务结果分表，方便后续审计。

### Day31：结算幂等与奖励流水

目标：

```text
idempotency_key
mission_instance_id 唯一结算
核心资产强事务
asset_ledger 简化版
```

回头优化：

```text
GM 操作日志 detail JSON 化
封禁/解封记录 before/after 状态
```

价值：

- 防止重复结算、重复发奖。
- 体现游戏后台对资产安全和审计的理解。

### Day32：排行榜与战绩查询

目标：

```text
Redis ZSet
同分按先达到者优先
玩家战绩查询
```

回头优化：

```text
排行榜分数计算单元测试
错误码区间整理
```

价值：

- 把结算结果转化成可展示、可查询的长期目标。

### Day33：GM 实时观察

目标：

```text
在线人数
WebSocket 连接数
小队数
匹配人数
运行中任务数
最近结算记录
```

回头优化：

```text
request_id / trace_id 日志串联
```

价值：

- 让 GM 后台从“查玩家”升级为“观察实时业务状态”。

### Day34：压测与并发检查

目标：

```text
cmd/tools/ws_bot
模拟多个玩家建连、建队、加入、ready
统计成功数、失败数、平均耗时
go test -race ./...
```

回头优化：

```text
慢操作日志
错误类型统计
```

价值：

- 用数据证明 WebSocket、小队和匹配不是只能手动点通。

### Day35：阶段复盘与展示

目标：

```text
README 更新
架构图
数据流图
接口测试说明
简历表达
面试讲法
```

价值：

- 把 Day1-Day35 的学习过程变成 GitHub 和 HR 能看懂的作品。

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
