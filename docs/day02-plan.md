# Day 02 学习计划：配置、数据库连接、注册登录地基

## 今日目标

今天要把后端从“只有 `/health` 的空服务”，推进到“能读取配置、能连接 PostgreSQL，并准备好注册/登录接口的基础结构”。

最终主线是：

```text
Docker 启动 PostgreSQL/Redis
        ↓
Go 后端读取配置
        ↓
Go 后端连接 PostgreSQL
        ↓
创建玩家账号表
        ↓
实现注册接口
        ↓
实现登录接口并返回 JWT
```

如果今天时间有限，优先完成到“数据库连接成功 + 建表成功”。注册、登录、JWT 可以作为 Day 02 后半段或 Day 03 的衔接。

## 当前项目状态

你现在已经完成：

- Go 后端可以启动。
- Gin 路由已经存在。
- `/health` 接口已经存在。
- `deploy/docker-compose.yml` 已经存在。
- `backend/internal/config/config.go` 已经创建。
- `backend/cmd/server/main.go` 已经改成从配置读取端口。

所以 Day 02 不需要从零开始，而是在已有骨架上继续往下接。

## 今天会学到什么

- Docker Compose 如何启动 PostgreSQL 和 Redis。
- 为什么后端要有配置模块。
- Go 程序如何读取环境变量。
- Go 后端如何连接 PostgreSQL。
- 数据库表为什么要设计字段、主键、唯一约束。
- 注册接口为什么要保存密码哈希，而不是明文密码。
- 登录接口为什么需要 JWT。

## 今日最终效果

完成后，项目至少应该做到：

- `GET /health` 仍然正常。
- 后端启动时可以读取配置。
- PostgreSQL 容器可以运行。
- Go 后端可以连接 PostgreSQL。
- 数据库里有 `players` 表。

如果进度顺利，继续做到：

- `POST /api/register` 可以注册账号。
- `POST /api/login` 可以登录并返回 JWT。
- Apifox 可以测通注册和登录。

## 任务 1：启动 PostgreSQL 和 Redis

### 任务目标

用 Docker Compose 启动项目依赖的 PostgreSQL 和 Redis。

PostgreSQL 用来保存长期数据，例如玩家账号、战绩、对局记录。

Redis 后面用来保存实时状态，例如在线玩家、匹配队列、排行榜。今天可以先启动它，但暂时不写 Redis 业务逻辑。

### 你要新增或修改什么

今天这个任务不需要新增文件，因为文件已经存在：

```text
E:\game-realtime-gm\deploy\docker-compose.yml
```

你要做的是启动 Docker Desktop，然后执行 Docker Compose 命令。

### 这个操作有什么用

后端服务本身只是业务代码。真正的项目还需要数据库、中间件等外部服务。

如果每个人都手动安装 PostgreSQL 和 Redis，环境很容易不一致。Docker Compose 的作用是把这些依赖服务写进一个配置文件里，让别人可以用同一条命令启动同样的环境。

这对简历项目很重要，因为项目不仅要“你电脑上能跑”，还要“别人按照 README 也能跑”。

### 这个文件负责什么

`deploy/docker-compose.yml` 负责描述项目运行时依赖哪些外部服务。

现在它负责：

- 启动 PostgreSQL。
- 启动 Redis。
- 设置 PostgreSQL 的账号、密码、数据库名。
- 把容器端口映射到本机端口。
- 用 Docker volume 保存数据，避免容器删除后数据直接丢失。

以后它可能还会扩展：

- 后端服务容器。
- 前端服务容器。
- Prometheus/Grafana 监控服务。
- 数据库初始化脚本。

### 具体操作步骤

先打开 Docker Desktop，等待它启动完成。

然后在 PowerShell 执行：

```powershell
cd E:\game-realtime-gm\deploy
docker compose up -d
docker ps
```

命令分析：

- `cd E:\game-realtime-gm\deploy`：进入 `deploy` 目录，因为 `docker-compose.yml` 在这里。
- `docker compose up -d`：按照 `docker-compose.yml` 启动服务，`-d` 表示后台运行。
- `docker ps`：查看当前正在运行的容器。

成功后应该看到类似容器：

```text
game_realtime_postgres
game_realtime_redis
```

### 配置内容

当前 `docker-compose.yml` 内容是：

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
      - "15432:5432"
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

### 配置逐段分析

```yaml
services:
```

表示下面要定义一组服务。这里的服务不是 Go 代码里的 service，而是 Docker 要启动的容器服务。

```yaml
postgres:
```

定义一个名为 `postgres` 的服务。它负责运行 PostgreSQL 数据库。

```yaml
image: postgres:16
```

表示使用官方 PostgreSQL 16 镜像。镜像可以理解为“别人已经打包好的 PostgreSQL 安装包和运行环境”。

```yaml
container_name: game_realtime_postgres
```

指定容器名称。这样用 `docker ps` 查看时比较清楚，不会出现随机名字。

```yaml
environment:
  POSTGRES_USER: game
  POSTGRES_PASSWORD: game123456
  POSTGRES_DB: game_realtime
```

这些是 PostgreSQL 容器启动时需要的环境变量。

- `POSTGRES_USER`：数据库用户名。
- `POSTGRES_PASSWORD`：数据库密码。
- `POSTGRES_DB`：启动时自动创建的数据库名。

```yaml
ports:
  - "15432:5432"
```

表示端口映射。

左边的 `15432` 是你电脑本机端口，右边的 `5432` 是容器内部端口。写成这样后，Go 后端可以通过 `localhost:15432` 明确连接容器里的 PostgreSQL，也不会和本机已经安装的 PostgreSQL 冲突。

```yaml
volumes:
  - postgres_data:/var/lib/postgresql/data
```

表示把 PostgreSQL 的数据保存到 Docker volume 里。否则容器删除后，数据库数据可能也没了。

```yaml
redis:
```

定义 Redis 服务。

```yaml
image: redis:7
```

表示使用官方 Redis 7 镜像。

```yaml
ports:
  - "6379:6379"
```

表示把 Redis 容器端口映射到本机 `6379`。后续 Go 后端可以通过 `localhost:6379` 连接 Redis。

### 和项目其他部分的关系

后续 `backend/internal/config/config.go` 里的数据库配置需要和这里保持一致。

例如 Docker Compose 里数据库用户是：

```text
game
```

那么 Go 配置里的 `DB_USER` 默认值也应该是：

```text
game
```

否则后端连接数据库时会因为账号、密码、数据库名不匹配而失败。

### 怎么验证成功

执行：

```powershell
docker ps
```

看到 `game_realtime_postgres` 和 `game_realtime_redis` 正在运行，就说明容器启动成功。

也可以执行：

```powershell
docker compose ps
```

如果状态是 `running`，说明服务正在运行。

### 常见错误

错误 1：

```text
failed to connect to the docker API
```

原因：Docker Desktop 没有启动，或者 Docker Engine 还没准备好。

解决：打开 Docker Desktop，等左下角或界面显示 Docker 已运行后再执行命令。

错误 2：

```text
Bind for 0.0.0.0:5432 failed: port is already allocated
```

原因：你本机已经有 PostgreSQL 占用了 `5432` 端口。

解决：把 `docker-compose.yml` 里的端口改成：

```yaml
ports:
  - "15432:5432"
```

然后 Go 配置里的 `DB_PORT` 也要改成 `15432`。

## 任务 2：后端读取配置

### 任务目标

让 Go 后端启动时从统一的配置模块读取端口、数据库信息和 JWT 密钥。

