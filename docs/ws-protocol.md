> 状态：随实现更新
> 适用范围：项目实战 / 前端开发 / 求职展示
> 最后更新：2026-07-12

# WebSocket 协议

## 定位

本文记录 `game-realtime-gm` 的 WebSocket 消息协议。HTTP API 以 `docs/api-overview.md` 为准，实时消息以本文为准。

游戏 Demo 是后端能力的验证器，所有实时交互必须遵守本文协议。

## 连接地址

```text
ws://localhost:8080/ws?token=玩家token
```

当前只允许玩家 token 建立 WebSocket。管理员 token 不能连接玩家 WebSocket。

## 客户端消息格式

```json
{
  "type": "squad.create",
  "request_id": "req-001",
  "data": {}
}
```

字段说明：

| 字段 | 说明 |
| --- | --- |
| `type` | 消息类型，必填 |
| `request_id` | 客户端请求 ID，建议每次操作唯一 |
| `data` | 业务数据 |

## 服务端消息格式

```json
{
  "type": "squad.create.result",
  "request_id": "req-001",
  "code": 0,
  "message": "ok",
  "data": {},
  "server_time": "2026-07-12T10:00:00Z"
}
```

字段说明：

| 字段 | 说明 |
| --- | --- |
| `type` | 服务端消息类型 |
| `request_id` | 对应客户端请求 ID；主动广播可为空 |
| `code` | `0` 表示成功，非 0 表示错误 |
| `message` | 简短消息 |
| `data` | 业务数据 |
| `server_time` | 服务端时间 |

## 命名规则

```text
模块.动作
模块.动作.result
模块.state.changed
server.error
server.welcome
```

示例：

- `debug.echo`
- `debug.echo.result`
- `squad.create`
- `squad.create.result`
- `squad.state.changed`
- `mission.position.update`
- `mission.position.snapshot`

## 当前已实现或已规划的小队消息

| type | 说明 |
| --- | --- |
| `server.welcome` | 连接成功欢迎消息 |
| `server.error` | 统一错误响应 |
| `debug.echo` | 调试回显 |
| `debug.echo.result` | 调试回显结果 |
| `squad.create` | 创建小队 |
| `squad.create.result` | 创建小队结果 |
| `squad.join` | 加入小队 |
| `squad.join.result` | 加入小队结果 |
| `squad.leave` | 离开小队 |
| `squad.leave.result` | 离开小队结果 |
| `squad.ready` | 设置准备状态 |
| `squad.ready.result` | 设置准备状态结果 |
| `squad.me` | 查询当前小队 |
| `squad.me.result` | 当前小队结果 |
| `squad.state.changed` | 小队状态变化广播，Day27 规划 |

## 任务和位置同步规划

后续任务相关消息应围绕共斗 PVE 语义设计：

- `mission.start`
- `mission.start.result`
- `mission.state.changed`
- `mission.position.update`
- `mission.position.snapshot`
- `mission.finish`
- `mission.finish.result`
- `settlement.created`

位置同步必须节流。Demo 不应每帧发送位置，建议控制在 10Hz 左右。最终状态以后端广播为准。

## version 字段规划

后续小队和任务状态广播建议携带 `version`：

```json
{
  "type": "squad.state.changed",
  "data": {
    "version": 3,
    "event": "member_ready_changed",
    "squad": {}
  }
}
```

客户端收到旧 `version` 时，不应覆盖新状态。

## 重连建议

客户端断线后：

1. 使用指数退避加随机抖动重连。
2. 重连成功后先拉取当前小队或任务状态。
3. 不依赖断线期间收到的旧广播。

## 当前不做

- 不把 JSON 全量替换为 Protobuf。
- 不引入 Socket.IO 或 Colyseus SDK。
- 不实现 KCP、QUIC、自研 TCP。
- 不把 Demo 的画面需求反向变成后端协议主线。
