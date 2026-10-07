# WebSocket JSON 协议

> 文档角色：当前 WebSocket 连接与消息唯一契约
> 权威级别：L1（协议事实源）
> 状态：已实现
> 适用范围：玩家长连接、小队、任务会话、匹配与结算
> 事实来源：`handler/ws.go` 与 `ws/message.go`
> 最后更新：2026-08-31

## 1. 协议边界

当前 WebSocket 用于业务控制消息和状态广播，不是逐帧战斗同步协议。本文只记录代码已经支持的消息；没有 `version` 字段、位置消息、状态同步、帧同步或未来 Dedicated Server 协议。

## 2. 建立连接

```text
ws://localhost:8080/ws?token=<player-jwt>
```

只接受玩家 JWT。升级前 HTTP 错误：

| HTTP | code | message |
| --- | ---: | --- |
| 401 | 40121 | `websocket token missing` |
| 401 | 40122 | `invalid websocket token` |
| 403 | 40331 | `player token required` |

当前学习环境使用 query token。访问日志只记录 `/ws` path，不记录 raw query；但 URL 仍可能进入客户端历史、代理或其他基础设施，因此公开部署前需要更安全的握手鉴权方案。

连接参数：

- 仅接受文本 JSON 消息。
- 单条消息最大 4096 字节。
- 服务端每 30 秒发送 ping，收到 pong 后将读期限延长 70 秒。
- 单次写期限 10 秒，同一连接的所有写操作由 mutex 串行化。
- 同一玩家新连接注册后会关闭旧连接。
- 在线 Redis key TTL 为 120 秒，连接存活时每 60 秒续期。

## 3. 通用 envelope

客户端：

```json
{
  "type": "squad.create",
  "request_id": "req-001",
  "data": {}
}
```

| 字段 | 必填 | 说明 |
| --- | --- | --- |
| `type` | 是 | 消息类型，不能为空 |
| `request_id` | 否 | 客户端关联请求和直接响应；广播通常为空 |
| `data` | 按类型 | 消息数据对象 |

服务端：

```json
{
  "type": "squad.create.result",
  "request_id": "req-001",
  "code": 0,
  "message": "ok",
  "data": {},
  "server_time": "2026-08-31T10:00:00+08:00"
}
```

成功 `code=0`。协议错误统一使用 `type=server.error` 并尽量回传原 `request_id`。

## 4. 连接级消息

### `server.welcome`

连接成功后服务端主动发送，`data` 包含：

```text
connection_id / player_id / username
connected_at / last_pong_at
online_players / online_ttl_seconds
```

`online_players` 是当前单体进程的连接数，不是全服 CCU。

### `debug.echo`

请求 `data` 可为任意 JSON。响应 `debug.echo.result` 的 `data` 包含 `received_type` 和 `received_data`。该消息用于调试和 `ws_bot` 基线，不是业务聊天。

## 5. 小队消息

| 客户端 type | `data` | 直接响应 |
| --- | --- | --- |
| `squad.create` | `{}` | `squad.create.result` |
| `squad.join` | `{"squad_id":"squad_1"}` | `squad.join.result` |
| `squad.leave` | `{}` | `squad.leave.result` |
| `squad.ready` | `{"ready":true}` | `squad.ready.result` |
| `squad.me` | `{}` | `squad.me.result` |

创建、加入、ready 和查询响应的 `data.squad`：

```json
{
  "id": "squad_1",
  "leader_id": 1,
  "max_members": 4,
  "members": [
    {
      "player_id": 1,
      "username": "player01",
      "ready": true,
      "online": true,
      "joined_at": "2026-08-31T10:00:00+08:00"
    }
  ],
  "created_at": "2026-08-31T10:00:00+08:00",
  "updated_at": "2026-08-31T10:00:00+08:00"
}
```

离队响应包含 `data.squad`（解散时可能省略）和 `data.disbanded`。

### `squad.state.changed`

状态变化发送给除操作者外的在线小队成员。`data` 包含 `event`、`actor_player_id`、`squad` 和可选 `disbanded`。

