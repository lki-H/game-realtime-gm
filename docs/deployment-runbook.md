# 本地部署、运维与排障手册

> 文档角色：环境准备、启动停止、诊断、发布演练与 Troubleshooting
> 权威级别：L1（当前运行手册）
> 状态：适用于本地/测试环境
> 适用范围：Windows + Docker Desktop + 宿主机 Go 服务
> 事实来源：当前配置加载、Docker Compose、schema、seed 与 pprof 实现
> 最后更新：2026-10-11

## R3 本机新增运行项

- V2本机服务可开启 `PVE_ARCHIVE_ENABLED=true`、`PVE_ARCHIVE_RETENTION_DAYS`、`PVE_ARCHIVE_BATCH_SIZE`；归档只处理已终态、已结算、无活动占用和无待处理操作的Run，默认关闭。
- 指标服务通过 `PVE_METRICS_ENABLED=true`、loopback `PVE_METRICS_ADDR` 和独立 `PVE_METRICS_TOKEN` 开启；该token不等于测试事件token，Prometheus只通过Bearer读取。不要绑定公网地址。
- `WS_MAX_CONNECTIONS` 和 `WS_MAX_CONNECTIONS_PER_IP` 限制连接资源；替换同一玩家连接仍然允许，超额连接返回42972。
- 本机监控使用 `deploy/docker-compose.monitoring.yml`、`deploy/monitoring/prometheus.example.yml` 和Grafana provisioning文件；Prometheus/Grafana只绑定127.0.0.1，发布前必须替换临时密码和token。
- 四个Unity Player本机验收使用 `deploy/verify-r3-unity.ps1`，它要求隔离V2配置、临时密码和独立事件token；脚本不会向项目文件写密码。
- Dockerfile和CI锁定Go 1.27.1；本机Go/React验证和`game-realtime-gm:r3-local`镜像构建通过。Docker Desktop使用Clash Verge的Allow LAN及手动代理访问Docker Hub；共享环境应使用受控代理，不要把本机代理地址写进仓库。

## R3 本机新增运行项

- V2本机服务可开启 `PVE_ARCHIVE_ENABLED=true`、`PVE_ARCHIVE_RETENTION_DAYS`、`PVE_ARCHIVE_BATCH_SIZE`；归档只处理已终态、已结算、无活动占用和无待处理操作的Run，默认关闭。
- 指标服务通过 `PVE_METRICS_ENABLED=true`、loopback `PVE_METRICS_ADDR` 和独立 `PVE_METRICS_TOKEN` 开启；该token不等于测试事件token，Prometheus只通过Bearer读取。不要绑定公网地址。
- `WS_MAX_CONNECTIONS` 和 `WS_MAX_CONNECTIONS_PER_IP` 限制连接资源；替换同一玩家连接仍然允许，超额连接返回42972。
- 本机监控使用 `deploy/docker-compose.monitoring.yml`、`deploy/monitoring/prometheus.example.yml` 和Grafana provisioning文件；Prometheus/Grafana只绑定127.0.0.1，发布前必须替换临时密码和token。
- 四个Unity Player本机验收使用 `deploy/verify-r3-unity.ps1`，它要求隔离V2配置、临时密码和独立事件token；脚本不会向项目文件写密码。

## 1. 运行模型

当前只有 MySQL 和 Redis 运行在 Docker Compose 中；Go 服务在宿主机通过 `go run` 或本地二进制启动。没有 Go Dockerfile、反向代理、TLS、Kubernetes、Helm、Ingress、HPA 或自动部署。

依赖顺序：

```text
Docker Desktop
-> MySQL 与 Redis 容器
-> schema/seed
-> Go 服务
-> HTTP/WebSocket 验证
```

## 2. 前置条件

- Go 版本满足 `backend/go.mod`。
- Docker Desktop 与 Docker Engine 已启动。
- 本机 `3306`、`6379` 和应用端口未被其他进程占用。
- 在共享环境运行前，已覆盖所有本地开发密码和 `JWT_SECRET`。

检查：

