# API 总览

本文档记录当前项目已经实现的 HTTP API 和 WebSocket 消息，方便本地测试与项目复盘。

默认服务地址：

```text
http://localhost:8080
```

## 通用响应格式

成功响应通常为：

```json
{
  "code": 0,
  "message": "ok",
  "data": {}
}
```

错误响应通常为：

```json
{
  "code": 40001,
  "message": "invalid request"
}
```

## 鉴权方式

玩家接口使用玩家 token：

```text
Authorization: Bearer 玩家token
```

管理员接口使用管理员 token：

```text
Authorization: Bearer 管理员token
```

注意：

```text
Bearer 后面有一个空格。
```

玩家 token 不能访问 `/api/admin/*` 接口。

管理员 token 也不等同于普通玩家身份。

## 健康检查

### GET /health

用途：检查后端服务是否启动。

鉴权：不需要。

响应示例：

```json
{
  "code": 0,
  "message": "ok"
}
```

## 认证模块

### POST /api/register

用途：注册玩家账号。

鉴权：不需要。

请求体：

```json
{
  "username": "player01",
  "password": "123456",
  "nickname": "玩家01"
}
```

主要错误：

```text
40001 invalid request
40901 username already exists
50001 generate password hash failed
50002 create player failed
```

### POST /api/login

用途：玩家登录并获取玩家 token。

鉴权：不需要。

请求体：

```json
{
  "username": "player01",
  "password": "123456"
}
```

响应重点：

```json
{
  "code": 0,
  "message": "login success",
  "data": {
    "token": "玩家token"
  }
}
```

主要错误：

```text
40101 username or password is wrong
40321 player is banned
```

### POST /api/admin/login

用途：管理员登录并获取管理员 token。

鉴权：不需要。

请求体：

```json
{
  "username": "admin",
  "password": "admin123456"
}
```

响应重点：

```json
{
  "code": 0,
  "message": "admin login success",
  "data": {
    "token": "管理员token"
  }
}
```

主要错误：

```text
40011 invalid request
40111 username or password is wrong
50031 query admin failed
50032 generate admin token failed
```

## 玩家模块

以下接口需要玩家 token。

### GET /api/me

用途：查询当前登录玩家信息。

鉴权：玩家 token。

### PATCH /api/me/nickname

用途：修改当前登录玩家昵称。

鉴权：玩家 token。

请求体：

```json
{
  "nickname": "新的昵称"
}
```

主要错误：

```text
40002 invalid request
40003 nickname cannot be empty
40401 player not found
50011 update nickname failed
```

### GET /api/players

用途：玩家侧查看玩家列表。

鉴权：玩家 token。

常用查询参数：

```text
page=1
page_size=10
keyword=player
```

示例：

```text
GET /api/players?page=1&page_size=10
```

### GET /api/players/:id

用途：玩家侧查看某个玩家详情。

鉴权：玩家 token。

示例：

```text
GET /api/players/1
```

主要错误：

```text
40004 invalid player id
40401 player not found
50012 query player failed
```

## WebSocket 实时连接模块

### GET /ws

用途：建立玩家 WebSocket 长连接，用于验证实时通信入口、玩家 token 鉴权、进程内连接管理和 Redis 在线状态续期。

连接地址：

```text
ws://localhost:8080/ws?token=玩家token
```

鉴权：需要玩家 token，通过 `token` query 参数传入。

连接成功后，服务端会主动发送统一格式的欢迎消息：

```json
{
  "type": "server.welcome",
  "code": 0,
  "message": "ok",
  "data": {
    "connection_id": "conn_1_1780000000000000000_a1b2c3d4e5f60708",
    "player_id": 1,
    "username": "player01",
    "connected_at": "2026-06-29T10:00:00+08:00",
    "last_pong_at": "2026-06-29T10:00:00+08:00",
    "online_players": 1,
    "online_ttl_seconds": 120
  },
  "server_time": "2026-06-29T10:00:00+08:00"
}
```

welcome 中连接会话字段说明：

| 字段 | 说明 |
| --- | --- |
| `connection_id` | 当前这一次 WebSocket 连接的唯一标识，用于区分同一个玩家的新旧连接 |
| `connected_at` | 当前连接建立时间 |
| `last_pong_at` | 最近一次收到客户端 pong 的时间；刚连接成功时初始值等于 `connected_at` |
| `online_players` | 当前 Go 进程内连接管理器记录的玩家连接数 |
| `online_ttl_seconds` | Redis 在线状态 TTL 秒数 |