当前可实际发送的 event：

- `member_joined`
- `member_left`
- `ready_changed`
- `member_disconnected`
- `member_reconnected`
- `leader_changed`

断线会保留成员但设为 `online=false`、`ready=false`。队长断线或离队后按当前 Manager 规则转移；重连不会自动夺回队长。

## 6. 任务会话消息

| 客户端 type | `data` | 直接响应 |
| --- | --- | --- |
| `mission.create` | `{"mission_id":"training_ground"}` | `mission.create.result` |
| `mission.ready` | `{}` | `mission.ready.result` |
| `mission.start` | `{}` | `mission.start.result` |
| `mission.finish` | `{}` | `mission.finish.result` |
| `mission.cancel` | `{}` | `mission.cancel.result` |
| `mission.me` | `{}` | `mission.me.result` |

响应 `data.mission` 包含 `id`、`mission_id`、`squad_id`、`player_ids`、`status`、创建/更新时间和各阶段可选时间。

状态机：

```text
waiting -> ready -> running -> finished
waiting -> canceled
```

创建和所有迁移要求操作者是当前小队队长。ready/start 还要求所有成员 online 且 ready。任务的 `running` 是业务生命周期，不表示存在固定 Tick 战斗服。

### `mission.state.changed`

除操作者外的任务参与者会收到 `mission_created`、`mission_ready`、`mission_started`、`mission_finished` 或 `mission_canceled`。`data` 包含 `event`、`actor_player_id` 和完整 `mission`。

## 7. 匹配消息

| 客户端 type | `data` | 直接响应 |
| --- | --- | --- |
| `matchmaking.enqueue` | `{"mission_id":"training_ground","role":"any"}` | `matchmaking.enqueue.result` |
| `matchmaking.cancel` | `{}` | `matchmaking.cancel.result` |
| `matchmaking.me` | `{}` | `matchmaking.me.result` |

`data.ticket` 包含 `id`、`mission_id`、`player_id`、可选 `squad_id`、`role`、`status`、可选队列位置和创建/更新/超时时间。

当前状态机只有 `queued -> canceled/timeout`，没有 `matched`。ticket 默认 30 秒超时，终态数据短期保留 10 分钟。

### `matchmaking.state.changed`

后台超时扫描命中后，若玩家在线则直接发送 `event=match_timeout` 和完整 ticket。当前代码只主动广播 `match_timeout`；`match_queued` 和 `match_canceled` 常量没有进入主动广播路径。

## 8. 结算消息

客户端请求：

```json
{
  "type": "settlement.create",
  "request_id": "settle-1",
  "data": {
    "mission_instance_id": "mission_instance_...",
    "idempotency_key": "settle_training_001",
    "nonce": "nonce_training_001"
  }
}
```

只有任务所属小队队长可提交。服务端读取 finished 任务的时间与参与者，计算分数和固定奖励；客户端多传的 score/reward 字段不会参与计算。

`settlement.create.result` 的 `data.settlement` 包含一条 `record` 和每名参与者的 `rewards`。字段对应 MySQL `mission_records` 与 `reward_records`，包括任务/小队/提交者、请求键、耗时、分数、奖励类型、数量、状态和时间。

首次创建成功后，其他在线参与者收到 `settlement.created`，内容相同。幂等重试返回已有结果，但不会再次广播；仍会 best-effort 尝试修复 Redis 排行榜投影。

## 9. 错误码

### 基础协议

| code | message |
| ---: | --- |
| 40024 | `invalid websocket message json` |
| 40025 | `websocket message type required` |
| 40026 | `websocket only supports text json messages` |
| 40424 | `unsupported websocket message type` |
| 50024 | `update online status failed` |
| 50025 | `generate websocket connection id failed` |

超过 4096 字节会由 WebSocket 读取层关闭/报错，不保证返回 JSON 错误。

### 小队与任务