```powershell
go version
docker version
docker compose version
Get-NetTCPConnection -State Listen -ErrorAction SilentlyContinue |
  Where-Object LocalPort -in 3306,6379,8080,6060
```

## 3. 环境变量

| 变量 | 当前默认/用途 | 共享环境要求 |
| --- | --- | --- |
| `APP_PORT` | `8080`，HTTP 与 WebSocket | 选择明确端口 |
| `APP_HOST` | 空值，沿用所有网卡 | 本机验收设 `127.0.0.1`；容器内按端口映射需要选择 |
| `DB_MAX_OPEN_CONNS/DB_MAX_IDLE_CONNS` | `20/10` | 玩法服务open至少2（一条所有权、一条业务）；idle允许0，不能超过open |
| `REDIS_POOL_SIZE/REDIS_MAX_ACTIVE_CONNS` | `10/20` | base池与硬连接上限；不能只设PoolSize而允许额外连接无限扩张 |
| `REDIS_POOL_TIMEOUT_MS` | `1000` | 池耗尽等待预算；必须为正，故障请求不无限等空闲连接 |
| `DB_HOST` / `DB_PORT` | `localhost:3306` | 指向受控 MySQL |
| `DB_USER` / `DB_PASSWORD` / `DB_NAME` | 本地开发连接 | 必须通过 Secret 覆盖 |
| `REDIS_ADDR` / `REDIS_PASSWORD` / `REDIS_DB` | `localhost:6379` / 空 / 0 | 按环境隔离并鉴权 |
| `JWT_SECRET` | 代码含开发回退值 | 任何共享/公网环境必须使用强随机 Secret |
| `PPROF_ENABLED` | `false` | 默认保持关闭 |
| `PPROF_ADDR` | `127.0.0.1:6060` | 不得绑定公网地址 |

不要打印完整环境、DSN、Authorization 头或 token。`docker compose config` 可能展开环境值，不要将输出粘贴到公开日志。

## 4. 启动依赖

```powershell
cd .\deploy
docker compose config
docker compose up -d
docker compose ps
```

预期容器：

```text
game_realtime_mysql
game_realtime_redis
```

查看日志：

```powershell
docker compose logs --tail 100 mysql
docker compose logs --tail 100 redis
```

MySQL 首次初始化可能比 Redis 慢；出现连接失败时先等待 MySQL ready，再启动 Go 服务。

## 5. 初始化数据库

在仓库根目录执行全量 schema：

```powershell
Get-Content .\backend\internal\database\schema.sql -Raw |
  docker exec -i game_realtime_mysql sh -lc 'mysql -u"$MYSQL_USER" -p"$MYSQL_PASSWORD" "$MYSQL_DATABASE"'
```

本地管理员 seed：

```powershell
Get-Content .\backend\internal\database\seed.sql -Raw |
  docker exec -i game_realtime_mysql sh -lc 'mysql -u"$MYSQL_USER" -p"$MYSQL_PASSWORD" "$MYSQL_DATABASE"'
```

这些命令可能显示 MySQL 的命令行密码警告；凭据来自容器开发环境，仍不要复制完整命令输出到公开材料。任何共享环境都必须先替换 seed 管理员凭据。

验证表：

```powershell
docker exec game_realtime_mysql sh -lc `
  'mysql -u"$MYSQL_USER" -p"$MYSQL_PASSWORD" "$MYSQL_DATABASE" -e "SHOW TABLES;"'