Day 24 起，客户端发送 WebSocket 业务消息时，需要使用统一 JSON 协议。

客户端消息格式：

```json
{
  "type": "debug.echo",
  "request_id": "req-001",
  "data": {
    "text": "hello day24"
  }
}
```

服务端响应格式：

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

当前支持的业务消息：

| type | 说明 |
| --- | --- |
| `debug.echo` | 调试用 echo 消息，服务端会按统一格式返回 `debug.echo.result` |
| `squad.create` | 创建小队，创建者成为队长并自动设置为 `ready=true` |
| `squad.join` | 加入指定小队，需要 `data.squad_id` |
| `squad.leave` | 离开当前小队；最后一人离开时小队解散 |
| `squad.ready` | 设置当前玩家准备状态，需要 `data.ready` |
| `squad.me` | 查询当前玩家所在小队 |
| `squad.state.changed` | 服务端主动推送的小队状态变化广播，例如成员加入、离开、ready 变化 |
| `mission.create` | 队长为当前小队创建 waiting 任务会话，需要 `data.mission_id` |
| `mission.ready` | 队长执行 `waiting -> ready` |
| `mission.start` | 队长执行 `ready -> running` |
| `mission.finish` | 队长执行 `running -> finished` |
| `mission.cancel` | 队长执行 `waiting -> canceled` |
| `mission.me` | 查询当前玩家最近的任务会话 |
| `mission.state.changed` | 服务端向其他任务参与者推送完整任务会话快照 |
| `matchmaking.enqueue` | 创建单玩家 queued ticket，需要 `mission_id` 和 `role` |
| `matchmaking.cancel` | 取消当前 queued ticket |
| `matchmaking.me` | 查询当前或最近 ticket |
| `matchmaking.state.changed` | 票据超时时主动通知在线玩家 |

Day 27 起，服务端会在小队成员加入、离开、ready 状态变化时，向小队内其他在线成员推送 `squad.state.changed`。小队状态当前保存在 Go 进程内存中，服务重启后会清空。

Day 28 起，服务端支持小队成员断线/重连、队长转移和任务会话状态机。任务会话同样保存在 Go 进程内存中，服务重启后会清空。

Day 29 起，服务端使用 Redis 保存 matchmaking ticket、任务等待队列、超时索引和玩家索引，支持查询、取消和超时通知。

创建小队请求：

```json
{
  "type": "squad.create",
  "request_id": "req-create-001",
  "data": {}
}
```

创建小队响应：

```json
{
  "type": "squad.create.result",
  "request_id": "req-create-001",
  "code": 0,
  "message": "ok",
  "data": {
    "squad": {
      "id": "squad_1",
      "leader_id": 1,
      "max_members": 4,
      "members": [
        {
          "player_id": 1,
          "username": "player01",
          "ready": true,
          "online": true,
          "joined_at": "2026-07-01T10:00:00+08:00"
        }
      ],
      "created_at": "2026-07-01T10:00:00+08:00",
      "updated_at": "2026-07-01T10:00:00+08:00"
    }
  },
  "server_time": "2026-07-01T10:00:00+08:00"
}
```

加入小队请求：

```json
{
  "type": "squad.join",
  "request_id": "req-join-001",
  "data": {
    "squad_id": "squad_1"
  }
}
```

设置准备状态请求：

```json
{
  "type": "squad.ready",
  "request_id": "req-ready-001",
  "data": {
    "ready": true
  }
}
```

离开小队请求：

```json
{
  "type": "squad.leave",
  "request_id": "req-leave-001",
  "data": {}
}
```

离开小队响应中的 `disbanded` 表示小队是否已经因为最后一个成员离开而解散：

```json
{
  "type": "squad.leave.result",
  "request_id": "req-leave-001",
  "code": 0,
  "message": "ok",
  "data": {
    "squad": null,
    "disbanded": true
  },
  "server_time": "2026-07-01T10:00:00+08:00"
}
```

查询当前小队请求：

```json
{
  "type": "squad.me",
  "request_id": "req-me-001",
  "data": {}
}
```