| code | message |
| ---: | --- |
| 40027 | `invalid squad join data` |
| 40028 | `squad_id required` |
| 40029 | `invalid squad ready data` |
| 40030 | `invalid mission create data` |
| 40031 | `mission_id required` |
| 40332 | `squad leader required` |
| 40426/40427/40428 | 玩家不在小队 / 小队不存在 / 任务不存在 |
| 40926/40927 | 已在小队 / 小队满员 |
| 40928/40929 | 成员离线 / 未全员 ready |
| 40930/40931/40932 | 已有活跃任务 / 非法迁移 / 任务小队已变化 |
| 50026/50027 | 小队 / 任务操作失败 |

### 匹配

| code | message |
| ---: | --- |
| 40032 | `invalid matchmaking enqueue data` |
| 40033/40034 | mission_id / role 必填 |
| 40035 | `invalid matchmaking value` |
| 40429 | `matchmaking ticket not found` |
| 40933/40934/40935 | 已排队 / ticket 非 queued / ticket 已过期 |
| 50028 | `matchmaking operation failed` |

### 结算

| code | message |
| ---: | --- |
| 40036 | `invalid settlement create data` |
| 40037/40038/40040 | mission instance / nonce / idempotency key 必填 |
| 40039/40041 | nonce / idempotency key 格式非法 |
| 40333 | `settlement leader required` |
| 40334 | `player not in mission` |
| 40430 | `settlement mission not found` |
| 40936/40937/40938 | 任务未结束 / nonce 重放 / 任务时间非法 |
| 40939/40940 | 任务小队已变化 / key 属于另一任务 |
| 50029 | `settlement operation failed` |

## 10. 客户端重连建议

1. 连接断开后使用有限次数、带退避的重连，不进行无间隔死循环。
2. 收到新的 `server.welcome` 后，以新 `connection_id` 为准。
3. 依次请求 `squad.me`、`mission.me`、`matchmaking.me` 恢复可查询状态；404 表示当前没有对应状态。
4. 小队和任务会话只在当前 Go 进程内存中，服务重启后无法恢复，客户端必须允许它们不存在。
5. 不依赖广播作为唯一事实；丢失广播后使用查询消息重新获取快照。

## 11. 当前不支持

- 二进制消息、协议版本协商或 Protobuf。
- 聊天、位置、输入帧、逐帧快照、预测、回滚或弱网补偿。
- 匹配成功、跨实例房间或会话恢复。
- 使用当前 JSON WebSocket 作为未来战斗同步协议。

## 12. V2/R3控制面

以上1—11节保留legacy协议范围。显式 `GAMEPLAY_MODE=v2` 时 `/ws`只接受 `schema_version=2` 和 `v2.*`。请求携带 `request_id`和稳定 `operation_id`，同意图重试不换operation_id；同键不同内容冲突40970，归档回执41071要求查询当前活动。重连/结果查询不推进业务。

```json
{"schema_version":2,"type":"v2.party.ready","request_id":"request_example","operation_id":"operation_example","data":{"party_id":"party_example","ready":true,"roster_version":2,"plan_version":1,"selection_version":1}}
```

成功响应为对应 `type.result`，包含code/message/data和request_id；主动通知不依赖客户端ACK作为资产提交条件。动作覆盖 `v2.social.*`、`v2.party.*`、`v2.recruitment.*`、`v2.match.*`、`v2.task.pause`和 `v2.run.leave/reconnect/result`，具体结构以领域Request、客户端和测试为准。房主邀请和更改局外方案，成员自己准备和选择任务；候选逐人确认，房主不能结束Run或替其他成员确认。

HTTP `/api/v2/me/activity`用于丢通知后恢复当前Party/ticket/proposal/Run及本人结果；队友任务被裁剪。`/api/v2/runs/{run_id}/results`追加本人participant_result和reward_grants。原成员重连恢复同一Run/任务尝试，局内不补新玩家。

V2文本上限16KiB，发送队列上限64，全局/IP连接配额默认256/64，拒绝超额握手42972；单玩家命令默认120次/分钟，超限42970、Redis无法复核50370。V2周期复核封禁和撤销，旧连接代次不能删除新连接。客户端不能发送受信loaded/kill/finish；内部事件监听不属于公共WS或OpenAPI。