```

应看到 7 张表：`players`、`admins`、`admin_operation_logs`、`mission_records`、`reward_records`、`asset_ledger`、`player_assets`。

`migrations/day31_settlement_assets.sql` 只用于明确处于 Day30 结构的旧本地库；新环境只执行当前 `schema.sql`，不要同时盲目重复执行增量 migration。

## 6. 启动与检查 Go 服务

```powershell
cd .\backend
go run .\cmd\server
```

正常日志应包含数据库、Redis 连接成功和监听端口，不应输出密码、JWT 或 WebSocket raw query。

另开终端检查：

```powershell
Invoke-RestMethod http://localhost:8080/health
```

`/health` 只检查 HTTP 活性；`GET /ready` 在共同一秒预算内检查 MySQL/Redis，且进程生命周期有效才返回 200，否则返回 503/code=50301。探针不返回密码、连接地址或原始依赖错误，不代表新匹配准入已开启；维护/排空使用原有控制面状态。服务启动先完成事实恢复再监听，因此启动期间不会提前就绪。

健康检查只证明 HTTP 进程响应。再执行登录、在线或 GM 摘要，才能覆盖 MySQL/Redis 业务链路。

## 7. pprof

只在本机诊断时临时启用：

```powershell
$env:PPROF_ENABLED = "true"
$env:PPROF_ADDR = "127.0.0.1:6060"
go run .\cmd\server
```

检查：

```powershell
Invoke-WebRequest http://127.0.0.1:6060/debug/pprof/
```

结束后清理当前 PowerShell 环境变量：

```powershell
Remove-Item Env:PPROF_ENABLED -ErrorAction SilentlyContinue
Remove-Item Env:PPROF_ADDR -ErrorAction SilentlyContinue
```

profile、goroutine dump 和运行日志保存在仓库外，不提交 Git。pprof 不得暴露公网。

## 8. 停止与重启

Go 服务：在运行终端按 `Ctrl+C`。服务执行信号驱动的优雅关闭，先停止接收新请求并关闭 Worker/WS；仍需等待终止日志，不能把进程强杀当作正常停止。

停止依赖但保留容器和数据：

```powershell
cd .\deploy
docker compose stop
```

重新启动：

```powershell
docker compose start
```

停止并删除容器/网络但保留命名 volume：

```powershell
docker compose down
```

不要在日常重启时使用 `down -v`，它会删除数据库和 Redis volume。删除前必须先备份并明确确认目标。

## 9. 常见故障

### 端口被占用

```powershell
Get-NetTCPConnection -State Listen -LocalPort 3306,6379,8080,6060 -ErrorAction SilentlyContinue |
  Select-Object LocalAddress,LocalPort,OwningProcess
```

再用 `Get-Process -Id <pid>` 识别进程。MySQL 本机服务与 Docker MySQL 不应同时占用 `3306`；可以停用不参与项目的本机服务，或明确改变映射和 `DB_PORT`。

### MySQL 未就绪或 schema 缺失

- `docker compose ps` 查看状态。
- `docker compose logs --tail 100 mysql` 查看初始化错误。
- 使用 `SHOW TABLES` 确认 7 张表。
- 若出现字段/索引冲突，先确认数据库当前版本，不要反复执行 migration。

### Redis 未启动

```powershell
docker exec game_realtime_redis redis-cli PING
```

预期 `PONG`。服务启动阶段 Redis ping 失败会直接退出。

### JWT 配置错误

症状：刚登录获得的 token 在另一个进程持续返回 invalid token。确认签发和校验服务使用相同 `JWT_SECRET`，且没有在终端间使用不同环境变量。不要在日志中打印 Secret 或 token。

### WebSocket 连接失败

- 确认使用玩家 token，不是管理员 token。
- URL 必须是 `ws://<host>:<port>/ws?token=...`。
- 检查 token 是否过期、应用端口是否一致。
- AccessLog 只应显示 `path=/ws`；公开截图前再次检查无 query/token。
- 非文本、超过 4096 字节或 70 秒未 pong 会导致错误/断开。

### 排行榜与 MySQL 不一致

MySQL 结算成功但 Redis 同步失败时，资产仍然有效。使用同一幂等结算请求重试可再次触发排行榜同步；当前没有自动重建命令，严禁手工修改资产表来“修榜”。

## 10. 本地/测试发布演练

当前发布流程仅是演练：