你已经基本完成这个任务。今天要做的是对照检查、理解代码，并用环境变量测试它是否真的生效。

### 你要新增或修改什么

已经新增：

```text
E:\game-realtime-gm\backend\internal\config\config.go
```

已经修改：

```text
E:\game-realtime-gm\backend\cmd\server\main.go
```

如果你的文件内容和下面一致，可以不用再改。

### 这个操作有什么用

端口、数据库地址、数据库账号、JWT 密钥这些信息都属于“配置”。

配置的特点是：不同环境可能不一样。

例如：

- 本地 Docker 开发数据库在 `localhost:15432`。
- 服务器上的数据库可能在另一个内网地址。
- 本地 JWT 密钥可以简单一点。
- 线上 JWT 密钥必须更复杂、更安全。

如果把这些信息写死在业务代码里，每次换环境都要改代码。这不利于维护，也不安全。

配置模块的作用就是：把这些会变化的信息集中管理。

### 这个文件负责什么

`backend/internal/config/config.go` 负责读取配置。

现在它负责：

- 读取后端服务端口。
- 读取 PostgreSQL 连接信息。
- 读取 JWT 密钥。
- 如果环境变量不存在，就使用本地开发默认值。

以后它可能扩展：

- Redis 配置。
- 日志级别配置。
- 跨域配置。
- Token 过期时间配置。

`backend/cmd/server/main.go` 是程序入口。

后端启动时最先执行 `main.go`，所以在这里调用 `config.Load()` 是合理的。

### 具体操作步骤

先确认文件存在：

```powershell
cd E:\game-realtime-gm\backend
dir internal\config
```

应该看到：

```text
config.go
```

然后运行后端：

```powershell
go run .\cmd\server
```

正常情况下应该看到：

```text
server listening on :8080
```

再测试环境变量是否生效：

```powershell
$env:APP_PORT="8081"
go run .\cmd\server
```

这时应该看到：

```text
server listening on :8081
```

测试完清理环境变量：

```powershell
Remove-Item Env:APP_PORT
```

### 代码内容：`config.go`

```go
package config

import "os"

type Config struct {
	AppPort   string
	Database  DatabaseConfig
	JWTSecret string
}

type DatabaseConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	Name     string
	SSLMode  string
}

func Load() Config {
	return Config{
		AppPort: getEnv("APP_PORT", "8080"),
		Database: DatabaseConfig{
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnv("DB_PORT", "15432"),
			User:     getEnv("DB_USER", "game"),
			Password: getEnv("DB_PASSWORD", "game123456"),
			Name:     getEnv("DB_NAME", "game_realtime"),
			SSLMode:  getEnv("DB_SSLMODE", "disable"),
		},
		JWTSecret: getEnv("JWT_SECRET", "game-realtime-dev-secret"),
	}
}

func getEnv(key string, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}
```

### 代码逐段分析：`config.go`

```go
package config
```

表示这个文件属于 `config` 包。其他地方可以通过导入 `internal/config` 来使用这里的配置读取能力。

```go
import "os"
```

引入 Go 标准库 `os`。这里主要用 `os.Getenv()` 读取环境变量。

```go
type Config struct {
	AppPort   string
	Database  DatabaseConfig
	JWTSecret string
}
```

`Config` 是整个后端配置的总结构。

- `AppPort`：后端 HTTP 服务监听端口。
- `Database`：数据库配置，里面再细分 host、port、user 等字段。
- `JWTSecret`：JWT 签名密钥，后续登录接口生成 token 时会用到。

```go
type DatabaseConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	Name     string
	SSLMode  string
}
```

`DatabaseConfig` 专门保存数据库连接信息。

- `Host`：数据库地址，本地 Docker 一般是 `localhost`。
- `Port`：后端访问数据库时使用的本机端口。PostgreSQL 容器内部默认是 `5432`，但这里使用映射后的 `15432`，避免和本机 PostgreSQL 冲突。
- `User`：数据库用户名。
- `Password`：数据库密码。
- `Name`：数据库名。
- `SSLMode`：PostgreSQL SSL 模式，本地开发通常用 `disable`。

```go
func Load() Config {
```

`Load` 是配置模块对外暴露的主要函数。后续启动程序时调用它，就能拿到完整配置。

```go
AppPort: getEnv("APP_PORT", "8080"),
```

表示优先读取环境变量 `APP_PORT`。如果没有配置，就默认使用 `8080`。

```go
Host: getEnv("DB_HOST", "localhost"),
```

表示数据库地址默认是 `localhost`。因为 Docker Compose 把 PostgreSQL 端口映射到了本机，所以 Go 后端从本机访问即可。

```go
JWTSecret: getEnv("JWT_SECRET", "game-realtime-dev-secret"),
```

表示 JWT 密钥也来自配置。现在默认值用于本地开发，后续真正部署时应该换成更复杂的值。

```go
func getEnv(key string, defaultValue string) string {
```

这是一个小工具函数。

- `key` 是环境变量名称。
- `defaultValue` 是没有配置环境变量时使用的默认值。
- 返回值是最终使用的配置值。

```go
value := os.Getenv(key)
if value == "" {
	return defaultValue
}
return value
```

这段逻辑表示：如果环境变量为空，就返回默认值；否则返回环境变量里的值。

### 代码内容：`main.go`

```go
package main

import (
	"log"
	"net/http"

	"game-realtime-gm/backend/internal/config"
	"game-realtime-gm/backend/internal/router"
)

func main() {
	cfg := config.Load()

	r := router.New()

	srv := &http.Server{
		Addr:    ":" + cfg.AppPort,
		Handler: r,
	}

	log.Println("server listening on :" + cfg.AppPort)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
```

### 代码逐段分析：`main.go`

```go
package main
```

表示这是一个可执行程序入口包。Go 程序要运行，必须有 `package main` 和 `func main()`。

```go
"game-realtime-gm/backend/internal/config"
```

引入刚刚创建的配置模块。

```go
cfg := config.Load()
```

程序启动时读取配置。`cfg` 里现在有端口、数据库配置、JWT 密钥。

```go
r := router.New()
```

创建路由。现在路由里已经有 `/health`。

```go
Addr: ":" + cfg.AppPort,
```

HTTP 服务监听端口不再写死成 `:8080`，而是从配置读取。

### 和项目其他部分的关系

任务 3 连接 PostgreSQL 时，会继续使用：

```go
cfg.Database
```

任务 6 登录签发 JWT 时，会使用：

```go
cfg.JWTSecret
```

所以任务 2 是后面所有基础设施的入口。

### 怎么验证成功

执行：

```powershell
cd E:\game-realtime-gm\backend
go run .\cmd\server
```

访问：

```text
http://localhost:8080/health
```

期望返回：

```json
{
  "code": 0,
  "message": "ok"
}
```

再测试端口环境变量：

```powershell
$env:APP_PORT="8081"
go run .\cmd\server
```

访问：

```text
http://localhost:8081/health
```

能访问就说明配置生效。

### 常见错误

错误 1：

```text
package game-realtime-gm/backend/internal/config is not in std
```

原因：命令执行目录不对，或者 `go.mod` 模块路径和 import 路径不一致。

解决：在 `E:\game-realtime-gm\backend` 下执行：

```powershell
go run .\cmd\server
```

错误 2：

```text
listen tcp :8080: bind: Only one usage of each socket address is normally permitted
```

原因：8080 端口已经被另一个进程占用。

解决：换端口测试：

```powershell
$env:APP_PORT="8081"
go run .\cmd\server
```