小队状态变化时，其他在线成员会收到服务端主动推送：

~~~json
{
  "type": "squad.state.changed",
  "code": 0,
  "message": "ok",
  "data": {
    "event": "member_joined",
    "actor_player_id": 2,
    "squad": {
      "id": "squad_1",
      "leader_id": 1,
      "max_members": 4,
      "members": [
        {
          "player_id": 1,
          "username": "player01",
          "ready": true,
          "online": true,
          "joined_at": "2026-07-01T10:00:00+08:00"
        },
        {
          "player_id": 2,
          "username": "player02",
          "ready": false,
          "online": true,
          "joined_at": "2026-07-01T10:01:00+08:00"
        }
      ]
    }
  },
  "server_time": "2026-07-01T10:01:00+08:00"
}
~~~

支持的 `event`：

| event | 说明 |
| --- | --- |
| `member_joined` | 有成员加入小队 |
| `member_left` | 有成员离开小队 |
| `ready_changed` | 有成员修改 ready 状态 |
| `member_disconnected` | 成员断线，保留成员但设置 `online=false`、`ready=false` |
| `member_reconnected` | 保留成员重新连接，设置 `online=true`，ready 仍为 false |
| `leader_changed` | 队长断线或主动离队后发生队长转移 |
| `squad_disbanded` | 小队解散事件类型已预留；当前最后一名成员离开时只返回发起者自己的 `squad.leave.result` |

### Day28 任务会话

mission_instance 是任务会话与结算生命周期元数据，不是战斗服进程，也不保存逐帧物理、技能或 AI 状态。

合法状态路径：

~~~text
waiting -> ready -> running -> finished
waiting -> canceled
~~~

创建任务请求：

~~~json
{
  "type": "mission.create",
  "request_id": "mission-create-001",
  "data": {
    "mission_id": "training_ground"
  }
}
~~~

创建任务结果：

~~~json
{
  "type": "mission.create.result",
  "request_id": "mission-create-001",
  "code": 0,
  "message": "ok",
  "data": {
    "mission": {
      "id": "mission_instance_1",
      "mission_id": "training_ground",
      "squad_id": "squad_1",
      "player_ids": [1, 2],
      "status": "waiting",
      "created_at": "2026-08-22T14:00:00+08:00",
      "updated_at": "2026-08-22T14:00:00+08:00"
    }
  },
  "server_time": "2026-08-22T14:00:00+08:00"
}
~~~

状态操作请求都使用空对象 data：

~~~json
{
  "type": "mission.ready",
  "request_id": "mission-ready-001",
  "data": {}
}
~~~

把 type 分别改为：

| type | 合法前置状态 | 目标状态 |
| --- | --- | --- |
| mission.ready | waiting，且所有小队成员在线并 ready | ready |
| mission.start | ready，且所有小队成员在线并 ready | running |
| mission.finish | running | finished |
| mission.cancel | waiting | canceled |

发起者收到对应的 mission.*.result，其他参与者收到：

~~~json
{
  "type": "mission.state.changed",
  "code": 0,
  "message": "ok",
  "data": {
    "event": "mission_started",
    "actor_player_id": 1,
    "mission": {
      "id": "mission_instance_1",
      "mission_id": "training_ground",
      "squad_id": "squad_1",
      "player_ids": [1, 2],
      "status": "running",
      "created_at": "2026-08-22T14:00:00+08:00",
      "updated_at": "2026-08-22T14:01:00+08:00",
      "ready_at": "2026-08-22T14:00:30+08:00",
      "started_at": "2026-08-22T14:01:00+08:00"
    }
  },
  "server_time": "2026-08-22T14:01:00+08:00"
}
~~~

任务广播事件：

| event | 状态 |
| --- | --- |
| mission_created | waiting |
| mission_ready | ready |
| mission_started | running |
| mission_finished | finished |
| mission_canceled | canceled |

查询当前玩家最近任务：

~~~json
{
  "type": "mission.me",
  "request_id": "mission-me-001",
  "data": {}
}
~~~

当前限制：

- 只有当前小队队长可以创建和改变任务状态。
- 同一小队不能同时存在两个未结束任务。
- 任务、小队和成员在线状态都只保存在当前 Go 进程内存中。
- 任务创建后保存参与玩家 ID 快照，Day28 暂不处理任务中途成员增减锁定。

