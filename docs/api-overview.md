# HTTP API 使用指南

> 文档角色：面向开发与学习的 HTTP 调用说明
> 权威级别：L2（人类阅读指南）
> 状态：已实现
> 适用范围：当前公共 HTTP、V2 查询和管理接口
> 事实来源：`openapi.yaml`、Router 与 Handler
> 最后更新：2026-10-10

完整方法、参数、schema、安全方案和每个操作的业务错误码以 [OpenAPI 3.1](openapi.yaml) 为准。本文只保留便于手工调用和理解的流程，不复制完整字段定义。WebSocket 见 [WebSocket 协议](ws-protocol.md)。

## 1. 基础约定

默认地址：

```text
http://localhost:8080
```

成功与失败都使用 JSON envelope：

```json
{
  "code": 0,
  "message": "ok",
  "data": {}
}
```

`code=0` 表示成功；失败时 HTTP 状态码和业务 `code` 同时表达错误类别。所有 HTTP 响应都带 `X-Request-ID`，客户端可以传入只含字母、数字、`_`、`-`、`.` 且不超过 64 字符的值；否则服务端生成新值。

## 2. 鉴权

玩家与管理员都使用 Bearer JWT，但 subject 类型互相隔离：

```http
Authorization: Bearer <player-token>
```

```http
Authorization: Bearer <admin-token>
```

- 玩家受保护接口：缺少/格式错误/无效 token 分别返回 `40102/40103/40104`，管理员 token 返回 `40301`。
- 管理员接口：缺少/格式错误/无效 token 分别返回 `40112/40113/40114`，玩家 token 返回 `40311`。
- JWT 当前有效期为 24 小时，没有刷新 token。V2 对玩家状态和会话代次复核，封禁后旧 HTTP、WS 和重连被拒绝；legacy历史接口按其原鉴权边界限用。

## 3. 路由清单

### 公共接口

| 方法 | 路径 | 用途 |
| --- | --- | --- |
| GET | `/health` | 检查 Go HTTP 进程是否响应 |
| GET | `/ready` | 检查 MySQL/Redis 与进程生命周期就绪 |
| POST | `/api/register` | 注册玩家并创建零余额资产行 |
| POST | `/api/login` | 玩家登录 |
| POST | `/api/admin/login` | 管理员登录 |

### 玩家接口

| 方法 | 路径 | 用途 |
| --- | --- | --- |
| GET | `/api/me` | 当前玩家资料 |
| PATCH | `/api/me/nickname` | 修改昵称 |
| GET | `/api/players` | 分页/关键词查询玩家 |
| GET | `/api/players/:id` | 玩家详情 |
| POST | `/api/online/heartbeat` | 写入 120 秒在线 TTL |
| GET | `/api/online/status` | 查询自己的 online key |
| GET | `/api/leaderboards/:mission_id` | 任务 Top N |
| GET | `/api/leaderboards/:mission_id/me` | 当前玩家个人排名 |
| GET | `/api/me/mission-records` | 当前玩家 settled 战绩 |

### 管理员接口

| 方法 | 路径 | 用途 |
| --- | --- | --- |
| GET | `/api/admin/me` | 当前管理员身份 |
| GET | `/api/admin/dashboard/summary` | 玩家、在线和审计统计 |
| GET | `/api/admin/dashboard/recent-operation-logs` | 最近操作日志 |
| GET | `/api/admin/players` | GM 玩家列表 |
| GET | `/api/admin/players/:id` | GM 玩家详情 |
| POST | `/api/admin/players/:id/ban` | 封禁玩家并事务写审计 |
| POST | `/api/admin/players/:id/unban` | 解封玩家并事务写审计 |
| GET | `/api/admin/operation-log-actions` | 审计动作筛选项 |
| GET | `/api/admin/operation-logs` | 分页/多条件筛选审计 |
| GET | `/api/admin/operation-logs/:id` | 单条审计详情 |
| GET | `/api/admin/realtime/summary` | 当前单体实例近实时摘要 |
| GET | `/api/admin/realtime/players/:id` | 玩家业务上下文聚合 |
| GET | `/api/admin/settlements` | settled 结算分页与筛选 |
| GET | `/api/admin/leaderboards/:mission_id` | GM 任务榜单 |

`GET /ws` 是 HTTP Upgrade 入口，但它的业务消息不属于 OpenAPI；见 [WebSocket 协议](ws-protocol.md)。

## 4. 最小验证流程

### 4.1 健康检查

```powershell
Invoke-RestMethod http://localhost:8080/health
```

预期：HTTP 200、`code=0`。这不证明 MySQL、Redis 或全部业务健康。

### 4.2 注册玩家

```powershell
$body = @{
  username = "player01"
  password = "local-test-password"
  nickname = "Player 01"
} | ConvertTo-Json

Invoke-RestMethod `
  -Method Post `
  -Uri http://localhost:8080/api/register `
  -ContentType application/json `
  -Body $body
```

成功返回 201。重复用户名返回 `40901`。注册在同一事务中写 `players` 和 `player_assets`。

### 4.3 玩家登录和受保护请求