## 任务 3：后端连接 PostgreSQL

### 任务目标

让 Go 后端启动时连接 PostgreSQL，并在日志里确认数据库连接成功。

### 你要新增或修改什么

建议新增：

```text
E:\game-realtime-gm\backend\internal\database\database.go
```

建议修改：

```text
E:\game-realtime-gm\backend\cmd\server\main.go
```

还需要新增 Go 依赖：

```text
github.com/jackc/pgx/v5/pgxpool
```

`pgx` 是 Go 里常用的 PostgreSQL 驱动。`pgxpool` 提供连接池能力。

### 这个操作有什么用

注册、登录、玩家信息、战绩都需要保存到数据库。

在写注册接口之前，后端必须先具备数据库连接能力。否则接口收到请求后没有地方保存数据。

连接池的作用是复用数据库连接。每个请求都重新连接数据库会很慢，也浪费资源。连接池会维护一组可复用连接，让后端更适合处理多个请求。

### 这个文件负责什么

`backend/internal/database/database.go` 负责数据库初始化。

现在它负责：

- 根据配置拼接 PostgreSQL 连接字符串。
- 创建数据库连接池。
- 用 `Ping` 检查数据库是否真的可用。
- 把连接池返回给 `main.go`。

以后它可能扩展：

- 关闭数据库连接。
- 初始化数据库迁移。
- 添加数据库健康检查。
- 管理多个数据库连接。

### 具体操作步骤

进入后端目录：

```powershell
cd E:\game-realtime-gm\backend
```

安装 PostgreSQL 驱动：

```powershell
go get github.com/jackc/pgx/v5/pgxpool
```

命令分析：

- `go get` 用来添加 Go 依赖。
- `github.com/jackc/pgx/v5/pgxpool` 是 PostgreSQL 连接池包。
- 执行后 `go.mod` 和 `go.sum` 会更新。

创建目录：

```powershell
mkdir internal\database
```

创建文件：

```text
backend/internal/database/database.go
```

### 代码内容：`database.go`

```go
package database

import (
	"context"
	"fmt"

	"game-realtime-gm/backend/internal/config"

	"github.com/jackc/pgx/v5/pgxpool"
)

func NewPostgresPool(ctx context.Context, cfg config.DatabaseConfig) (*pgxpool.Pool, error) {
	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host,
		cfg.Port,
		cfg.User,
		cfg.Password,
		cfg.Name,
		cfg.SSLMode,
	)

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}

	return pool, nil
}
```

### 代码逐段分析：`database.go`

```go
package database
```

表示这个文件属于 `database` 包。以后和数据库初始化相关的代码可以集中放在这里。

```go
import (
	"context"
	"fmt"
)
```

- `context`：用于控制连接、请求的生命周期。数据库操作通常都会带 `context`。
- `fmt`：这里用于拼接数据库连接字符串。

```go
"game-realtime-gm/backend/internal/config"
```

引入配置模块。数据库连接需要用到 `config.DatabaseConfig`。

```go
"github.com/jackc/pgx/v5/pgxpool"
```

引入 pgx 的连接池包。

```go
func NewPostgresPool(ctx context.Context, cfg config.DatabaseConfig) (*pgxpool.Pool, error) {
```

定义一个函数，用来创建 PostgreSQL 连接池。

- `ctx`：上下文，用来控制数据库连接过程。
- `cfg`：数据库配置。
- `*pgxpool.Pool`：返回连接池。
- `error`：如果连接失败，返回错误。

```go
dsn := fmt.Sprintf(...)
```

`dsn` 是数据库连接字符串，里面包含 host、port、user、password、dbname、sslmode。

```go
pool, err := pgxpool.New(ctx, dsn)
```

创建连接池。如果连接字符串格式错误，或者配置有问题，这里可能返回错误。

```go
if err := pool.Ping(ctx); err != nil {
	pool.Close()
	return nil, err
}
```

`Ping` 用来确认数据库真的能连通。

如果 Ping 失败，先关闭连接池，再返回错误。这样可以避免资源泄漏。

```go
return pool, nil
```

连接成功后返回连接池。

### 修改 `main.go`

把数据库连接接入启动流程。

示例：

```go
package main

import (
	"context"
	"log"
	"net/http"

	"game-realtime-gm/backend/internal/config"
	"game-realtime-gm/backend/internal/database"
	"game-realtime-gm/backend/internal/router"
)

func main() {
	cfg := config.Load()
	ctx := context.Background()

	dbPool, err := database.NewPostgresPool(ctx, cfg.Database)
	if err != nil {
		log.Fatal("connect database failed: ", err)
	}
	defer dbPool.Close()
	log.Println("database connected")

	r := router.New()

	srv := &http.Server{
		Addr:    ":" + cfg.AppPort,
		Handler: r,
	}

	log.Println("server listening on :" + cfg.AppPort)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
```

### 代码逐段分析：`main.go` 修改点

```go
import "context"
```

新增 `context`，用于创建数据库连接时传入上下文。

```go
"game-realtime-gm/backend/internal/database"
```

引入数据库模块。

```go
ctx := context.Background()
```

创建一个基础上下文。程序启动阶段还没有具体 HTTP 请求，所以用 `context.Background()`。

```go
dbPool, err := database.NewPostgresPool(ctx, cfg.Database)
```

用配置里的数据库信息创建连接池。

```go
if err != nil {
	log.Fatal("connect database failed: ", err)
}
```

如果数据库连接失败，程序直接退出。因为数据库不可用时，注册、登录等核心功能都无法正常工作。

```go
defer dbPool.Close()
```

程序退出前关闭数据库连接池。

```go
log.Println("database connected")
```

打印连接成功日志，方便你确认数据库连接阶段已经通过。

### 和项目其他部分的关系

数据库连接池后续会传给注册、登录相关代码。

现在 `router.New()` 还没有接收数据库参数。后面实现注册接口时，可以把路由改成：

```go
r := router.New(dbPool, cfg)
```

这样 handler 就可以通过数据库连接池查询和写入数据。

### 怎么验证成功

先确保 Docker 容器运行：

```powershell
cd E:\game-realtime-gm\deploy
docker compose up -d
docker ps
```

再启动后端：

```powershell
cd E:\game-realtime-gm\backend
go run .\cmd\server
```

期望看到：

```text
database connected
server listening on :8080
```

再访问：

```text
http://localhost:8080/health
```

返回 `code=0`，说明接入数据库后原来的健康检查也没有坏。

### 常见错误

错误 1：

```text
connect: connection refused
```

原因：PostgreSQL 容器没有启动，或者端口不对。

排查：

```powershell
docker ps
```

确认 `game_realtime_postgres` 是否运行。

错误 2：

```text
password authentication failed
```

原因：Go 配置里的用户名或密码和 Docker Compose 里的不一致。

检查：

- `DB_USER` 是否是 `game`
- `DB_PASSWORD` 是否是 `game123456`

错误 3：

```text
database "game_realtime" does not exist
```

原因：数据库名不一致，或者 PostgreSQL volume 是旧数据，启动时没有重新初始化。

排查：

- 检查 `POSTGRES_DB`
- 检查 `DB_NAME`

## 任务 4：创建玩家账号表

### 任务目标

在 PostgreSQL 中创建 `players` 表，用来保存玩家账号信息。

### 你要新增或修改什么

建议新增 SQL 文件：

```text
E:\game-realtime-gm\backend\internal\database\schema.sql
```

