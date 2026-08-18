# Go 游戏后台、GM 运营后台与共斗 PVE 实时服务基础

这是一个持续迭代中的 Go 后端学习项目，目标是模拟游戏业务里的玩家服务、在线状态、实时连接和 GM 管理后台能力。

项目当前重点放在后端工程基础和游戏后台业务闭环：玩家注册登录、JWT 鉴权、管理员登录、管理员权限校验、玩家查询、封禁解封、GM 操作日志、WebSocket 玩家长连接、Redis 在线状态、小队房间基础和接口文档。

后续业务方向会逐步收束到“共斗 PVE 游戏后台与实时服务基础”：先在单体 Go 服务中实现小队房间、任务匹配、副本生命周期、结算记录和 GM 观察能力，再把 UDP/KCP、状态同步、战斗服拆分等内容作为二期研究方向。

## 项目定位

- 面向方向：Go 后端实习、游戏服务端实习、游戏后台/运营工具方向实习。
- 业务场景：游戏玩家基础服务 + GM 运营管理后台 + 共斗 PVE 实时服务基础。
- 学习方式：按 `docs/day/dayXX-plan.md` 每天拆分一个小需求推进。
- 当前进度：已推进到 Day 26，正在从 WebSocket 连接能力进入共斗 PVE 小队房间基础能力。
- 迭代方式：后续新增功能时，也会回头优化已完成模块，例如登录鉴权、GM 操作日志、Redis 在线状态、WebSocket 连接生命周期和小队房间边界。

## 参考方向

项目后续参考分成两类：

```text
开源架构参考：Nakama、Pitaya、Colyseus、Open Match、Agones、Gin-Vue-Admin 等。
真实业务模型参考：AccelByte、PlayFab、AWS GameLift、Epic Online Services、Hathora、Pragma、Centrifugo 等。
```

这些参考只用于学习成熟项目里的命名、边界、状态流转和业务链路，不会在当前阶段直接引入完整平台、云服务、Kubernetes、微服务集群或商业 SDK。

详细说明见：

```text
docs/project-direction-pve.md
docs/project-technical-standards.md
```

## 技术栈

- Go
- Gin
- PostgreSQL 16
- Redis 7
- Docker Compose
- JWT
- bcrypt
- pgx
- go-redis
- WebSocket

## 当前功能

### 玩家侧

- 玩家注册
- 玩家登录
- JWT 玩家鉴权
- 查询当前玩家信息
- 修改玩家昵称
- 玩家列表
- 玩家详情
- Redis 在线心跳
- 在线状态查询
- 被封禁玩家禁止登录

### 实时连接侧

- WebSocket 玩家 token 鉴权
- 管理员 token 禁止连接玩家 WebSocket
- 玩家连接注册到内存连接管理器
- 同一玩家重复连接时，新连接替换旧连接
- welcome 消息返回当前进程在线连接数
- WebSocket 连接成功后写入 Redis 在线状态
- WebSocket 连接保持时续期 Redis TTL
- WebSocket 断开后停止续期，等待 Redis key 自动过期
- WebSocket ping/pong 心跳
- WebSocket 读超时、写超时和单连接写锁
- WebSocket 统一 JSON 消息协议
- WebSocket 连接会话 ID
- 小队房间基础消息：创建小队、加入小队、离开小队、设置准备状态、查询当前小队

### GM 管理侧

- 管理员登录
- JWT 管理员鉴权
- 查询当前管理员信息
- GM 查询玩家列表
- GM 查询玩家详情
- GM 封禁玩家
- GM 解封玩家
- GM 操作日志记录
- GM 操作日志分页查询
- GM 操作日志按操作类型、管理员、目标类型、目标 id 筛选
- GM 操作日志按时间范围筛选
- GM 操作日志快捷范围筛选：今天、最近 7 天、最近 30 天
- GM 操作日志详情查询

## 目录结构

```text
game-realtime-gm/
├── backend/                 # Go 后端服务
│   ├── cmd/
│   │   ├── server/          # 服务入口
│   │   └── tools/           # 辅助工具
│   └── internal/
│       ├── auth/            # JWT、密码相关逻辑
│       ├── cache/           # Redis 连接
│       ├── config/          # 配置加载
│       ├── database/        # 数据库连接和建表 SQL
│       ├── handler/         # HTTP handler
│       ├── middleware/      # 玩家和管理员鉴权中间件
│       ├── model/           # 业务模型
│       ├── router/          # 路由注册
│       ├── squad/           # 小队房间基础逻辑
│       └── ws/              # WebSocket 连接管理
├── deploy/                  # Docker Compose 环境
├── docs/                    # 项目实战文档、接口协议和展示规划
│   ├── adr/                 # 架构决策记录
│   ├── day/                 # 每日学习计划
└── frontend/                # 预留 GM 后台和游戏 Demo
```

## 快速启动

### 1. 启动 PostgreSQL 和 Redis

```powershell
cd E:\game-realtime-gm\deploy
docker compose up -d
docker ps
```

正常情况下会看到：

```text
game_realtime_postgres
game_realtime_redis
```

端口映射：

```text
PostgreSQL: localhost:15432 -> 5432
Redis:      localhost:6379  -> 6379
```