1. 记录待发布 commit、Go/Docker 版本和环境变量名称，不记录 Secret 值。
2. 执行 `gofmt -d`、`go test ./...`、`go vet ./...`；按风险执行 Linux race。
3. 备份 MySQL并记录校验值。
4. 在测试环境执行 schema/migration，再启动 Go 服务。
5. 运行健康、登录、WebSocket、结算、排行和 GM 冒烟。
6. 失败时停止新进程，恢复上一已知代码版本；数据库变更按已验证恢复方案处理，不能只回退代码。
7. 记录结果、问题、恢复耗时和后续动作。

这不构成正式 CI/CD、生产发布或线上值班经验。

## 11. 当前不具备

- 自动部署流水线及生产镜像仓库；已有本机构建镜像与GitHub Actions测试流水线。
- systemd/Windows Service 守护、Kubernetes probes、自动扩缩容。
- TLS、域名、反向代理、可信代理配置和公网安全基线。
- 共享环境Prometheus/Grafana、外部告警通知、OpenTelemetry和集中日志；已有本机指标采集与告警规则。
- 蓝绿/金丝雀、自动回滚、HA MySQL/Redis 或跨区域灾备。

备份与恢复步骤见 [备份与恢复](backup-and-recovery.md)，安全发布前置条件见 [安全设计](security-design.md)。

## 12. R1 V2 本地迁移与资源配置

V2启动前在明确目标库执行 `go run ./cmd/tools/v2_migrate -stage r4`（backend目录），需要 `DB_*` 与 `V2_MIGRATION_CONFIRM=I_UNDERSTAND_V2_MIGRATION`。工具与玩法服务共享数据库排他锁，运行中的服务会阻止迁移。已有dirty/错误checksum/不完整schema不得直接再跑SQL；先按备份恢复说明处理。day40增加持久化维护准入和幂等控制；默认模式为v2，旧模式只作为显式回归。

R2 day38 已在本机开发库完成备份恢复副本验证后执行，`day37_v2_pve_foundation` 与 `day38_v2_product_model` 均为 applied；R4 day40 已在恢复副本和本机开发库验证并应用。后续环境仍需独立备份恢复副本和停写窗口；执行 `go run ./cmd/tools/v2_migrate -stage r4` 会按顺序重复校验day37—40。R2/R4新表不修改legacy资产/历史，失败按迁移账本处理。

新增配置：`HTTP_MAX_BODY_BYTES=1048576`、`AUTH_RATE_LIMIT_PER_MINUTE=20`、`WS_RATE_LIMIT_PER_MINUTE=30`、`WS_COMMAND_RATE_LIMIT_PER_MINUTE=120`、`PVE_MAX_PROPOSAL_ROUNDS=8`，V2启动要求正值。Gin拒绝默认信任转发头；若未来加反向代理，必须显式配置受信代理。operator修复入口无需启用内部事件监听，普通gm只读；详见OpenAPI和 [维护与回退实施](design/v2-r4-implementation.md)。

## 13. R5 独立实验运行

先执行 `experiments/r5-m6/deploy/verify.ps1`，默认用仓库外 D 盘证据和固定 `gm-r5-verify` 隔离项目，结束清理实验进程、容器、卷和临时凭据。需要手动学习则加 `-KeepEnvironment`，再对输出目录运行 `deploy/start.ps1/stop.ps1 -EnvironmentDirectory <目录>`；`-ResetData` 只删除本实验数据。端口只绑定本机，原开发数据和主服务玩法模式不修改。

完整初学者步骤、具体端口、日志/指标、重建和 needs_repair 排障见 [独立实验说明](../experiments/r5-m6/README.md)。主服务不必配置 gRPC 或 RabbitMQ；停止整个实验即可回退。CI新增独立契约/单元检查，远程执行须以实际 run 为准，不将本机 PASS 称为远程 CI。

主Worker遇到无效通知会隔离到outbox的needs_repair，GM通知记录仅显示元数据，不返回私聊正文；排空ready-to-stop和needs_repair指标包含该行。修复须先核对来源/原操作并恢复有效payload，再由受控运维恢复pending；禁止为“清零指标”直接删除未知事实。当前没有公开写通知接口。R5主reports队列补DLX参数后，之前保留的实验环境需用stop -ResetData清理再重新验收初始化；不覆盖开发数据库。