今天先用手动执行 SQL 的方式学习表结构。后面再考虑迁移工具。

### 这个操作有什么用

数据库不是随便存 JSON 的地方。真实项目需要先设计表结构，明确每个字段保存什么数据。

注册接口要把用户写进数据库，所以必须先有账号表。

### 这个文件负责什么

`schema.sql` 负责记录数据库表结构。

现在它负责：

- 创建 `players` 表。
- 定义账号字段。
- 定义密码哈希字段。
- 定义用户名唯一约束。
- 定义创建时间和更新时间。

以后它可能扩展：

- GM 管理员表。
- 房间表。
- 对局表。
- 战绩表。
- 操作日志表。

### 具体操作步骤

创建文件：

```text
backend/internal/database/schema.sql
```

写入建表 SQL。

然后进入 PostgreSQL 容器执行：

```powershell
docker exec -it game_realtime_postgres psql -U game -d game_realtime
```

进入 `psql` 后，可以复制 SQL 执行。

也可以以后再学习用命令直接执行 SQL 文件。

### SQL 内容

```sql
CREATE TABLE IF NOT EXISTS players (
    id BIGSERIAL PRIMARY KEY,
    username VARCHAR(64) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    nickname VARCHAR(64) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

### SQL 逐段分析

```sql
CREATE TABLE IF NOT EXISTS players
```

创建一张名为 `players` 的表。

`IF NOT EXISTS` 表示如果表已经存在，就不要重复创建，避免报错。

```sql
id BIGSERIAL PRIMARY KEY
```

`id` 是玩家的唯一编号。

- `BIGSERIAL`：PostgreSQL 自动递增的大整数。
- `PRIMARY KEY`：主键，表示这条记录的唯一身份。

```sql
username VARCHAR(64) NOT NULL UNIQUE
```

`username` 是登录账号。

- `VARCHAR(64)`：最多 64 个字符。
- `NOT NULL`：不能为空。
- `UNIQUE`：不能重复，两个玩家不能用同一个账号。

```sql
password_hash VARCHAR(255) NOT NULL
```

保存密码哈希。

注意：这里不叫 `password`，而叫 `password_hash`，是为了提醒自己不能保存明文密码。

真实项目中，数据库里不应该出现用户原始密码。

```sql
nickname VARCHAR(64) NOT NULL
```

保存玩家昵称。昵称可以和登录账号分开。

```sql
created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
```

记录账号创建时间。

`TIMESTAMPTZ` 是带时区的时间类型。

```sql
updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
```

记录账号最后更新时间。

后续修改昵称、封禁状态、玩家资料时，可以更新这个字段。

### 和项目其他部分的关系

注册接口会向 `players` 表插入数据。

登录接口会根据 `username` 查询 `players` 表，再校验 `password_hash`。

GM 后台以后也会查询 `players` 表展示玩家列表。

### 怎么验证成功

进入 PostgreSQL：

```powershell
docker exec -it game_realtime_postgres psql -U game -d game_realtime
```

执行：

```sql
\dt
```

应该看到 `players` 表。

再执行：

```sql
\d players
```

应该看到字段：

```text
id
username
password_hash
nickname
created_at
updated_at
```

### 常见错误

错误 1：

```text
relation "players" already exists
```

原因：表已经存在。

解决：使用 `CREATE TABLE IF NOT EXISTS` 可以避免这个错误。

错误 2：

```text
psql: command not found
```

原因：你在本机 PowerShell 里直接执行了 `psql`，但本机没有配置 psql。

解决：使用 Docker 容器里的 psql：

```powershell
docker exec -it game_realtime_postgres psql -U game -d game_realtime
```

## 任务 5：实现注册接口

### 任务目标

实现 `POST /api/register`，让用户可以注册账号，并把账号保存到 PostgreSQL。

完成后，你可以用 Apifox 或 PowerShell 发送一段 JSON：

```json
{
  "username": "test001",
  "password": "123456",
  "nickname": "测试玩家"
}
```

后端会把账号写入 `players` 表，但不会保存明文密码。

### 开始前先确认

你应该已经完成：

- PostgreSQL 容器正在运行。
- 后端可以连接 PostgreSQL。
- 数据库里已经有 `players` 表。
- `backend/internal/database/schema.sql` 已经记录建表 SQL。

在 Docker Desktop 的 PostgreSQL 容器终端里，可以输入：

```bash
psql -U game -d game_realtime
```

进入 `psql` 后输入：

```sql
\d players
```

如果能看到 `id`、`username`、`password_hash`、`nickname`、`created_at`、`updated_at`，就可以继续。

### 你要新增或修改什么

新增或填写：

```text
backend/internal/handler/auth.go
backend/internal/model/player.go
```

修改：

```text
backend/internal/router/router.go
backend/cmd/server/main.go
```

安装依赖：

```text
golang.org/x/crypto/bcrypt
```

### 这个操作有什么用

注册接口是账号系统的入口。

它把用户提交的账号、密码、昵称变成数据库中的一条玩家记录。

后续 WebSocket、房间、匹配、排行榜都需要知道“当前用户是谁”。所以账号系统是实时游戏服务的前置基础。

### 这个文件负责什么

`model/player.go` 负责描述“玩家数据”在 Go 代码中的样子。

它和数据库里的 `players` 表相互对应：

```text
PostgreSQL players 表
        ↓
Go model.Player 结构体
```

`handler/auth.go` 负责账号相关 HTTP 接口。

它会：

- 接收请求 JSON。
- 检查请求参数。
- 使用 bcrypt 生成密码哈希。
- 使用数据库连接池执行 SQL。
- 返回 JSON 响应。

以后可能扩展：

- 登录接口。
- 刷新 token。
- 修改密码。
- 退出登录。

`router.go` 负责注册接口路径。它要告诉 Gin：

```text
收到 POST /api/register
        ↓
交给 authHandler.Register 处理
```

`main.go` 是程序入口。它已经创建了数据库连接池，所以需要把连接池传给 `router.New(dbPool)`。

### 第 1 步：安装 bcrypt 依赖

在后端目录执行：

```powershell
cd E:\game-realtime-gm\backend
go get golang.org/x/crypto/bcrypt
```

命令分析：

- `cd E:\game-realtime-gm\backend`：进入 Go 模块目录，因为 `go.mod` 在这里。
- `go get`：下载依赖，并把依赖关系记录到 `go.mod` 和 `go.sum`。
- `golang.org/x/crypto/bcrypt`：Go 官方扩展库中的密码哈希包。

注意：不能只输入：

```powershell
golang.org/x/crypto/bcrypt
```

因为这是 Go 包名，不是 PowerShell 命令。

### 第 2 步：填写 `player.go`

文件位置：

```text
E:\game-realtime-gm\backend\internal\model\player.go
```

写入：

```go
package model

import "time"

type Player struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	Nickname     string    `json:"nickname"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
```

#### 代码逐段分析

```go
package model
```

表示这个文件属于 `model` 包。

`model` 可以理解为“项目里的数据模型”。它描述业务数据在 Go 代码里长什么样。

```go
import "time"
```

引入 Go 标准库中的时间类型，因为玩家表里有创建时间和更新时间。

```go
type Player struct {
```

定义一个 `Player` 结构体。

结构体可以理解为：把一组相关字段组合成一个完整的数据对象。

字段关系：

