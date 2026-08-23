# 关键数据流

> 状态：已实现数据流
> 最后更新：2026-08-23
> 适用范围：公开项目文档 / 一期成果

## 阅读说明

本文描述当前单体 Go 服务已经实现的关键请求链路。

图中的 `mission_instance` 表示任务会话与结算生命周期元数据，不表示逐帧战斗模拟进程。

## 1. 注册与登录

```mermaid
sequenceDiagram
    participant Client as 玩家客户端
    participant Gin as Gin Handler
    participant MySQL as MySQL
    participant JWT as JWT

    Client->>Gin: POST /api/register
    Gin->>Gin: 校验 username/password/nickname
    Gin->>Gin: bcrypt password
    Gin->>MySQL: BEGIN
    Gin->>MySQL: INSERT players
    Gin->>MySQL: INSERT player_assets(0)
    Gin->>MySQL: COMMIT
    Gin-->>Client: 201 + 玩家公开信息

    Client->>Gin: POST /api/login
    Gin->>MySQL: SELECT player + password_hash
    Gin->>Gin: bcrypt compare + banned check
    Gin->>JWT: 生成 player token
    JWT-->>Gin: signed token
    Gin-->>Client: 200 + token + 玩家公开信息
```

关键点：

- 注册同时创建零余额资产行。
- password hash 不返回客户端。
- 被封禁玩家不能登录。
- token subject type 区分玩家和管理员。

## 2. WebSocket 与在线状态

```mermaid
sequenceDiagram
    participant Client as 玩家客户端
    participant Handler as WebSocket Handler
    participant Manager as WS Manager
    participant Redis as Redis
    participant Squad as Squad Manager

    Client->>Handler: GET /ws?token=player_token
    Handler->>Handler: 校验玩家 token
    Handler->>Manager: Register(player_id, connection_id)
    alt 已有旧连接
        Manager-->>Handler: old connection
        Handler->>Handler: 关闭旧连接
    end
    Handler->>Redis: SET online:player:id connection_id EX ttl
    Handler-->>Client: server.welcome

    loop 连接保持
        Handler->>Client: ping
        Client-->>Handler: pong
        Handler->>Redis: 刷新 online TTL
    end

    Client-xHandler: 断开
    Handler->>Manager: Unregister(player_id, connection_id)
    Handler->>Squad: HandleDisconnect(player_id)
    Squad-->>Handler: online=false / ready=false / leader change
    Handler-->>Client: 其他在线成员收到 squad.state.changed
```

关键点：

- 新连接替换旧连接。
- 旧连接断开时不能注销新连接。
- 在线状态通过 Redis TTL 自动过期。
- AccessLog 只记录 path，不记录 query token。

## 3. 小队与任务会话

```mermaid
sequenceDiagram
    participant Leader as 队长
    participant Member as 队员
    participant WS as WebSocket Handler
    participant Squad as Squad Manager
    participant Mission as Mission Manager

    Leader->>WS: squad.create
    WS->>Squad: Create
    Squad-->>WS: squad
    WS-->>Leader: squad.create.result

    Member->>WS: squad.join
    WS->>Squad: Join
    Squad-->>WS: updated squad
    WS-->>Member: squad.join.result
    WS-->>Leader: squad.state.changed

    Member->>WS: squad.ready(true)
    WS->>Squad: SetReady
    WS-->>Leader: squad.state.changed

    Leader->>WS: mission.create
    WS->>Squad: 校验队长与成员
    WS->>Mission: Create(mission_id, squad_id, players)
    Mission-->>WS: waiting instance
    WS-->>Leader: mission.create.result

    Leader->>WS: mission.ready / start / finish
    WS->>Mission: Transition
    Mission-->>WS: updated instance
    WS-->>Leader: mission.state.changed
    WS-->>Member: mission.state.changed
```

合法状态：

```text
waiting -> ready -> running -> finished
waiting -> canceled
```

任务会话只管理业务生命周期，不处理物理、技能、AI 或命中。

## 4. 匹配 ticket

