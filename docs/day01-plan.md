# Day 01 任务计划：项目初始化与环境验证

## 今日目标

今天只做一件事：把项目地基搭起来。

完成后应达到：

- 项目目录结构清晰
- Go 后端可以启动
- `/health` 接口可以访问
- MySQL / Redis 可以通过 Docker Compose 启动
- README 有基本说明
- Git 有第一条提交记录

今天不要做登录、WebSocket、React 页面、匹配系统、排行榜、K8s。

## 1. 环境确认

在 PowerShell 中确认：

```powershell
go version
git --version
docker --version
docker compose version
node -v
npm -v
pnpm -v
```

Go 相关配置建议确认：

```powershell
go env GOPROXY GOPATH GOROOT GOCACHE
```

期望结果：

```text
GOPROXY = https://goproxy.cn,direct
GOPATH  = E:\GoPath
GOROOT  = D:\GO
GOCACHE = E:\DevCache\go-build
```

如果当前终端仍显示旧的 `GOPATH=D:\GO`，关闭 PowerShell / IDE 后重新打开。

## 2. 项目目录

项目根目录：

```text
E:\game-realtime-gm
```

当前结构：

```text
game-realtime-gm/
├── backend/
├── frontend/
├── deploy/
├── docs/
└── README.md
```

今天主要工作在：

```text
E:\game-realtime-gm\backend
E:\game-realtime-gm\deploy
```

## 3. 后端初始化

进入后端目录：

```powershell
cd E:\game-realtime-gm\backend
```

`go.mod` 已经存在，不需要重复执行 `go mod init`。

已完成依赖：

```powershell
go get github.com/gin-gonic/gin
```

今天建议新增目录：

```text
backend/
├── cmd/
│   └── server/
│       └── main.go
├── internal/
│   ├── handler/
│   │   └── health.go
│   └── router/
│       └── router.go
├── go.mod
└── go.sum
```

### 3.1 main.go

位置：

```text
backend/cmd/server/main.go
```

职责：

- 创建路由
- 启动 HTTP 服务
- 监听 `:8080`

### 3.2 health.go

位置：

```text
backend/internal/handler/health.go
```

职责：

- 提供健康检查接口

接口：

```text
GET /health
```

返回：

```json
{
  "code": 0,
  "message": "ok"
}
```

### 3.3 router.go

位置：

```text
backend/internal/router/router.go
```

职责：

- 注册路由
- 暂时只注册 `/health`

## 4. 运行后端

在 `backend` 目录执行：

```powershell
go run .\cmd\server
```

浏览器或 Apifox 访问：

```text
http://localhost:8080/health
```

验收返回：

```json
{
  "code": 0,
  "message": "ok"
}
```

## 5. Docker Compose

今天先准备 MySQL 和 Redis。

文件位置：

```text
deploy/docker-compose.yml
```

建议服务：

- PostgreSQL 或 MySQL 二选一
- Redis

因为你本机已经装了 PostgreSQL，但为了项目最终可复现，建议 Docker Compose 里也写一份数据库服务。

今日建议先用 PostgreSQL：

```yaml
services:
  postgres:
    image: postgres:16
    container_name: game_realtime_postgres
    environment:
      POSTGRES_USER: game
      POSTGRES_PASSWORD: game123456
      POSTGRES_DB: game_realtime
    ports:
      - "5432:5432"
    volumes:
      - postgres_data:/var/lib/postgresql/data

  redis:
    image: redis:7
    container_name: game_realtime_redis
    ports:
      - "6379:6379"
    volumes:
      - redis_data:/data

volumes:
  postgres_data:
  redis_data:
```

启动：

```powershell
cd E:\game-realtime-gm\deploy
docker compose up -d
```

查看：

```powershell
docker ps
```

停止：

```powershell
docker compose down
```

注意：如果本机 PostgreSQL 已占用 `5432`，可以把 Docker 端口改成：

```yaml
ports:
  - "15432:5432"
```

## 6. README 今日补充

根目录 `README.md` 今天至少补充：

- 项目目标
- 技术栈
- 目录结构
- 后端启动命令
- Docker Compose 启动命令
- `/health` 接口说明

## 7. Git 初始化

在项目根目录执行：

```powershell
cd E:\game-realtime-gm
git init
git status
```

建议新增 `.gitignore`：

```text
.idea/
.vscode/
*.log
*.tmp
node_modules/
dist/
build/
.env
```

提交：

```powershell
git add .
git commit -m "chore: initialize project structure"
```

## 8. 今日验收清单

完成以下内容即可收工：

- [ ] 环境命令全部能正常输出
- [ ] `backend` 有可运行的 Go 服务
- [ ] `/health` 能返回 `code=0`
- [ ] `deploy/docker-compose.yml` 已创建
- [ ] Docker 可以启动 PostgreSQL/Redis
- [ ] README 已补充基础说明
- [ ] Git 已初始化
- [ ] 完成第一条提交

## 9. 今日不做

今天不要做：

- 用户注册登录
- JWT
- WebSocket
- 房间系统
- 匹配系统
- Redis 业务逻辑
- React 页面
- GM 后台
- Prometheus / Grafana
- Kubernetes

这些从 Day 02 以后再开始。

## 10. 明日预告

Day 02 目标：

- 连接 PostgreSQL
- 设计玩家/账号表
- 实现注册接口
- 实现登录接口
- 引入 JWT
- 用 Apifox 测试接口