```powershell
$login = Invoke-RestMethod `
  -Method Post `
  -Uri http://localhost:8080/api/login `
  -ContentType application/json `
  -Body (@{username="player01"; password="local-test-password"} | ConvertTo-Json)

$playerHeaders = @{Authorization = "Bearer $($login.data.token)"}
Invoke-RestMethod -Uri http://localhost:8080/api/me -Headers $playerHeaders
```

封禁玩家使用正确密码登录会返回 403、`40321`，响应 `data.reason` 包含封禁原因。

### 4.4 在线状态

```powershell
Invoke-RestMethod `
  -Method Post `
  -Uri http://localhost:8080/api/online/heartbeat `
  -Headers $playerHeaders

Invoke-RestMethod `
  -Uri http://localhost:8080/api/online/status `
  -Headers $playerHeaders
```

HTTP 心跳和 WebSocket 都使用 `online:player:<id>`。WebSocket 断开后停止续期，但 key 在 TTL 到期前可能短时间仍为 online。

### 4.5 分页、筛选和排行榜

```text
GET /api/players?page=1&page_size=10&keyword=player
GET /api/leaderboards/training_ground?limit=10
GET /api/leaderboards/training_ground/me
GET /api/me/mission-records?page=1&page_size=10
```

玩家列表和战绩 `page_size` 最大 50。排行榜每个任务只保留个人最佳分；同分时先达到者优先。排行榜是 Redis 查询投影，MySQL settled 记录和资产才是长期事实。

## 5. 管理员操作

管理员登录后构造：

```powershell
$adminHeaders = @{Authorization = "Bearer <admin-token>"}
```

封禁和解封请求：

```powershell
Invoke-RestMethod `
  -Method Post `
  -Uri http://localhost:8080/api/admin/players/1/ban `
  -Headers $adminHeaders `
  -ContentType application/json `
  -Body (@{reason="local verification"} | ConvertTo-Json)
```

`reason` 去除首尾空白后不能为空，最多 200 个 Unicode 字符。重复封禁返回 `40951`，未封禁玩家执行解封返回 `40961`。玩家状态和危险操作审计在同一事务提交。

操作日志支持：

```text
page / page_size
action
admin_username
target_type / target_id
start_time / end_time（RFC3339）
range=today|last_7_days|last_30_days
```

显式 `start_time`/`end_time` 存在时，快捷 `range` 不生效。

## 6. GM 实时观察边界

`/api/admin/realtime/summary` 和玩家观察仅在显式 `GAMEPLAY_MODE=legacy` 注册，V2返回404。V2使用 `/api/admin/v2/observations/{entity}`，维护状态使用 `/api/admin/v2/control`。旧实时观察依次读取当前进程内存、Redis 和 MySQL：

- 连接、小队、任务来自当前 Go 进程。
- queued ticket 来自 Redis。
- 玩家、余额和 settled 结算来自 MySQL。

响应中的 `observed_at` 是聚合完成时间。结果不是跨存储原子快照，也不是多实例全服指标。高频只读观察只写带 request ID 的标准日志，不写 `admin_operation_logs`；封禁/解封仍写数据库审计。

## 7. 常见错误定位

| 现象 | 优先检查 |
| --- | --- |
| 连接拒绝 | Go 服务端口、`APP_PORT`、进程日志 |
| 401 | Authorization 头、Bearer 格式、token 是否过期 |
| 403 | 是否混用了玩家/管理员 token，玩家是否被封禁 |
| 500 数据库类错误 | MySQL 容器、schema、连接环境变量 |
| 500 在线/匹配/排行错误 | Redis 容器与 `REDIS_ADDR` |
| 分页结果意外 | 实现会对非法值回退默认并将 page_size 截断为上限 |

每个操作的精确业务错误码可在 [OpenAPI](openapi.yaml) 的 `x-error-codes` 查看。

## 8. V2 使用入口

设置 `GAMEPLAY_MODE=v2` 并完成 V2 迁移后，玩家动作走 `/ws` 的 `v2.*` 消息，查询走 `/api/v2/*`。`operation_id` 在同一次动作重试中保持不变；未知执行结果可从 `/api/v2/operations/{operation_id}` 查回。个人任务可选，不兼容绑定允许参战但不累计任务。

`/api/v2/parties/{party_id}/snapshot` 与 `/api/v2/runs/{run_id}/snapshot` 按成员/历史参与者授权；不向队友暴露完整个人任务选择和进度。好友、私聊、未读、招募、任务目录也提供 HTTP 查询。管理员通过 `/api/admin/v2/observations/{entity}` 查看 parties/proposals/runs/tasks/pending，通过 `/api/admin/v2/metrics` 读取 Prometheus 文本。

内部事件入口只在独立 loopback 端口启用，默认关闭；公共 HTTP/WS 与 GM 页面都没有事件写入能力。隔离运行和演示命令见 [V2 运行指南](pve-release-guide.md)。

## 9. 活性与就绪

`GET /health` 只证明 HTTP 进程能响应。`GET /ready` 在一秒共同预算内检查 MySQL/Redis，生命周期取消或依赖故障时返回 503/code=50301，恢复后返回 200。就绪不代表维护状态允许新匹配；准入以 V2 control 为准。本机验收用 `APP_HOST=127.0.0.1` 限定主监听。