```mermaid
sequenceDiagram
    participant Player as 玩家
    participant WS as WebSocket Handler
    participant Match as Matchmaking Manager
    participant Redis as Redis
    participant Loop as Timeout Loop

    Player->>WS: matchmaking.enqueue
    WS->>Match: Enqueue(mission_id, role)
    Match->>Redis: HSET ticket
    Match->>Redis: ZADD mission queue
    Match->>Redis: ZADD timeout index
    Match->>Redis: SET player -> ticket
    Match-->>WS: queued ticket + queue position
    WS-->>Player: matchmaking.enqueue.result

    alt 玩家取消
        Player->>WS: matchmaking.cancel
        WS->>Match: Cancel
        Match->>Redis: remove queue + timeout
        WS-->>Player: canceled result
    else 到期
        Loop->>Redis: 扫描 timeout index
        Loop->>Match: CleanupExpired
        Match->>Redis: remove queue + timeout
        WS-->>Player: matchmaking.state.changed(timeout)
    end
```

当前只实现 queued、canceled 和 timeout，没有 matched 撮合成功算法。

## 5. 幂等结算、资产与排行榜

```mermaid
sequenceDiagram
    participant Leader as 队长客户端
    participant WS as WebSocket Handler
    participant Mission as Mission Manager
    participant Settle as Settlement Service
    participant MySQL as MySQL
    participant Rank as Leaderboard Service
    participant Redis as Redis
    participant Members as 任务成员

    Leader->>WS: settlement.create(instance_id, key, nonce)
    WS->>Mission: 校验 finished 任务、队长和参与者
    WS->>Settle: Create
    Settle->>Settle: 服务端计算耗时、score、reward
    Settle->>MySQL: 查询已有幂等结果

    alt 已有结果
        MySQL-->>Settle: existing settlement
    else 首次结算
        Settle->>MySQL: BEGIN
        Settle->>MySQL: INSERT mission_records
        Settle->>MySQL: SELECT player_assets FOR UPDATE
        Settle->>MySQL: INSERT reward_records
        Settle->>MySQL: UPDATE player_assets
        Settle->>MySQL: INSERT asset_ledger
        Settle->>MySQL: COMMIT
    end

    Settle->>Rank: Sync best score
    Rank->>Redis: Lua update ZSet + player index
    Settle-->>WS: settlement result
    WS-->>Leader: settlement.create.result
    WS-->>Members: settlement.created（仅首次）
```

幂等防线：

```text
UNIQUE mission_instance_id
UNIQUE idempotency_key
UNIQUE submitted_by_player_id + nonce
```

MySQL 是结算和资产事实源。Redis 排行榜是提交后 best-effort 更新的可重建查询投影。

## 6. GM 只读观察

```mermaid
sequenceDiagram
    participant Admin as 管理员调用方
    participant MW as AdminAuth + RequestID
    participant Handler as Observation Handler
    participant Service as Observation Service
    participant Managers as WS/Squad/Mission
    participant Redis as Redis
    participant MySQL as MySQL

    Admin->>MW: GET /api/admin/realtime/summary
    MW->>MW: 校验 admin token
    MW->>Handler: admin_id + request_id
    Handler->>Service: Summary
    Service->>Managers: 连接、小队、任务统计
    Service->>Redis: queued ticket 数
    Service->>MySQL: settled 结算数
    Service-->>Handler: 近实时快照 + observed_at
    Handler-->>Admin: JSON + X-Request-ID
```

GM 观察是单实例、近实时、非跨组件原子快照。只读轮询写标准日志，不写高频 `admin_operation_logs`。

## 7. Day34 工程验证

```mermaid
sequenceDiagram
    participant Bot as ws_bot
    participant API as Go HTTP/WS
    participant GM as GM Summary
    participant Pprof as Local pprof
    participant Test as go test / race

    Bot->>API: 注册或复用测试账号
    Bot->>API: 20 个登录与 WebSocket
    Bot->>API: 5 个小队 + 10000 echo
    GM->>API: 读取 20 连接 / 5 小队
    Pprof->>API: CPU / heap / goroutine 采样
    Bot->>API: leave + close
    GM->>API: 读取 0 连接 / 0 小队
    Test->>Test: go test / vet
    Test->>Test: Docker Linux go test -race
```

完整实测见 [Day34 性能基线](performance/day34-baseline.md)。

## 数据流共同规则

- 客户端输入先校验，再进入业务层。
- 所有 SQL 使用参数化查询。
- WebSocket 请求和响应使用 `request_id` 关联。
- 核心资产只能由服务端计算并在 MySQL 事务中提交。
- Redis 数据必须允许超时、清理或重建。
- Manager 对外返回复制后的 DTO 或统计，不暴露锁、map 和连接对象。
