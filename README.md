# Go 实时游戏服务与 GM 运营后台

这是一个持续迭代中的 Go 后端学习项目，目标是模拟游戏业务里的玩家服务、在线状态和 GM 管理后台能力。

项目当前重点放在后端工程基础和 GM 后台业务闭环：玩家注册登录、JWT 鉴权、管理员登录、管理员权限校验、玩家查询、封禁解封、GM 操作日志记录、日志筛选和日志详情查询。

## 项目定位

- 面向方向：Go 后端实习、游戏服务端实习。
- 业务场景：游戏玩家基础服务 + GM 运营管理后台。
- 学习方式：按 `docs/dayXX-plan.md` 每天拆分一个小需求推进。
- 当前进度：已完成 Day 01 到 Day 15。

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
│       ├── database/        # PostgreSQL 连接和建表 SQL
│       ├── handler/         # HTTP handler
│       ├── middleware/      # 玩家和管理员鉴权中间件
│       ├── model/           # 业务模型
│       └── router/          # 路由注册
├── deploy/                  # Docker Compose 环境
├── docs/                    # 学习计划、项目文档、交接文档
└── frontend/                # 预留前端管理端目录
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
```

## 学习过程记录

项目保留了按天推进的学习文档：

```text
docs/day01-plan.md
docs/day02-plan.md
...
docs/day15-plan.md
```

这些文档记录了从基础项目搭建到 GM 后台核心能力的逐步实现过程，适合展示项目的学习路径、需求拆解和问题排查过程。

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
- JWT 鉴权和权限区分
- GM 后台业务建模
- 操作日志审计思路
- Docker 本地开发环境搭建
- 持续学习和文档化能力

后续计划：

- GM 后台 dashboard 统计接口
- 操作日志筛选选项接口
- 更完整的接口测试用例
- 前端管理端页面
- 服务器部署和线上演示环境