| Go 字段 | 数据库字段 | 作用 |
| --- | --- | --- |
| `ID` | `id` | 玩家唯一编号 |
| `Username` | `username` | 登录账号 |
| `PasswordHash` | `password_hash` | bcrypt 生成的密码哈希 |
| `Nickname` | `nickname` | 玩家昵称 |
| `CreatedAt` | `created_at` | 创建时间 |
| `UpdatedAt` | `updated_at` | 更新时间 |

```go
PasswordHash string `json:"-"`
```

这里的 `json:"-"` 很重要。

它表示：即使后端把 `Player` 转成 JSON 返回给客户端，也不要把密码哈希放进响应。

密码哈希虽然不是明文密码，但仍然属于敏感信息。

### 第 3 步：填写 `auth.go`

文件位置：

```text
E:\game-realtime-gm\backend\internal\handler\auth.go
```

写入：

```go
package handler

import (
	"errors"
	"net/http"

	"game-realtime-gm/backend/internal/model"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

type AuthHandler struct {
	db *pgxpool.Pool
}

func NewAuthHandler(db *pgxpool.Pool) *AuthHandler {
	return &AuthHandler{db: db}
}

type registerRequest struct {
	Username string `json:"username" binding:"required,min=3,max=64"`
	Password string `json:"password" binding:"required,min=6,max=72"`
	Nickname string `json:"nickname" binding:"required,max=64"`
}

func (h *AuthHandler) Register(c *gin.Context) {
	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40001,
			"message": "invalid request",
		})
		return
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50001,
			"message": "generate password hash failed",
		})
		return
	}

	var player model.Player
	err = h.db.QueryRow(
		c.Request.Context(),
		`INSERT INTO players (username, password_hash, nickname)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (username) DO NOTHING
		 RETURNING id, username, nickname, created_at, updated_at`,
		req.Username,
		string(passwordHash),
		req.Nickname,
	).Scan(
		&player.ID,
		&player.Username,
		&player.Nickname,
		&player.CreatedAt,
		&player.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		c.JSON(http.StatusConflict, gin.H{
			"code":    40901,
			"message": "username already exists",
		})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50002,
			"message": "create player failed",
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"code":    0,
		"message": "register success",
		"data":    player,
	})
}
```

#### 代码逐段分析

```go
type AuthHandler struct {
	db *pgxpool.Pool
}
```

`AuthHandler` 是账号接口处理器。

它内部保存了一个数据库连接池 `db`。注册接口需要向数据库写入玩家，所以必须能拿到数据库连接池。

```go
func NewAuthHandler(db *pgxpool.Pool) *AuthHandler {
	return &AuthHandler{db: db}
}
```

这是创建 `AuthHandler` 的函数。

路由初始化时会调用它，把 `main.go` 创建好的数据库连接池传进来。

```go
type registerRequest struct {
	Username string `json:"username" binding:"required,min=3,max=64"`
	Password string `json:"password" binding:"required,min=6,max=72"`
	Nickname string `json:"nickname" binding:"required,max=64"`
}
```

这个结构体描述注册请求 JSON。

- `json:"username"`：JSON 里的 `username` 会放进 Go 的 `Username` 字段。
- `required`：字段不能为空。
- `min=3,max=64`：账号长度限制。
- `min=6,max=72`：密码长度限制。bcrypt 最多处理 72 字节密码。
- `max=64`：昵称最长 64 个字符。

```go
if err := c.ShouldBindJSON(&req); err != nil {
```

Gin 会读取请求 JSON，把字段放入 `req`，并按照 `binding` 规则校验。

如果 JSON 格式不正确或缺少必填字段，就返回 HTTP `400 Bad Request`。

```go
passwordHash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
```

把用户输入的密码转换成 bcrypt 哈希。

- `[]byte(req.Password)`：把字符串密码转换成字节数组。
- `bcrypt.DefaultCost`：使用 bcrypt 默认计算强度。
- `passwordHash`：最终保存到数据库的哈希，不是原始密码。

```sql
INSERT INTO players (username, password_hash, nickname)
VALUES ($1, $2, $3)
```

向 `players` 表插入数据。

- `$1` 对应 `req.Username`。
- `$2` 对应 `string(passwordHash)`。
- `$3` 对应 `req.Nickname`。

使用 `$1`、`$2`、`$3` 传参，而不是直接拼接 SQL，可以防止 SQL 注入。

```sql
ON CONFLICT (username) DO NOTHING
```

如果用户名已经存在，就不要重复插入。

数据库表里的 `username` 有 `UNIQUE` 唯一约束，所以数据库会帮助我们阻止重复账号。

```sql
RETURNING id, username, nickname, created_at, updated_at
```

插入成功后，让 PostgreSQL 立即返回新玩家信息。

注意：这里没有返回 `password_hash`，因为接口响应不应该泄露密码哈希。

```go
if errors.Is(err, pgx.ErrNoRows) {
```

如果用户名重复，`ON CONFLICT DO NOTHING` 不会插入数据，也不会返回玩家记录。

这时 pgx 会返回 `pgx.ErrNoRows`，后端把它转换成友好的 JSON：

```json
{
  "code": 40901,
  "message": "username already exists"
}
```

```go
c.JSON(http.StatusCreated, gin.H{
```

注册成功后返回 HTTP `201 Created`。

`data` 里包含新玩家信息，但不包含密码哈希。

### 第 4 步：修改 `router.go`

文件位置：

```text
E:\game-realtime-gm\backend\internal\router\router.go
```

替换为：

```go
package router

import (
	"net/http"

	"game-realtime-gm/backend/internal/handler"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func New(db *pgxpool.Pool) http.Handler {
	r := gin.Default()

	authHandler := handler.NewAuthHandler(db)

	r.GET("/health", handler.Health)

	api := r.Group("/api")
	api.POST("/register", authHandler.Register)

	return r
}
```

#### 代码逐段分析

```go
func New(db *pgxpool.Pool) http.Handler {
```

原来是：

```go
func New() http.Handler
```

现在增加了 `db *pgxpool.Pool` 参数，因为路由初始化账号接口时，需要把数据库连接池交给 `AuthHandler`。

```go
authHandler := handler.NewAuthHandler(db)
```

创建账号接口处理器，并把数据库连接池传进去。

```go
api := r.Group("/api")
api.POST("/register", authHandler.Register)
```

注册接口路径。

两行组合起来就是：

```text
POST /api/register
```

收到这个请求后，Gin 会执行：

```go
authHandler.Register
```

### 第 5 步：修改 `main.go`

文件位置：

```text
E:\game-realtime-gm\backend\cmd\server\main.go
```

找到：

```go
r := router.New()
```

改成：

```go
r := router.New(dbPool)
```

#### 代码分析

`main.go` 已经创建了：

```go
dbPool, err := database.NewPostgresPool(ctx, cfg.Database)
```

现在把 `dbPool` 传给路由。

调用关系变成：

```text
main.go 创建 dbPool
        ↓
router.New(dbPool)
        ↓
handler.NewAuthHandler(dbPool)
        ↓
Register 使用 dbPool 写入 players 表
```

### 第 6 步：整理格式和依赖

在后端目录执行：

```powershell
cd E:\game-realtime-gm\backend
gofmt -w .\internal\model\player.go .\internal\handler\auth.go .\internal\router\router.go .\cmd\server\main.go
go mod tidy
go test ./...
```

命令分析：

- `gofmt -w`：自动整理 Go 代码格式，并写回文件。
- `go mod tidy`：根据当前代码真正用到的包，整理 `go.mod` 和 `go.sum`。
- `go test ./...`：编译并检查当前模块下所有包。即使还没有测试文件，也能发现语法错误和依赖问题。

如果你之前看到：