### Day29 Redis Matchmaking Ticket

当前 ticket 状态：

~~~text
queued -> canceled
queued -> timeout
~~~

入队请求：

~~~json
{
  "type": "matchmaking.enqueue",
  "request_id": "enqueue-001",
  "data": {
    "mission_id": "training_ground",
    "role": "damage"
  }
}
~~~

入队结果：

~~~json
{
  "type": "matchmaking.enqueue.result",
  "request_id": "enqueue-001",
  "code": 0,
  "message": "ok",
  "data": {
    "ticket": {
      "id": "ticket_9_1787384926200835700_bb603bcc3ab2efa1",
      "mission_id": "training_ground",
      "player_id": 9,
      "role": "damage",
      "status": "queued",
      "queue_position": 1,
      "created_at": "2026-08-22T15:48:46+08:00",
      "updated_at": "2026-08-22T15:48:46+08:00",
      "timeout_at": "2026-08-22T15:49:16+08:00"
    }
  },
  "server_time": "2026-08-22T15:48:46+08:00"
}
~~~

取消请求：

~~~json
{
  "type": "matchmaking.cancel",
  "request_id": "cancel-001",
  "data": {}
}
~~~

查询请求：

~~~json
{
  "type": "matchmaking.me",
  "request_id": "matchmaking-me-001",
  "data": {}
}
~~~

在线玩家的 ticket 超时时会收到：

~~~json
{
  "type": "matchmaking.state.changed",
  "code": 0,
  "message": "ok",
  "data": {
    "event": "match_timeout",
    "ticket": {
      "id": "ticket_9_1787384926200835700_bb603bcc3ab2efa1",
      "mission_id": "training_ground",
      "player_id": 9,
      "role": "damage",
      "status": "timeout",
      "created_at": "2026-08-22T15:48:46+08:00",
      "updated_at": "2026-08-22T15:49:16+08:00",
      "timeout_at": "2026-08-22T15:49:16+08:00"
    }
  },
  "server_time": "2026-08-22T15:49:16+08:00"
}
~~~

Redis key：

| Key | 类型 | 说明 |
| --- | --- | --- |
| `matchmaking:queue:<mission_id>` | ZSet | 按 created_at 排列 queued ticket |
| `matchmaking:timeouts` | ZSet | 按 timeout_at 扫描超时 ticket |
| `matchmaking:ticket:<ticket_id>` | Hash | ticket 详情 |
| `matchmaking:player:<player_id>` | String | 玩家当前或最近 ticket ID |

取消或超时后，ticket 会从两个 ZSet 移除；Hash 和玩家索引保留约 10 分钟供 `matchmaking.me` 查询。当前 30 秒超时仅用于本地演示。

当前没有实现 `matched`、凑人算法、小队整体原子入队或匹配成功后自动创建任务会话。

当前 WebSocket 业务消息错误：