### 2. 初始化数据库表

```powershell
cd E:\game-realtime-gm
docker exec -i game_realtime_postgres psql -U game -d game_realtime < .\backend\internal\database\schema.sql
```

如果使用 PowerShell 重定向遇到问题，也可以进入容器后手动执行 SQL：

```powershell
docker exec -it game_realtime_postgres psql -U game -d game_realtime
```

### 3. 创建默认管理员账号

进入 psql 后执行：

```sql
INSERT INTO admins (username, password_hash, display_name, role)
VALUES ('admin', '$2a$10$KNFOxy/nnJyeCFCDx6keI.ocYSbPiNTtG583zEYpPYwwxzhxAt8HK', '系统管理员', 'super_admin')
ON CONFLICT (username) DO UPDATE
SET password_hash = EXCLUDED.password_hash,
    display_name = EXCLUDED.display_name,
    role = EXCLUDED.role,
    updated_at = NOW();
```

默认登录信息：

```text
username: admin
password: admin123456
```

说明：这是本地开发学习账号，正式部署时必须更换密码和 JWT secret。

### 4. 启动后端

```powershell
cd E:\game-realtime-gm\backend
go run .\cmd\server
```

默认服务地址：

```text
http://localhost:8080
```

健康检查：

```text
GET http://localhost:8080/health
```

## 常用环境变量

项目支持通过环境变量覆盖默认配置：

```text
APP_PORT=8080
DB_HOST=localhost
DB_PORT=15432
DB_USER=game
DB_PASSWORD=game123456
DB_NAME=game_realtime
DB_SSLMODE=disable
REDIS_ADDR=localhost:6379
REDIS_PASSWORD=
REDIS_DB=0
JWT_SECRET=game-realtime-dev-secret
```

本地学习可以使用默认值。公开部署时应使用强密码和独立的 `JWT_SECRET`。

## 接口文档

接口总览见：

```text
docs/api-overview.md
```

WebSocket 消息协议见：

```text
docs/ws-protocol.md
```

前端共享类型约定见：

```text
docs/frontend-api-types.md
```

核心接口包括：

```text
POST /api/register
POST /api/login
POST /api/admin/login
GET  /api/admin/players
POST /api/admin/players/:id/ban
POST /api/admin/players/:id/unban
GET  /api/admin/operation-logs
GET  /api/admin/operation-logs/:id
GET  /api/admin/dashboard/summary
GET  /api/admin/dashboard/recent-operation-logs
GET  /ws
```

## 学习过程记录

项目保留了按天推进的学习文档：

```text
docs/day/day01-plan.md
docs/day/day02-plan.md
...
docs/day/day26-plan.md
```

这些文件当前统一放在：

```text
docs/day/
```

这些文档记录了从基础项目搭建到 GM 后台核心能力的逐步实现过程，适合展示项目的学习路径、需求拆解和问题排查过程。

项目方向说明见：

```text
docs/project-direction-pve.md
```

长期技术规范和参考项目矩阵见：

```text
docs/project-technical-standards.md
```

## 文档导航

项目实战展示入口：

```text
docs/presentation-plan.md
```

GM 后台和游戏 Demo 前端规划：

```text
docs/frontend-plan.md
docs/demo-plan.md
```

演示数据、测试与演示验收：

```text
docs/data-seed-plan.md
docs/test-and-demo-plan.md
```

接口、协议与前端类型：

```text
docs/api-overview.md
docs/ws-protocol.md
docs/frontend-api-types.md
```

架构决策和项目文档：

```text
docs/adr/
```

## 本地验证

后端编译测试：

```powershell
cd E:\game-realtime-gm\backend
go test ./...
```

Docker 服务检查：

```powershell
cd E:\game-realtime-gm\deploy
docker ps
```

数据库表检查：

```powershell
docker exec -it game_realtime_postgres psql -U game -d game_realtime
\dt
```

## 当前阶段说明

这是一个学习和求职展示项目，目前更适合展示：

- Go Web 后端基础能力
- PostgreSQL 表设计和查询能力
- Redis 在线状态能力
- WebSocket 长连接和连接生命周期
- WebSocket 统一消息协议
- 小队房间基础业务
- JWT 鉴权和权限区分
- GM 后台业务建模
- 操作日志审计思路
- 共斗 PVE 小队、匹配、任务副本等后续业务设计能力
- 从成熟游戏后端平台中提炼 Party、Presence、Matchmaking、Game Session 等概念，并映射到本项目的小队、在线状态、PVE 匹配和任务副本
- Docker 本地开发环境搭建
- 持续学习和文档化能力

后续计划：

- 小队状态广播
- 任务副本状态机
- PVE 匹配 ticket、Redis 队列、匹配超时和取消匹配
- 任务结算记录、奖励记录、幂等 key 和 nonce 防重放
- 排行榜、战绩查询和 GM 实时观察
- WebSocket 限流、错误响应、消息大小限制和封禁踢线
- `ws_bot` 压测、`go test -race` 并发检查、基础指标和日志链路
- 更完整的接口测试和 WebSocket 测试
- README、架构图、数据流图和简历表达阶段复盘
- React GM 后台轻量版
- React + Phaser 轻量游戏 Demo
- 服务器部署和线上演示环境
