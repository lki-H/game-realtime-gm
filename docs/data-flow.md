# 关键业务数据流

> 文档角色：跨模块时序与数据流说明
> 权威级别：L2（当前流程说明）
> 状态：已实现
> 适用范围：一期关键业务链路
> 事实来源：当前 Handler、Service、Manager、MySQL 与 Redis 调用顺序
> 最后更新：2026-08-31

本文只描述调用顺序；模块职责见 [系统详细设计](system-design.md)，字段契约见 [OpenAPI](openapi.yaml) 与 [WebSocket 协议](ws-protocol.md)。

## 1. 注册与初始资产

```mermaid
sequenceDiagram
    participant C as HTTP Client
    participant H as AuthHandler
    participant DB as MySQL

    C->>H: POST /api/register
    H->>H: 校验字段并生成 bcrypt hash
    H->>DB: BEGIN
    H->>DB: INSERT players
    H->>DB: INSERT player_assets(0)
    H->>DB: SELECT player
    H->>DB: COMMIT
    H-->>C: 201 register success
```

用户名唯一冲突返回 409；任一步失败都回滚，不产生只有账号没有资产行的正常新玩家。

## 2. WebSocket 在线与断线

```mermaid
sequenceDiagram
    participant C as Player Client
    participant H as WS Handler
    participant W as WS Manager
    participant R as Redis
    participant S as Squad Manager

    C->>H: GET /ws?token=...
    H->>H: 校验 player JWT
    H->>W: Register(connection)
    W-->>W: 替换同玩家旧连接
    H->>R: SET online key EX 120s
    H-->>C: server.welcome
    loop 连接存活
        H->>R: 续期 online TTL
        H-->>C: WebSocket ping
    end
    C--xH: 断开
    H->>W: Unregister(current connection)
    H->>S: HandleDisconnect
    H-->>C: squad.state.changed（向其他在线成员）
```

Redis 在线 key 不在断开时立即删除，等待 TTL 过期，减少短暂断线抖动。访问日志只记录 `/ws` path，不记录 query。

## 3. 小队与任务会话

```mermaid
sequenceDiagram
    participant L as Leader
    participant M as Member
    participant H as WS Handler
    participant S as Squad Manager
    participant Q as Mission Manager
    participant W as WS Manager

    L->>H: squad.create
    H->>S: Create
    H-->>L: squad.create.result
    M->>H: squad.join
    H->>S: Join
    H->>W: Broadcast squad.state.changed
    M->>H: squad.ready(true)
    H->>S: SetReady
    H->>W: Broadcast ready_changed
    L->>H: mission.create
    H->>H: 校验队长、全员在线/ready
    H->>Q: Create(waiting)
    H->>W: Broadcast mission_created
    L->>H: mission.ready/start/finish
    H->>Q: Transition
    H->>W: Broadcast mission.state.changed
```

任务会话在 Go 内存中；`running` 仅表示业务生命周期，不表示存在独立战斗进程。

## 4. 匹配 ticket 与超时

```mermaid
sequenceDiagram
    participant C as Player
    participant H as WS Handler
    participant M as Matchmaking Manager
    participant R as Redis
    participant T as Timeout Loop
    participant W as WS Manager

    C->>H: matchmaking.enqueue
    H->>M: Enqueue
    M->>R: 检查 player index
    M->>R: TxPipeline ticket/player/queue/timeout
    H-->>C: queued ticket + queue_position
    loop 每秒
        T->>M: CleanupExpired
        M->>R: 查询到期成员并转为 timeout
        T->>W: matchmaking.state.changed
    end
```

当前没有 `matched` 状态或多名玩家撮合成功流程。

## 5. 幂等结算与排行榜

```mermaid
sequenceDiagram
    participant C as Player
    participant H as WS Handler
    participant S as Settlement Service
    participant M as Mission Manager
    participant DB as MySQL
    participant L as Leaderboard Service
    participant R as Redis

    C->>H: settlement.create
    H->>S: Create(request, player, squad)
    S->>M: GetByID + finished 校验
    S->>DB: BEGIN
    S->>DB: INSERT mission_records
    loop 按 player_id 升序
        S->>DB: INSERT reward_records
        S->>DB: SELECT player_assets FOR UPDATE
        S->>DB: UPDATE balance + INSERT ledger
    end
    S->>DB: COMMIT
    S-->>H: settlement + created flag
    H->>L: best-effort SyncSettlement
    L->>R: Lua 更新个人最佳分
    H-->>C: settlement.create.result
```

若唯一键表明请求已处理，Service 加载已有记录并返回，不重复发奖。若排行榜同步失败，已提交的 MySQL 资产仍是事实。

## 6. GM 观察

```mermaid
sequenceDiagram
    participant G as GM Client
    participant A as AdminAuth + RequestID
    participant O as Observation Service
    participant Mem as In-memory Managers
    participant R as Redis
    participant DB as MySQL

    G->>A: GET /api/admin/realtime/summary
    A->>O: 通过管理员身份
    O->>Mem: connection/squad/mission stats
    O->>R: queued count
    O->>DB: settled count
    O-->>G: aggregate + observed_at + X-Request-ID
```

读操作跨三个数据源依次执行，因此是近实时聚合，不是原子快照。