| code | message | 场景 |
| --- | --- | --- |
| `40024` | `invalid websocket message json` | 客户端发送的不是合法 JSON |
| `40025` | `websocket message type required` | JSON 中缺少 `type` |
| `40026` | `websocket only supports text json messages` | 客户端发送了非文本消息 |
| `40027` | `invalid squad join data` | `squad.join` 的 `data` 不是合法 JSON |
| `40028` | `squad_id required` | `squad.join` 缺少 `data.squad_id` |
| `40029` | `invalid squad ready data` | `squad.ready` 的 `data` 不是合法 JSON |
| `40030` | `invalid mission create data` | `mission.create.data` 不是合法 JSON |
| `40031` | `mission_id required` | 缺少任务模板 ID |
| `40032` | `invalid matchmaking enqueue data` | `matchmaking.enqueue.data` 不是合法 JSON |
| `40033` | `matchmaking mission_id required` | 匹配 mission_id 为空 |
| `40034` | `matchmaking role required` | 匹配 role 为空 |
| `40035` | `invalid matchmaking value` | mission_id/role 含非法字符或过长 |
| `40332` | `squad leader required` | 普通成员尝试创建或改变任务状态 |
| `40426` | `player not in squad` | 玩家不在小队中，却执行离开、准备或查询当前小队 |
| `40427` | `squad not found` | 指定小队不存在 |
| `40428` | `mission instance not found` | 玩家没有任务会话 |
| `40429` | `matchmaking ticket not found` | 玩家没有可查询 ticket |
| `40424` | `unsupported websocket message type` | `type` 暂未支持 |
| `40926` | `player already in squad` | 玩家已经在小队中，又尝试创建或加入小队 |
| `40927` | `squad is full` | 小队人数已满 |
| `40928` | `squad member is offline` | ready/start 时存在离线成员 |
| `40929` | `squad members are not ready` | ready/start 时存在未准备成员 |
| `40930` | `squad already has active mission` | 同一小队重复创建未结束任务 |
| `40931` | `invalid mission state transition` | 任务状态跳转不合法 |
| `40932` | `mission squad changed` | 当前小队与任务创建时小队不一致 |
| `40933` | `player already queued` | 玩家重复入队 |
| `40934` | `matchmaking ticket not queued` | 已取消/超时 ticket 再次取消 |
| `40935` | `matchmaking ticket expired` | 取消时 ticket 已过期 |
| `50024` | `update online status failed` | 连接建立后更新 Redis 在线状态失败 |
| `50025` | `generate websocket connection id failed` | 服务端生成 WebSocket 连接 ID 失败 |
| `50026` | `squad operation failed` | 小队操作发生未预期的服务端错误 |
| `50027` | `mission operation failed` | 任务操作发生未预期的服务端错误 |
| `50028` | `matchmaking operation failed` | Redis 或 ticket 解析发生未预期错误 |

连接成功后，服务端会写入 Redis 在线状态：

```text
online:player:<player_id>
```

Day 25 起，Redis 在线状态的 value 保存当前 WebSocket 连接的 `connection_id`：

```text
GET online:player:1
"conn_1_1780000000000000000_a1b2c3d4e5f60708"
```

当前 TTL 为：

```text
120 秒
```

WebSocket 连接保持期间，后端会定时续期该 Redis key，并持续写入当前连接的 `connection_id`。连接断开后，后端停止续期，不主动删除 key，等待 TTL 自动过期。

Day 23 起，服务端会为 WebSocket 连接增加 ping/pong 心跳与读写超时：

```text
服务端每 30 秒发送一次 ping。
客户端返回 pong 后，服务端刷新读超时时间。
如果长期收不到 pong，ReadMessage 会超时返回错误，连接会退出。
连接退出后，进程内连接管理器会注销该玩家连接，Redis 在线状态续期也会停止。
```

当前限制：

```text
单条 WebSocket 消息最大 4096 字节。
WebSocket 写操作设置 10 秒写超时。
pong 等待时间为 70 秒。
超出消息大小限制时，服务端会结束当前连接，不继续处理该消息，也不保证通过同一连接返回 `server.error`。
```

WebSocket 验证重点：