```text
missing go.sum entry
```

`go mod tidy` 通常会补齐依赖校验记录。

### 第 7 步：启动并测试注册接口

启动后端：

```powershell
cd E:\game-realtime-gm\backend
go run .\cmd\server
```

成功日志：

```text
database connected
server listening on :8080
```

#### 用 Apifox 测试

创建请求：

```text
POST http://localhost:8080/api/register
```

Body 类型选择：

```text
JSON
```

填写：

```json
{
  "username": "test001",
  "password": "123456",
  "nickname": "测试玩家"
}
```

第一次注册期望返回 HTTP `201`：

```json
{
  "code": 0,
  "message": "register success",
  "data": {
    "id": 1,
    "username": "test001",
    "nickname": "测试玩家",
    "created_at": "...",
    "updated_at": "..."
  }
}
```

注意：响应里不会出现 `password_hash`。

再次发送同样请求，期望返回 HTTP `409`：

```json
{
  "code": 40901,
  "message": "username already exists"
}
```

#### 用 PowerShell 测试

也可以在另一个 PowerShell 窗口执行：

```powershell
$body = @{
    username = "test001"
    password = "123456"
    nickname = "测试玩家"
} | ConvertTo-Json

Invoke-RestMethod `
    -Method Post `
    -Uri "http://localhost:8080/api/register" `
    -ContentType "application/json" `
    -Body $body
```

这里用 `Invoke-RestMethod`，因为它更适合 PowerShell。Windows PowerShell 里的 `curl` 有时是别名，行为和常见教程里的 curl 不完全一样。

### 第 8 步：去 PostgreSQL 检查数据

在 Docker Desktop 的 PostgreSQL 容器终端执行：

```bash
psql -U game -d game_realtime
```

进入 `psql` 后执行：

```sql
SELECT id, username, password_hash, nickname, created_at
FROM players;
```

你应该看到类似：

```text
id | username | password_hash                         | nickname
---+----------+---------------------------------------+----------
1  | test001  | $2a$10$...                            | 测试玩家
```

`password_hash` 应该是一串较长的内容，而不是：

```text
123456
```

这说明后端没有保存明文密码。

### 接口字段分析

- `username`：登录账号，必须唯一。
- `password`：用户原始密码，只能用于本次生成哈希，不能直接存数据库。
- `nickname`：玩家展示昵称。
- `code`：项目自定义业务状态码，`0` 表示成功。
- `message`：给调用方看的简单结果说明。
- `data`：注册成功后返回的新玩家公开信息。

### 注册逻辑总结

注册接口现在按这个顺序工作：

```text
读取并校验请求 JSON
        ↓
用 bcrypt 生成 password_hash
        ↓
向 players 表插入数据
        ↓
如果 username 重复，返回 409
        ↓
如果成功，返回玩家公开信息
```

为什么使用数据库唯一约束处理重复账号：

即使两个注册请求几乎同时到达，数据库也能保证最终只会有一个相同用户名。业务代码再把冲突转换成容易理解的 JSON。

为什么要用 bcrypt：

bcrypt 是专门用于密码哈希的算法，比普通 MD5/SHA 更适合保存密码。

### 和项目其他部分的关系

注册接口需要：

- 使用 `players` 表。
- 使用数据库连接池。
- 使用路由注册。
- 后续和登录接口共享玩家查询逻辑。

### 怎么验证成功

按照第 7 步发送请求，再按照第 8 步检查数据库。

### 常见错误

错误 1：

```text
expected 'package', found 'EOF'
```

原因：你创建了 `auth.go` 或 `player.go`，但文件还是空的。

解决：先按照上面的步骤把代码写进文件，再执行 `go test ./...`。

错误 2：

```text
missing go.sum entry
```

原因：安装依赖后，依赖校验记录还没有整理完整。

解决：

```powershell
cd E:\game-realtime-gm\backend
go mod tidy
```

错误 3：

```text
cannot use router.New() without arguments
```

原因：`router.New` 已经改成需要数据库连接池，但 `main.go` 还是旧写法。

解决：把：

```go
r := router.New()
```

改成：

```go
r := router.New(dbPool)
```

错误 4：

```text
404 page not found
```

原因：路由没有注册，或者请求地址写错。

检查：

```go
api.POST("/register", authHandler.Register)
```

请求地址必须是：

```text
POST http://localhost:8080/api/register
```

错误 5：

```text
invalid request
```

原因：请求 JSON 缺少字段、格式错误，或者账号密码长度不符合要求。

检查 Apifox Body 类型是否为 JSON，并确认三个字段都填写。

## 任务 6：实现登录接口和 JWT

### 任务目标

实现 `POST /api/login`，让用户用账号密码登录，并返回 JWT token。

完成后，调用方可以先注册：

```text
POST /api/register
```

再登录：

```text
POST /api/login
```

登录成功后，后端会返回一段 token。后续 WebSocket、玩家信息、房间接口都会靠这个 token 判断“当前请求是谁发来的”。

### 你要新增或修改什么

新增：

```text
E:\game-realtime-gm\backend\internal\auth\jwt.go
```

修改：

```text
E:\game-realtime-gm\backend\internal\handler\auth.go
E:\game-realtime-gm\backend\internal\router\router.go
E:\game-realtime-gm\backend\cmd\server\main.go
```

安装依赖：

```text
github.com/golang-jwt/jwt/v5
```

### 这个操作有什么用

HTTP 是无状态的。

意思是：用户第一次请求登录成功后，下一次请求后端并不会天然知道“这个请求是谁发来的”。

JWT 的作用是：登录成功后，后端给客户端一个 token。客户端后续访问接口时带上 token，后端通过 token 判断用户身份。

你可以把 JWT 理解成后端签发的一张临时通行证。

### 第 1 步：安装 JWT 依赖

在后端目录执行：

```powershell
cd E:\game-realtime-gm\backend
go get github.com/golang-jwt/jwt/v5
```

命令分析：

- `cd E:\game-realtime-gm\backend`：进入 Go 模块目录，因为 `go.mod` 在这里。
- `go get`：下载依赖，并记录到 `go.mod` 和 `go.sum`。
- `github.com/golang-jwt/jwt/v5`：Go 里常用的 JWT 库。

### 第 2 步：新建 `internal/auth/jwt.go`

文件位置：

```text
E:\game-realtime-gm\backend\internal\auth\jwt.go
```

操作类型：新建文件。如果 `internal/auth` 目录不存在，先创建目录：

```powershell
cd E:\game-realtime-gm\backend
mkdir internal\auth
```

然后在 `jwt.go` 里写入完整代码：

```go
package auth

import (
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	PlayerID int64  `json:"player_id"`
	Username string `json:"username"`
	jwt.RegisteredClaims
}

func GenerateToken(secret string, playerID int64, username string) (string, error) {
	claims := Claims{
		PlayerID: playerID,
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatInt(playerID, 10),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}
```

#### 代码逐段分析

```go
package auth
```

表示这个文件属于 `auth` 包。这里的 `auth` 专门放认证、鉴权相关工具代码。

注意：`internal/auth` 和 `internal/handler/auth.go` 不是一回事。

- `internal/auth`：放 JWT 这类认证工具。
- `internal/handler/auth.go`：放注册、登录这类 HTTP 接口处理函数。

```go
type Claims struct {
	PlayerID int64  `json:"player_id"`
	Username string `json:"username"`
	jwt.RegisteredClaims
}
```

`Claims` 表示 JWT 里保存的信息。

