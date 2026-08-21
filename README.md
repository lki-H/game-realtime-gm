# Go 游戏后台与 GM 运营服务

这是一个持续迭代的 Go 后端学习项目，用于实现游戏业务中的玩家服务、管理员运营能力、在线状态和实时小队连接。

当前代码已完成 Day 27：玩家与管理员鉴权、玩家管理、封禁解封、GM 操作日志、Dashboard 统计、Redis 在线状态、WebSocket 连接生命周期、小队基础操作，以及小队状态主动广播。

## 当前边界

- 当前仓库是单体 Go 游戏业务服务，不是完整商业游戏服务器。
- 小队状态保存在 Go 进程内存中，服务重启后会清空。
- WebSocket 当前负责玩家连接、在线状态和小队基础消息。
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

### GM 管理

- 管理员登录和 JWT 鉴权
- 当前管理员信息
- Dashboard 玩家与操作统计
- 玩家列表、详情、封禁和解封
- GM 操作日志记录、分页、筛选和详情
- 封禁状态修改与操作日志使用同一数据库事务

### 在线与实时连接

- 玩家 WebSocket token 鉴权
- 管理员 token 与玩家 token 权限隔离
- 同一玩家重复连接时替换旧连接
- 连接 ID、ping/pong、读写超时和单连接写锁
- Redis 在线状态写入、续期和过期
- 小队创建、加入、准备状态、查询和离开
- 小队成员加入、离开和 ready 变化时，向其他在线成员推送 `squad.state.changed`

## 数据职责

```text
MySQL
  players                 玩家账号、资料和封禁状态
  admins                  管理员账号和角色
  admin_operation_logs    GM 操作审计日志

Redis
  online:player:<id>      当前玩家 WebSocket 在线状态和连接 ID

Go 进程内存
  WebSocket 连接管理
  小队及成员准备状态
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
│       ├── database/        MySQL 连接、schema 和 seed
│       ├── handler/         HTTP 与 WebSocket handler
│       ├── middleware/      玩家和管理员鉴权
│       ├── model/           数据模型
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
POST /api/admin/login
GET  /api/admin/me
GET  /api/admin/dashboard/summary
GET  /api/admin/players
GET  /api/admin/players/:id
POST /api/admin/players/:id/ban
POST /api/admin/players/:id/unban
GET  /api/admin/operation-logs
GET  /api/admin/operation-logs/:id
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

## 项目文档

- `docs/api-overview.md`：当前 HTTP API、WebSocket 消息、错误码和验证步骤。
- `docs/adr/0006-使用MySQL作为主数据库.md`：主数据库迁移到 MySQL 的架构决策。
- `docs/github-workflow.md`：公开仓库的安全提交与推送流程。