```text
1. 使用玩家登录接口获取玩家 token。
2. 用 Apifox 或其他 WebSocket 客户端连接 ws://localhost:8080/ws?token=玩家token。
3. 连接成功后确认收到 server.welcome 消息，并检查 data.connection_id 是否存在。
4. 保持连接 30 秒以上，观察后端日志是否出现 websocket pong received。
5. 发送 debug.echo JSON 消息，确认服务端返回 debug.echo.result。
6. 发送 squad.create，确认服务端返回 squad.create.result。
7. 使用第二个玩家 token 建立另一个 WebSocket 连接，发送 squad.join 加入第一个玩家创建的小队，确认玩家 1 收到 squad.state.changed，event=member_joined。
8. 发送 squad.ready，确认发起者收到 squad.ready.result，其他成员收到 event=ready_changed。
9. 发送 squad.leave，确认发起者收到 squad.leave.result，其他成员收到 event=member_left。
10. 发送 squad.me，确认能查询当前玩家所在小队。
11. 进入 Redis 查看 online:player:<player_id> 的 value，确认它是 connection_id。
12. 查看 online:player:<player_id> 的 TTL，确认连接保持时 TTL 会被续期。
13. 使用同一个玩家 token 再开一个 WebSocket 连接，确认新的 connection_id 会替换旧连接。
14. 关闭 WebSocket 连接后，确认后端日志出现 websocket disconnected，Redis key 等待 TTL 自动过期。
15. 发送未知 type、缺少 type 和非法 JSON，确认分别返回 40424、40025 和 40024。
16. 发送超过 4096 字节的文本消息，确认消息不会被 echo 或继续处理，连接按读取限制关闭。
17. 两名成员都在线并 ready，队长依次发送 mission.create、mission.ready、mission.start、mission.finish。
18. 确认发起者收到 mission.*.result，其他参与者收到 mission.state.changed。
19. 创建第二个 waiting 任务后发送 mission.cancel，确认状态变为 canceled。
20. 验证 waiting 直接 start、finished/canceled 回到 running 返回 40931。
21. 验证普通成员改变任务状态返回 40332，重复创建活跃任务返回 40930。
22. 让成员 ready=false 或断线，确认 mission.ready/start 分别返回 40929 或 40928。
23. 关闭普通成员连接后确认 online=false、ready=false；重连后 online=true、ready 仍为 false。
24. 关闭队长连接后确认广播 leader_changed，原队长重连后不会自动抢回队长。
25. 玩家 1、玩家 2 依次发送 matchmaking.enqueue，确认 queue_position 分别为 1、2。
26. 验证重复入队返回 40933，matchmaking.me 返回 queued ticket 和当前位置。
27. 玩家 2 取消 ticket，确认 status=canceled，重复取消返回 40934。
28. 保持玩家 1 在线等待约 30 秒，确认收到 matchmaking.state.changed，event=match_timeout。
29. 超时后通过 matchmaking.me 查询 timeout ticket，并确认可以创建新 ticket。
30. 使用 Redis CLI 确认 canceled/timeout ticket 已离开任务队列和超时索引，Hash/玩家索引短期保留。
31. 验证 40032、40033、40034、40035 和 40429 错误分支。
```

主要错误：

```text
40121 websocket token missing
40122 invalid websocket token
40331 player token required
```

说明：

```text
该接口是实时服务主线的玩家连接验证。
Day 20 已要求玩家 token，管理员 token 不能连接该玩家 WebSocket。
Day 21 已将玩家连接注册到 Go 进程内存连接管理器。
Day 22 已将 WebSocket 连接和 Redis 在线状态打通。
Day 23 已为 WebSocket 增加 ping/pong 心跳、读超时、写超时和写锁。
Day 24 已将 WebSocket 业务消息调整为统一 JSON 协议。
Day 25 已为每次 WebSocket 连接生成 connection_id，并将 Redis 在线状态 value 改为当前 connection_id。
Day 26 已新增小队房间基础消息，支持创建小队、加入小队、离开小队、设置准备状态和查询当前小队。
Day 27 已新增小队状态广播；加入、离开和 ready 状态变化会通知小队内其他在线成员。
Day 28 已新增小队成员在线状态、断线重连、队长转移和任务会话状态机。
Day 29 已新增 Redis matchmaking ticket、队列位置、取消、超时通知和残留清理。
online_players 表示当前 Go 进程内管理器记录的在线玩家连接数量。
online_ttl_seconds 表示 Redis 在线状态 TTL 秒数。
同一个玩家重复连接时，旧连接会被新连接替换；连接管理器通过 connection_id 避免旧连接断开时误注销新连接。
当前不主动删除 Redis 在线 key，原因是避免旧连接断开时误删新连接刚写入的在线状态。
当前小队状态只保存在 Go 进程内存，服务重启后会清空。
当前任务会话只保存在 Go 进程内存；它是业务生命周期元数据，不是战斗服进程。
当前 matchmaking ticket 保存在 Redis，支持 queued/canceled/timeout；尚未实现 matched 和真正撮合成功。
当前已支持小队、任务会话和匹配超时广播，但暂不做结算和排行榜消息。
服务重启后，内存连接状态会清空。
本地学习阶段使用 query 参数传 token；不要在日志里打印完整 token。
```

## 在线状态模块

以下接口需要玩家 token。

### POST /api/online/heartbeat

用途：玩家上报在线心跳，把在线状态写入 Redis。

鉴权：玩家 token。

Redis key：

```text
online:player:<player_id>
```

TTL：

```text
120 秒
```

响应重点：

```json
{
  "code": 0,
  "message": "heartbeat success",
  "data": {
    "online": true,
    "ttl_seconds": 120
  }
}
```

说明：

