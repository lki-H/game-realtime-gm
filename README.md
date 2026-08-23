# Go 游戏后台与 GM 运营服务

这是一个持续迭代的 Go 后端学习项目，用于实现游戏业务中的玩家服务、管理员运营能力、在线状态和实时小队连接。

当前代码已完成 Day 34：新增安全访问日志、WebSocket 多玩家流程工具、本机 pprof、Docker Linux race 检查和可复现性能基线。

## 当前边界

- 当前仓库是单体 Go 游戏业务服务，不是完整商业游戏服务器。
- 小队状态保存在 Go 进程内存中，服务重启后会清空。
- WebSocket 当前负责玩家连接、在线状态、小队、任务会话、匹配票据和结算记录消息。
- MySQL 使用三层唯一约束保护结算幂等，并在同一事务中提交任务结果、granted reward、玩家余额和资产流水。
- GM 实时摘要是当前单体进程、Redis 与 MySQL 依次读取形成的近实时视图，不是多实例全服原子快照。
- Day34 的 8/20 客户端结果来自本机单实例回环网络，只作为工程基线，不代表生产容量。
- pprof 默认关闭，显式启用时只监听独立本机端口，不进入业务 API。
- 当前尚未提供危险 GM 实时命令、完整资产流水查询、排行榜主动广播或服务重启后的任务恢复。
- 当前不包含逐帧战斗模拟、物理或技能判定、怪物 AI、客户端预测及商业级网络同步。

## 技术栈

- Go 1.25.6
- Gin 1.12
- MySQL 8.4.11 LTS
- Redis 7
- Docker Compose
- `database/sql` + `go-sql-driver/mysql`
- JWT + bcrypt
- `go-redis`
- Gorilla WebSocket

## 已实现功能

### 玩家服务

- 玩家注册和登录
- JWT 玩家鉴权
- 查询和修改当前玩家资料
- 玩家列表和玩家详情
- 被封禁玩家禁止登录
- 玩家注册与零余额 `player_assets` 行在同一个 MySQL 事务中创建

### GM 管理

- 管理员登录和 JWT 鉴权
- 当前管理员信息
- Dashboard 玩家与操作统计
- 玩家列表、详情、封禁和解封
- GM 操作日志记录、分页、筛选和详情
- 封禁状态修改与操作日志使用同一数据库事务
- 当前实例的连接、小队、匹配、任务和 settled 结算摘要
- 玩家在线、小队、任务、匹配、余额和最近结算聚合观察
- settled 结算分页筛选和管理员排行榜查询
- 所有 HTTP 响应返回经校验的 `X-Request-ID`；GM 观察标准日志关联管理员身份
- 只读观察不写入 `admin_operation_logs`，危险写操作继续保留数据库审计

### 在线与实时连接

- 玩家 WebSocket token 鉴权
- 管理员 token 与玩家 token 权限隔离
- 同一玩家重复连接时替换旧连接
- 连接 ID、ping/pong、读写超时和单连接写锁
- Redis 在线状态写入、续期和过期
- 小队创建、加入、准备状态、查询和离开
- 小队成员加入、离开和 ready 变化时，向其他在线成员推送 `squad.state.changed`
- 离队会真正移除成员，最后一名成员离开时解散小队，并有回归测试保护
- AccessLog 只记录 URL path，不记录 WebSocket query token
- 服务端不再逐条记录高频原始 WebSocket payload
- 断线成员保留在小队并设置 `online=false`、`ready=false`
- 队长断线或离队后转移给最早加入的在线成员
- 任务会话创建、ready、开始、结束、取消和查询
- `waiting -> ready -> running -> finished` 与 `waiting -> canceled` 合法状态跳转
- `mission.state.changed` 任务会话主动广播
- mission、squad 和 WebSocket 连接替换单元测试
- Redis Hash 保存 ticket，任务 ZSet 保存队列顺序，全局 ZSet 保存超时索引
- `matchmaking.enqueue`、`matchmaking.me` 和 `matchmaking.cancel`
- `queued -> canceled/timeout` 合法状态变化
- 30 秒本地演示超时和 `matchmaking.state.changed` 通知
- canceled/timeout 从两个 ZSet 移除，终态 ticket 短期保留
- 当前明确未实现 `matched` 和真正撮合算法
- `settlement.create` 只允许当前小队队长为 finished 任务创建或重试幂等结算
- 服务端计算通关耗时、分数和固定奖励，忽略客户端额外提交的 score/reward 字段
- `mission_instance_id`、`idempotency_key` 和玩家 nonce 三层唯一约束
- 相同请求或同一任务换 key 重试时返回已有结果，不重复发奖
- `mission_records.status=settled`，每名参与者生成一条 `reward_records.status=granted`
- `SELECT ... FOR UPDATE` 锁定资产行，任务、reward、余额和 ledger 同事务提交或回滚
- 其他在线任务参与者只在首次结算时收到 `settlement.created`
- 结算成功后 best-effort 同步 Redis 排行榜，幂等重试可修复投影
- 每个任务模板只保留每名玩家个人最佳分，同分时先达到者优先
- HTTP 查询排行榜 Top N、当前玩家排名和 MySQL settled 战绩分页

### 工程验证

- `cmd/tools/ws_bot` 自动执行注册/登录、并发建连、小队 create/join/ready、`debug.echo`、hold 和 leave
- 按阶段输出 success、failure、Average、P95、Maximum、错误类别和消息类型
- 8 客户端冒烟和 20 客户端、10000 echo 本机基线均为 0 失败
- CPU、heap 和 goroutine 使用独立本机 pprof 采样
- 官方 Go Linux Docker 镜像中的 `go test -race ./...` 全部通过