- `PlayerID`：玩家 ID。
- `Username`：账号名。
- `jwt.RegisteredClaims`：JWT 标准字段，例如签发时间、过期时间、主题。

```go
func GenerateToken(secret string, playerID int64, username string) (string, error)
```

这个函数负责生成 token。

- `secret`：JWT 密钥，来自 `config.go`。
- `playerID`：登录成功的玩家 ID。
- `username`：登录成功的账号名。
- 返回值 `string`：生成好的 token。
- 返回值 `error`：生成失败时的错误。

```go
ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
```

表示 token 24 小时后过期。

### 第 3 步：整文件替换 `handler/auth.go`

文件位置：

```text
E:\game-realtime-gm\backend\internal\handler\auth.go
```

操作类型：整文件替换。

把 `auth.go` 全部替换成下面内容：

```go
package handler

import (
	"errors"
	"net/http"

	tokenauth "game-realtime-gm/backend/internal/auth"
	"game-realtime-gm/backend/internal/model"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

type AuthHandler struct {
	db        *pgxpool.Pool
	jwtSecret string
}

func NewAuthHandler(db *pgxpool.Pool, jwtSecret string) *AuthHandler {
	return &AuthHandler{
		db:        db,
		jwtSecret: jwtSecret,
	}
}

type registerRequest struct {
	Username string `json:"username" binding:"required,min=3,max=64"`
	Password string `json:"password" binding:"required,min=6,max=72"`
	Nickname string `json:"nickname" binding:"required,max=64"`
}

type loginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

func (h *AuthHandler) Register(c *gin.Context) {
	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40001,
			"message": "invalid request",
		})
		return
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50001,
			"message": "generate password hash failed",
		})
		return
	}

	var player model.Player
	err = h.db.QueryRow(
		c.Request.Context(),
		`INSERT INTO players (username, password_hash, nickname)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (username) DO NOTHING
		 RETURNING id, username, nickname, created_at, updated_at`,
		req.Username,
		string(passwordHash),
		req.Nickname,
	).Scan(
		&player.ID,
		&player.Username,
		&player.Nickname,
		&player.CreatedAt,
		&player.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		c.JSON(http.StatusConflict, gin.H{
			"code":    40901,
			"message": "username already exists",
		})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50002,
			"message": "create player failed",
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"code":    0,
		"message": "register success",
		"data":    player,
	})
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40001,
			"message": "invalid request",
		})
		return
	}

	var player model.Player
	err := h.db.QueryRow(
		c.Request.Context(),
		`SELECT id, username, password_hash, nickname, created_at, updated_at
		 FROM players
		 WHERE username = $1`,
		req.Username,
	).Scan(
		&player.ID,
		&player.Username,
		&player.PasswordHash,
		&player.Nickname,
		&player.CreatedAt,
		&player.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    40101,
			"message": "username or password is wrong",
		})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50003,
			"message": "query player failed",
		})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(player.PasswordHash), []byte(req.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    40101,
			"message": "username or password is wrong",
		})
		return
	}

	token, err := tokenauth.GenerateToken(h.jwtSecret, player.ID, player.Username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50004,
			"message": "generate token failed",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "login success",
		"data": gin.H{
			"token": token,
			"player": gin.H{
				"id":       player.ID,
				"username": player.Username,
				"nickname": player.Nickname,
			},
		},
	})
}
```

#### 代码逐段分析

```go
tokenauth "game-realtime-gm/backend/internal/auth"
```

这里给 `internal/auth` 包起了一个别名 `tokenauth`。

原因是当前文件本身也叫 `auth.go`，而且业务上也叫 auth。用 `tokenauth` 可以让代码更清楚：这是 JWT token 工具包。

```go
type AuthHandler struct {
	db        *pgxpool.Pool
	jwtSecret string
}
```

`AuthHandler` 现在保存两样东西：

- `db`：注册和登录都要访问数据库。
- `jwtSecret`：登录成功后生成 JWT 需要密钥。

```go
func NewAuthHandler(db *pgxpool.Pool, jwtSecret string) *AuthHandler
```

创建账号接口处理器。

和任务 5 相比，这里多传了 `jwtSecret`。

```go
type loginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}
```

描述登录请求 JSON。

登录时只需要账号和密码，不需要昵称。

```sql
SELECT id, username, password_hash, nickname, created_at, updated_at
FROM players
WHERE username = $1
```

根据用户名查找玩家。

登录必须取出 `password_hash`，因为要用它校验用户输入的密码。

```go
if errors.Is(err, pgx.ErrNoRows)
```

如果数据库找不到这个用户名，返回登录失败。

注意这里不返回“用户名不存在”，而是统一返回：

```text
username or password is wrong
```

这样更安全，避免别人通过接口探测哪些账号存在。

```go
bcrypt.CompareHashAndPassword([]byte(player.PasswordHash), []byte(req.Password))
```

用 bcrypt 校验密码。

它不是把两个字符串直接比较，而是用 bcrypt 的规则判断“输入密码”和“数据库里的哈希”是否匹配。

```go
token, err := tokenauth.GenerateToken(h.jwtSecret, player.ID, player.Username)
```

密码正确后生成 JWT。

```go
"player": gin.H{
	"id":       player.ID,
	"username": player.Username,
	"nickname": player.Nickname,
}
```

登录成功时顺便返回玩家公开信息。

这里不返回 `password_hash`。

### 第 4 步：整文件替换 `router.go`

文件位置：

```text
E:\game-realtime-gm\backend\internal\router\router.go
```

操作类型：整文件替换。

把 `router.go` 全部替换成下面内容：

```go
package router

import (
	"net/http"

	"game-realtime-gm/backend/internal/config"
	"game-realtime-gm/backend/internal/handler"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func New(db *pgxpool.Pool, cfg config.Config) http.Handler {
	r := gin.Default()

	authHandler := handler.NewAuthHandler(db, cfg.JWTSecret)

	r.GET("/health", handler.Health)

	api := r.Group("/api")
	api.POST("/register", authHandler.Register)
	api.POST("/login", authHandler.Login)

	return r
}
```

#### 代码逐段分析

```go
"game-realtime-gm/backend/internal/config"
```

路由层需要读取 `cfg.JWTSecret`，所以要引入配置包。

```go
func New(db *pgxpool.Pool, cfg config.Config) http.Handler
```

和任务 5 相比，`router.New` 多了一个 `cfg config.Config` 参数。

这是因为账号处理器现在不只需要数据库，还需要 JWT 密钥。

```go
authHandler := handler.NewAuthHandler(db, cfg.JWTSecret)
```

创建账号处理器，并传入：

- 数据库连接池
- JWT 密钥

```go
api.POST("/login", authHandler.Login)
```

新增登录路由。

完整接口路径是：

```text
POST /api/login
```

### 第 5 步：修改 `main.go`

文件位置：

```text
E:\game-realtime-gm\backend\cmd\server\main.go
```

操作类型：只修改一行。

修改前：

```go
r := router.New(dbPool)
```

修改后：

```go
r := router.New(dbPool, cfg)
```

#### 代码分析

`main.go` 里已经有：

```go
cfg := config.Load()
```

`cfg` 里面包含：

```text
AppPort
Database
JWTSecret
```

登录接口需要 `JWTSecret`，所以 `main.go` 要把完整配置传给路由。

调用关系变成：

```text
main.go 读取 cfg
        ↓
router.New(dbPool, cfg)
        ↓
handler.NewAuthHandler(db, cfg.JWTSecret)
        ↓
Login 调用 GenerateToken
```