```text
HTTP 心跳和 WebSocket 连接都会写入同一类在线状态 key。
HTTP 心跳会把在线状态写成简单在线值。
Day 25 之后，玩家保持 WebSocket 连接时，会持续续期 online:player:<player_id>，并把 value 写成当前 WebSocket connection_id。
```

### GET /api/online/status

用途：查询在线状态。

鉴权：玩家 token。

响应示例：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "online": true
  }
}
```

说明：

```text
该接口通过检查 Redis 中 online:player:<player_id> 是否存在来判断在线状态。
如果玩家 WebSocket 连接保持中，Day 22 的续期逻辑会让该接口返回 online=true。
如果 WebSocket 断开，后端停止续期；在 TTL 自动过期前，该接口可能短时间仍返回 online=true。
TTL 过期后，该接口会返回 online=false。
```

## GM 管理模块

以下接口需要管理员 token。

### GET /api/admin/me

用途：查询当前管理员身份。

鉴权：管理员 token。

主要错误：

```text
40115 admin identity missing
```

### GET /api/admin/dashboard/summary

用途：查询 GM 后台首页统计数据。

鉴权：管理员 token。

返回数据：

```text
total_players：玩家总数
normal_players：正常玩家数
banned_players：被封禁玩家数
today_new_players：今日新增玩家数
online_players：当前在线玩家数
today_admin_operations：今日 GM 操作次数
```

响应示例：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "total_players": 12,
    "normal_players": 10,
    "banned_players": 2,
    "today_new_players": 3,
    "online_players": 1,
    "today_admin_operations": 8
  }
}
```

说明：

```text
玩家相关统计来自 MySQL players 表。
在线玩家数来自 Redis online:player:* key。
今日 GM 操作次数来自 MySQL admin_operation_logs 表。
```

主要错误：

```text
50091 count total players failed
50092 count normal players failed
50093 count banned players failed
50094 count today new players failed
50095 count online players failed
50096 count today admin operations failed
```

### GET /api/admin/dashboard/recent-operation-logs

用途：查询 GM 后台首页最近操作日志。

鉴权：管理员 token。

常用查询参数：

```text
limit=10
```

参数说明：

```text
limit：返回最近多少条日志，默认 10，最大 20。
```

响应重点：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "items": [
      {
        "id": 12,
        "admin_id": 1,
        "admin_username": "admin",
        "admin_role": "super_admin",
        "action": "admin.players.list",
        "target_type": "player",
        "target_id": null,
        "detail": "page=1,page_size=10",
        "ip": "::1",
        "user_agent": "Apifox/xxx",
        "created_at": "2026-06-18T09:00:00Z"
      }
    ],
    "limit": 10
  }
}
```

说明：

```text
该接口用于 dashboard 首页最近动态。
返回结果按 id DESC 排序。
该接口不额外写入 GM 操作日志。
```

主要错误：

```text
50101 query recent operation logs failed
50102 scan recent operation log failed
50103 read recent operation log rows failed
```

### GET /api/admin/players

用途：GM 查询玩家列表。

鉴权：管理员 token。

常用查询参数：

```text
page=1
page_size=10
keyword=player
status=normal
```

示例：

```text
GET /api/admin/players?page=1&page_size=10
GET /api/admin/players?status=banned
```

说明：

```text
该接口会记录 GM 操作日志。
```

### GET /api/admin/players/:id

用途：GM 查询玩家详情。

鉴权：管理员 token。

示例：

```text
GET /api/admin/players/1
```

主要错误：

```text
40041 invalid player id
40441 player not found
50045 query player failed
```

### POST /api/admin/players/:id/ban

用途：GM 封禁玩家。

鉴权：管理员 token。

请求体：

```json
{
  "reason": "违规发言"
}
```

示例：

```text
POST /api/admin/players/1/ban
```

主要错误：

```text
40051 invalid player id
40052 invalid request
40053 ban reason cannot be empty
40054 ban reason is too long
40116 admin identity missing
40451 player not found
40951 player already banned
```

说明：

```text
封禁玩家和记录 GM 操作日志在同一个事务中完成。
```

### POST /api/admin/players/:id/unban

用途：GM 解封玩家。

鉴权：管理员 token。

请求体：

```json
{
  "reason": "申诉通过"
}
```

示例：

```text
POST /api/admin/players/1/unban
```

主要错误：

```text
40061 invalid player id
40062 invalid request
40063 unban reason cannot be empty
40064 unban reason is too long
40461 player not found
40961 player is not banned
```

说明：

```text
解封玩家和记录 GM 操作日志在同一个事务中完成。
```

## GM 操作日志模块

以下接口需要管理员 token。

### GET /api/admin/operation-log-actions

用途：查询 GM 操作日志 action 筛选选项。

鉴权：管理员 token。

响应重点：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "items": [
      {
        "value": "admin.players.list",
        "label": "查询玩家列表"
      },
      {
        "value": "admin.players.detail",
        "label": "查看玩家详情"
      },
      {
        "value": "admin.players.ban",
        "label": "封禁玩家"
      },
      {
        "value": "admin.players.unban",
        "label": "解封玩家"
      }
    ]
  }
}
```

