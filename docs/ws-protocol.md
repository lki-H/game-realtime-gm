# WebSocket JSON 协议

> 文档角色：当前 WebSocket 连接与消息唯一契约
> 权威级别：L1（协议事实源）
> 状态：已实现
> 适用范围：玩家长连接、小队、任务会话、匹配与结算
> 事实来源：`handler/ws.go` 与 `ws/message.go`
> 最后更新：2026-10-06

## 1. 协议边界

WebSocket 用于业务控制消息和状态广播。第2—10节描述 legacy；第11节描述 `GAMEPLAY_MODE=v2`，其 envelope 带 `schema_version=2`。二者均不处理逐帧战斗同步。

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

## 11. V2 协议

当 `GAMEPLAY_MODE=v2` 时，同一 `/ws` 连接仍使用 JSON，但客户端必须发送 `schema_version: 2` 且 `type` 以 `v2.` 开头。动作使用稳定 `operation_id`，服务端响应包含原 `request_id`、业务 `code` 和可查询的结果；旧 `squad.*`、`mission.*`、`matchmaking.*`、`settlement.*` 消息不进入 V2 写链路。

主要消息类型：

```text
v2.social.friend_request
v2.social.friend_response
v2.social.message.send
v2.social.message.read
v2.party.create
v2.party.invite
v2.party.accept_invite
v2.party.selection
v2.party.ready
v2.party.plan_update
v2.party.regroup_propose
v2.party.regroup_respond
v2.recruitment.publish
v2.recruitment.apply
v2.recruitment.respond
v2.recruitment.withdraw
v2.match.enqueue
v2.match.cancel
v2.match.proposal_confirm
v2.match.proposal_reject
v2.recruitment.selection
v2.recruitment.ready
v2.recruitment.leave
v2.task.pause
v2.run.leave
v2.run.reconnect
v2.run.result
```

动作请求基本结构：

```json
{"schema_version":2,"type":"v2.party.ready","operation_id":"stable_intent_0001","request_id":"request_0001","data":{"party_id":"party_...","ready":true,"roster_version":2,"plan_version":1,"selection_version":1}}
```

`operation_id` 在同一意图重试时不变；同键不同内容返回40970。查询/重连 `v2.run.reconnect`、`v2.run.result` 不改变业务状态，可只带 `run_id`。成功响应为 `<type>.result`，包含 `schema_version/request_id/code/message/data`，业务变更通知包含其对象引用；客户端收到通知后拉取授权快照。

| 动作类型 | `data` 必需字段或含义 |
| --- | --- |
| `v2.social.friend_request/friend_delete/block/unblock` | `player_id` |
| `v2.social.friend_response` | `request_id`（申请ID）、`accept` |
| `v2.social.friend_withdraw` | `request_id`（原申请ID） |
| `v2.social.note` | `player_id`、`note`（至多128字节） |
| `v2.social.message.send` | `player_id`、`body`（至多512字节） |
| `v2.social.message.read` | `player_id`、`message_id`（已读水位） |
| `v2.party.create` | `{}`，初始不准备 |
| `v2.party.invite/transfer` | `party_id`、`player_id` |
| `v2.party.accept_invite` | `token`，指定接收者且10分钟有效 |
| `v2.party.join` | `party_id`，仅房主好友且friends_only |
| `v2.party.join_policy` | `party_id`、`join_policy=invite_only/friends_only` |
| `v2.party.leave` | `party_id`，不隐式退出Run |
| `v2.party.selection` | `party_id`、`task_key`（空表示不绑定）、`task_version` |
| `v2.party.plan_update` | `party_id`、`plan={operation,difficulty,rule_version,fill_policy,allow_partial,tags}` |
| `v2.party.ready` | `party_id`、`ready`、`roster_version/plan_version/selection_version` |
| `v2.recruitment.publish/apply/respond/withdraw` | 分别为`party_id`、`post_id`、`application_id/accept`或申请ID；接受后招募成员另用`selection/ready/leave`，不写好友成员 |
| `v2.task.pause` | `task_key`、`task_version`；局外停用保留已确认跨局进度，running/queued时拒绝 |
| `v2.match.enqueue` | 房主传`party_id`；不在好友房间的单排传`plan`与可选`task={task_key,task_version}` |
| `v2.match.cancel` | `{}`，只允许未分配票据合法调用者 |
| `v2.match.proposal_confirm/proposal_reject` | `proposal_id`、`revision` |
| `v2.run.leave/reconnect/result` | `run_id` |
| `v2.party.regroup_propose` | `run_id`、`owner_id`、`player_ids`、可选`plan`；已结算Run发起逐人同意 |
| `v2.party.regroup_respond` | `proposal_id`、`revision`、`accept`；最终复核版本/占用后原子迁移 |

V2业务码：40070参数、40370对象授权、40470不存在、40970状态/版本/幂等冲突、50070内部错误、50371维护暂停新准入。40321表示玩家封禁或会话撤销。40971表示在V2模式发送legacy消息/错误协议版本。内部测试错误码另在独立HTTP响应中返回，不属于玩家动作。

R1 增加入站连接和逐玩家命令限流：`WS_RATE_LIMIT_PER_MINUTE` 默认30，`WS_COMMAND_RATE_LIMIT_PER_MINUTE` 默认120；重连不重置玩家命令额度。命令耗尽返回 `server.error`（code 42970），Redis 无法复核时返回50370。文本仍限制16KiB，发送队列耗尽即关闭连接；HTTP查询恢复状态。候选自动重组按 `PVE_MAX_PROPOSAL_ROUNDS` 默认8轮封顶，达到上限发送 `v2.match.paused`，data 包含 `ticket_id/reason=proposal_round_limit`；清准备后由玩家重新发起。正常回队列保留原合法等待时间。

逐人结算提交后发送 `v2.participant.result`（run_id/player_id/status）；整个 Run 最终关闭后发送 `v2.run.result`。断线后先查 `/api/v2/me/activity`：无活动锁也可返回好友房间；已结算结果单独放 `latest_result`，正在等待其他成员结算的房间可返回 `settlement_run_id`。`/api/v2/runs/{run_id}/results` 在本人裁剪的 Run 字段上追加本人 `participant_result` 和 `reward_grants`，不会返回队友任务或队友奖励明细。

R2 增加 `damage/heal/rescue` 受信事件：事件payload必须带唯一action_id和有效效果；治疗目标必须是其他存活参战成员且health_after-health_before等于effective_amount，救援绑定已确认的spawn动作。重复action不重复贡献；自疗、无效效果、非参战目标被拒绝。V2广播经持久化outbox生成，通知不是事实来源。断线后通过 `/api/v2/me/activity`、`/api/v2/operations/{operation_id}`、`/api/v2/regroup/{proposal_id}` 和招募快照查询恢复。Run不提供玩家finish；即时任务完成写pending并由Worker确认，区间条件到Run终态才结算。

## 12. legacy 不支持的能力

- 二进制消息、协议版本协商或 Protobuf。
- 聊天、位置、输入帧、逐帧快照、预测、回滚或弱网补偿。
- 匹配成功、跨实例房间或会话恢复。
- 使用当前 JSON WebSocket 作为未来战斗同步协议。