## 数据职责

```text
MySQL
  players                 玩家账号、资料和封禁状态
  admins                  管理员账号和角色
  admin_operation_logs    GM 操作审计日志
  mission_records         finished 任务的幂等结算记录
  reward_records          每名参与者的 granted 奖励记录
  player_assets           玩家当前 soft_currency 余额
  asset_ledger            append-only 资产变更流水

Redis
  online:player:<id>      当前玩家 WebSocket 在线状态和连接 ID
  matchmaking:*           匹配票据、任务等待队列、超时索引和玩家索引
  leaderboard:{...}:*     每个任务的最佳分 ZSet 和玩家 member 索引

Go 进程内存
  WebSocket 连接管理
  小队及成员准备状态
  任务会话状态机
```

## 目录结构

```text
game-realtime-gm/
├── backend/
│   ├── cmd/
│   │   ├── server/          服务入口
│   │   └── tools/           辅助工具
│   └── internal/
│       ├── auth/            JWT 逻辑
│       ├── cache/           Redis 连接
│       ├── config/          环境变量配置
│       ├── database/        MySQL 连接、schema、seed 和 migration
│       ├── diagnostics/     默认关闭的本机 pprof 诊断服务
│       ├── handler/         HTTP 与 WebSocket handler
│       ├── leaderboard/     Redis 最佳分排行、同分排序和 MySQL 战绩查询
│       ├── matchmaking/     Redis 匹配票据、取消和超时
│       ├── middleware/      玩家和管理员鉴权
│       ├── model/           数据模型
│       ├── mission/         任务会话业务状态机
│       ├── observation/     GM 单实例摘要、玩家上下文和结算查询
│       ├── settlement/      幂等结算、资产强事务和已有结果查询
│       ├── router/          路由注册
│       ├── squad/           小队内存状态
│       └── ws/              WebSocket 协议与连接管理
├── deploy/                  Docker Compose 环境
└── docs/                    API、架构决策和 Git 工作流文档
```

## 快速启动

### 1. 启动 MySQL 和 Redis

```powershell
cd .\deploy
docker compose up -d
docker compose ps
```

正常情况下可以看到：

```text
game_realtime_mysql
game_realtime_redis
```

端口：

```text
MySQL: localhost:3306
Redis: localhost:6379
```

### 2. 初始化数据库

在项目根目录执行：

```powershell
cd ..
Get-Content .\backend\internal\database\schema.sql -Raw |
  docker exec -i game_realtime_mysql mysql -ugame -pgame123456 game_realtime

Get-Content .\backend\internal\database\seed.sql -Raw |
  docker exec -i game_realtime_mysql mysql -ugame -pgame123456 game_realtime
```

如果本地数据库已经运行过 Day30 schema，只执行一次 Day31 增量迁移：

```powershell
Get-Content .\backend\internal\database\migrations\day31_settlement_assets.sql -Raw |
  docker exec -i game_realtime_mysql mysql -ugame -pgame123456 game_realtime
```

全新数据库只需要执行最新 `schema.sql`，不要再重复执行 Day31 migration。

默认管理员仅用于本地开发：

```text
username: admin
password: admin123456
```

### 3. 启动 Go 服务

```powershell
cd .\backend
go run .\cmd\server
```

默认地址：

```text
http://localhost:8080
```

健康检查：

```text
GET http://localhost:8080/health
```

## 环境变量

```text
APP_PORT=8080
PPROF_ENABLED=false
PPROF_ADDR=127.0.0.1:6060
DB_HOST=localhost
DB_PORT=3306
DB_USER=game
DB_PASSWORD=game123456
DB_NAME=game_realtime
REDIS_ADDR=localhost:6379
REDIS_PASSWORD=
REDIS_DB=0
JWT_SECRET=game-realtime-dev-secret
```

以上值只用于本地开发。公网部署必须使用独立强密码和随机 `JWT_SECRET`，并通过未跟踪的环境配置注入。

## 主要接口

```text
POST /api/register
POST /api/login
GET  /api/me
PATCH /api/me/nickname
GET  /api/leaderboards/:mission_id
GET  /api/leaderboards/:mission_id/me
GET  /api/me/mission-records
POST /api/admin/login
GET  /api/admin/me
GET  /api/admin/dashboard/summary
GET  /api/admin/players
GET  /api/admin/players/:id
POST /api/admin/players/:id/ban
POST /api/admin/players/:id/unban
GET  /api/admin/operation-logs
GET  /api/admin/operation-logs/:id
GET  /api/admin/realtime/summary
GET  /api/admin/realtime/players/:id
GET  /api/admin/settlements
GET  /api/admin/leaderboards/:mission_id
GET  /ws
```

完整请求、响应和 WebSocket 消息示例见 `docs/api-overview.md`。

## 验证

```powershell
cd .\backend
go test ./...
go vet ./...

cd ..\deploy
docker compose config
```

当前 Windows 环境没有 GCC。Day34 使用官方 Go Linux 镜像完成 race：

```powershell
cd .\backend
docker run --rm -v "${PWD}:/workspace" -w /workspace golang:1.25-bookworm go test -race ./...
```

## 项目文档

- `docs/api-overview.md`：当前 HTTP API、WebSocket 消息、错误码和验证步骤。
- `docs/performance/day34-baseline.md`：Day34 冒烟、负载、pprof、race 和数据清理实测记录。
- `docs/adr/0006-使用MySQL作为主数据库.md`：主数据库迁移到 MySQL 的架构决策。
- `docs/github-workflow.md`：公开仓库的安全提交与推送流程。