### 第 6 步：整理格式和依赖

在后端目录执行：

```powershell
cd E:\game-realtime-gm\backend
gofmt -w .\internal\auth\jwt.go .\internal\handler\auth.go .\internal\router\router.go .\cmd\server\main.go
go mod tidy
go test ./...
```

命令分析：

- `gofmt -w`：格式化 Go 代码。
- `go mod tidy`：整理 `go.mod` 和 `go.sum`。
- `go test ./...`：编译检查所有包。

成功时应该看到类似：

```text
?    game-realtime-gm/backend/cmd/server        [no test files]
?    game-realtime-gm/backend/internal/auth     [no test files]
?    game-realtime-gm/backend/internal/handler  [no test files]
```

`[no test files]` 不是错误，它表示当前包还没有测试文件，但编译通过。

### 第 7 步：启动后端

执行：

```powershell
cd E:\game-realtime-gm\backend
go run .\cmd\server
```

成功日志应该包含：

```text
database connected
server listening on :8080
```

### 第 8 步：用 Apifox 测试登录

先确认已经注册过账号：

```json
{
  "username": "test001",
  "password": "123456",
  "nickname": "测试玩家"
}
```

然后创建登录请求：

```text
POST http://localhost:8080/api/login
```

Body 类型选择 JSON：

```json
{
  "username": "test001",
  "password": "123456"
}
```

成功时返回 HTTP `200`，类似：

```json
{
  "code": 0,
  "message": "login success",
  "data": {
    "token": "一长串 JWT",
    "player": {
      "id": 1,
      "username": "test001",
      "nickname": "测试玩家"
    }
  }
}
```

密码错误时返回 HTTP `401`：

```json
{
  "code": 40101,
  "message": "username or password is wrong"
}
```

### 第 9 步：用 PowerShell 测试登录

如果不用 Apifox，也可以开另一个 PowerShell：

```powershell
$body = @{
    username = "test001"
    password = "123456"
} | ConvertTo-Json

Invoke-RestMethod `
    -Method Post `
    -Uri "http://localhost:8080/api/login" `
    -ContentType "application/json" `
    -Body $body
```

如果成功，PowerShell 会显示 `code`、`message`、`data`。

### 登录逻辑总结

登录接口现在按这个顺序工作：

```text
读取并校验请求 JSON
        ↓
根据 username 查询 players 表
        ↓
如果找不到用户，返回 401
        ↓
用 bcrypt 校验密码
        ↓
如果密码错误，返回 401
        ↓
生成 JWT
        ↓
返回 token 和玩家公开信息
```

### 和项目其他部分的关系

后续这些功能都需要登录身份：

- 查看玩家自己的信息。
- 建立 WebSocket 连接。
- 加入房间。
- 进入匹配队列。
- GM 后台鉴权。

所以登录和 JWT 是后面权限系统的地基。

### 常见错误

错误 1：

```text
crypto/bcrypt: hashedPassword is not the hash of the given password
```

原因：密码错误。

解决：返回“用户名或密码错误”即可，不要告诉用户到底是用户名错还是密码错。

错误 2：

```text
JWT_SECRET 为空
```

原因：配置没有读取到 JWT 密钥。

解决：检查 `config.Load()` 里是否有默认值，或者环境变量是否设置正确。

错误 3：

```text
not enough arguments in call to router.New
```

原因：`router.New` 改成需要两个参数，但 `main.go` 还没改。

解决：把：

```go
r := router.New(dbPool)
```

改成：

```go
r := router.New(dbPool, cfg)
```

错误 4：

```text
undefined: handler.NewAuthHandler
```

原因：`handler/auth.go` 没有按完整代码替换，或者函数签名写错。

解决：确认函数是：

```go
func NewAuthHandler(db *pgxpool.Pool, jwtSecret string) *AuthHandler
```

错误 5：

```text
404 page not found
```

原因：没有在 `router.go` 里注册登录路由，或者请求方法/路径写错。

正确路径：

```text
POST http://localhost:8080/api/login
```

错误 6：

```text
401 username or password is wrong
```

原因可能是：

- 还没有注册这个账号。
- 密码输入错了。
- 数据库里的测试账号是手动插入的，`password_hash` 不是 bcrypt 生成的。

解决：先用 `/api/register` 注册一个新账号，再用同样的账号密码登录。

## 今日验收清单

- [ ] Docker Desktop 已启动。
- [ ] `game_realtime_postgres` 容器正在运行。
- [ ] `game_realtime_redis` 容器正在运行。
- [ ] `go run .\cmd\server` 可以启动后端。
- [ ] `/health` 仍然返回 `code=0`。
- [ ] 后端可以通过环境变量切换端口。
- [ ] 后端可以连接 PostgreSQL。
- [ ] 数据库中已创建 `players` 表。
- [ ] `players` 表字段能用 `\d players` 查看。
- [ ] 如果继续实现接口，`/api/register` 可以注册账号。
- [ ] 如果继续实现接口，`/api/login` 可以返回 JWT。

## 今日不要做

今天不要做：

- WebSocket 长连接。
- 心跳机制。
- 房间系统。
- 匹配队列。
- 排行榜。
- React GM 后台页面。
- 管理员 RBAC 权限。
- 微服务拆分。
- Kubernetes。

这些功能都依赖账号系统和数据库地基。今天先把地基打稳。

## 常见问题总览

### 1. Docker 命令报错

优先检查 Docker Desktop 是否启动。

执行：

```powershell
docker version
docker compose version
```

如果 Docker Engine 没启动，先打开 Docker Desktop。

### 2. PostgreSQL 端口冲突

如果本机已有 PostgreSQL 占用 `5432`，把 Docker Compose 改成：

```yaml
ports:
  - "15432:5432"
```

然后运行后端前设置：

```powershell
$env:DB_PORT="15432"
```

### 3. 后端启动后无法访问 `/health`

检查：

- 后端是否还在运行。
- 端口是不是 `8080`。
- 是否设置过 `APP_PORT=8081`。
- 浏览器访问地址是否正确。

### 4. 数据库连接失败

检查四件事：

- PostgreSQL 容器是否运行。
- `DB_HOST` 是否是 `localhost`。
- `DB_PORT` 是否和 Docker Compose 映射端口一致。
- `DB_USER`、`DB_PASSWORD`、`DB_NAME` 是否和 Docker Compose 一致。

## 今天学完后你应该能说清楚

今天结束时，你不只是“敲完代码”，而是应该能解释：

- Docker Compose 为什么适合启动数据库和 Redis。
- PostgreSQL 和 Redis 在这个项目里的分工。
- 为什么项目要有 `internal/config`。
- 为什么不能把数据库密码写死在业务代码里。
- Go 后端连接数据库为什么要用连接池。
- `players` 表为什么需要主键、唯一用户名和密码哈希。
- 注册和登录分别在后端做了什么。
- JWT 在后续接口鉴权里会起什么作用。

## 面试怎么讲这一部分

可以这样说：

```text
项目初期我先搭建了 Go + Gin 的后端骨架，并通过 Docker Compose 提供 PostgreSQL 和 Redis 依赖服务。随后抽离配置模块，使用环境变量管理服务端口、数据库连接信息和 JWT 密钥。数据库层使用 pgxpool 创建 PostgreSQL 连接池，并通过健康检查确认连接可用。账号模块设计了 players 表，使用唯一 username 和 password_hash 保存注册信息，为后续登录鉴权、WebSocket 玩家身份识别、房间系统和 GM 后台查询打基础。
```