说明：

```text
value 是实际用于 /api/admin/operation-logs?action=xxx 的筛选值。
label 是前端下拉框展示给 GM 看的中文名称。
这个接口只返回筛选选项，不额外写入 GM 操作日志。
```

### GET /api/admin/operation-logs

用途：分页查询 GM 操作日志。

鉴权：管理员 token。

常用查询参数：

```text
page=1
page_size=10
action=admin.players.list
admin_username=admin
target_type=player
target_id=1
start_time=2026-06-16T00:00:00%2B08:00
end_time=2026-06-16T23:59:59%2B08:00
range=today
```

支持的 `range`：

```text
today
last_7_days
last_30_days
```

示例：

```text
GET /api/admin/operation-logs?page=1&page_size=10
GET /api/admin/operation-logs?action=admin.players.ban
GET /api/admin/operation-logs?range=today
GET /api/admin/operation-logs?range=last_7_days
```

响应重点：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "items": [],
    "page": 1,
    "page_size": 10,
    "total": 0
  }
}
```

主要错误：

```text
40071 invalid target id
40072 invalid start_time
40073 invalid end_time
40074 start_time cannot be after end_time
40075 invalid range
50071 count operation logs failed
50072 query operation logs failed
50073 scan operation log failed
50074 read operation log rows failed
```

### GET /api/admin/operation-logs/:id

用途：根据日志 id 查询单条 GM 操作日志详情。

鉴权：管理员 token。

示例：

```text
GET /api/admin/operation-logs/1
```

响应重点：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "id": 1,
    "admin_id": 1,
    "admin_username": "admin",
    "admin_role": "super_admin",
    "action": "admin.players.list",
    "target_type": "player",
    "target_id": null,
    "detail": "page=1,page_size=10",
    "ip": "::1",
    "user_agent": "Apifox/xxx",
    "created_at": "2026-06-09T08:33:34.846804Z"
  }
}
```

主要错误：

```text
40081 invalid operation log id
40481 operation log not found
50081 query operation log failed
```

## 当前核心测试流程

### 1. 启动依赖

```powershell
cd E:\game-realtime-gm\deploy
docker compose up -d
```

### 2. 启动后端

```powershell
cd E:\game-realtime-gm\backend
go run .\cmd\server
```

### 3. 管理员登录

```http
POST http://localhost:8080/api/admin/login
Content-Type: application/json

{
  "username": "admin",
  "password": "admin123456"
}
```

### 4. 查询玩家列表并生成操作日志

```http
GET http://localhost:8080/api/admin/players?page=1&page_size=10
Authorization: Bearer 管理员token
```

### 5. 查询操作日志列表

```http
GET http://localhost:8080/api/admin/operation-logs?page=1&page_size=10
Authorization: Bearer 管理员token
```

### 6. 查询操作日志详情

```http
GET http://localhost:8080/api/admin/operation-logs/1
Authorization: Bearer 管理员token
```

## 对外展示说明

当前 API 已经形成一个基础 GM 后台闭环：

```text
管理员登录 -> 管理员鉴权 -> 查询玩家 -> 封禁/解封玩家 -> 记录操作日志 -> 查询日志 -> 查看日志详情
```

当前实现形成了下面的业务闭环：

```text
我用 Go + Gin + MySQL + Redis 实现了一个游戏 GM 管理后台后端，重点练习了 JWT 鉴权、权限隔离、事务处理、审计日志、分页筛选和 Docker 本地环境。
```
