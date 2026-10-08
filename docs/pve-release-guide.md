# V2 PVE发布与运行指南

> 文档角色：R4发布范围、启动配置与维护/回滚入口
> 权威级别：L1
> 状态：R4本机实施已验证；公开发布证据见R4发布记录
> 最后更新：2026-10-08

## 发布范围

V2是模块化单体控制面。MySQL保存好友房间、来源票据、候选、Run、任务、奖励和操作结果；Redis保存队列/展示投影，连接对象留在内存。好友房间与本局参战队伍分离，每局最多4人，可选个人任务，共同必需目标完成后统一结束，再逐人结算。

本次发布后端及测试、双React客户端、Unity工程/Windows构建与验证脚本、本机监控配置、CI、编号迁移和契约。排除个人规则、Day/学习历史、内部交接、原始日志、备份、node_modules、Unity缓存和本机服务凭据。默认是 `GAMEPLAY_MODE=v2`；旧一期仅作为显式 `GAMEPLAY_MODE=legacy` 回归模式保留。

## 数据库准备

legacy schema初始化与V2增量迁移是两步。已有库先备份并恢复到新副本，对账后再迁移；MySQL DDL不保证整文件回滚。旧历史不转成Run，不重新写账本。

```powershell
cd backend
$env:V2_MIGRATION_CONFIRM='I_UNDERSTAND_V2_MIGRATION'
$env:GAMEPLAY_MODE='v2'
go run ./cmd/tools/v2_migrate -stage r4
```

执行前显式设置目标 `DB_HOST/DB_PORT/DB_USER/DB_PASSWORD/DB_NAME`，不要打印DSN或完整环境。工具顺序检查day37/day38/day39/day40账本、checksum和结构；重复运行返回applied/reconciled，dirty/部分DDL/校验不一致会拒绝。day40只新增维护准入和控制回执表，不删除事件/资产。SQL按LF字节固定，已应用文件不做格式整理。

本次本机开发库备份恢复后补齐day39/day40，旧2玩家/2资产/余额0及历史摘要不变。备份留在仓库外，源/恢复库均重复迁移通过；这不意味着其他环境已自动迁移。

## 服务和客户端

宿主机启动前设置MySQL/Redis配置、独立随机 `JWT_SECRET`，显式 `GAMEPLAY_MODE=v2`，在backend执行 `go run ./cmd/server`。单库 `GET_LOCK` 保证只有一个玩法写实例；禁止同库legacy/V2同时运行。

| 配置 | 默认/职责 |
| --- | --- |
| `APP_PORT` | 8080，公共HTTP/WS |
| `PVE_TEST_EVENTS_ENABLED` | false；仅测试时开启 |
| `PVE_TEST_EVENTS_ADDR` | 127.0.0.1:8090，启动拒绝非loopback |
| `PVE_TEST_EVENTS_TOKEN` | 独立测试服务凭据，至少24字符 |
| `PVE_METRICS_ENABLED/ADDR/TOKEN` | false/127.0.0.1:8091/独立Bearer凭据；不得复用事件token |
| `PVE_ARCHIVE_ENABLED` | false；day39就绪后才可开启 |
| `PVE_ARCHIVE_RETENTION_DAYS/BATCH_SIZE` | 90/100；保留原指纹、序号和奖励业务键 |
| `WS_MAX_CONNECTIONS/WS_MAX_CONNECTIONS_PER_IP` | 256/64；同玩家连接替换仍可用 |

`/health`仅确认HTTP进程响应，不证明全链路健康。V2动作走 `/ws`，快照走 `/api/v2/*`；传 `schema_version=2`、`v2.*`、稳定 `operation_id`。同操作不同内容冲突；过期回执被压缩后返回41071，查询当前活动恢复，不能重执行旧意图。

GM/PVE网页在各自 `frontend`目录执行 `npm ci --ignore-scripts`、`npm run build`、`npm run dev`；开发代理指向本机8080。GM默认只读观察；仅实时复核为operator的管理员可提交needs_repair重试，必须先消除原故障，不能直接加币。Unity使用本目录工程，不自动调用内部事件。

## 镜像、监控与测试

```powershell
docker build -t game-realtime-gm:r3-local backend
```

Dockerfile使用Go1.27.1构建和非root distroless运行。运行容器仍需显式数据库/Redis地址和密钥；容器内localhost不是宿主机。现有时区语义要保留，旧DATETIME不能无条件按UTC解释。

`deploy/docker-compose.monitoring.yml`从 `PVE_MONITORING_CONFIG_DIR` 挂载prometheus.yml和metrics-token，Grafana密码通过 `PVE_GRAFANA_PASSWORD` 提供。使用 `deploy/monitoring/prometheus.example.yml` 模板，保持Prometheus/Grafana端口仅绑定本机。本机验证已能采集宿主机loopback指标；其他系统需先验证宿主机代理访问，不为方便而公开指标/事件端口。告警规则展示采集失败、缺号、积压和needs_repair；尚未接外部告警通知。

CI在Linux执行Go普通测试/vet、真实隔离MySQL/Redis V2 race、双前端npm ci/build。本机独立测试使用23306/26379、库 `game_realtime_v2_test`、Redis DB14。不要把fixture指向开发库，CI结束后必须清理测试资源。Unity验收与Docker构建分别验证，CI绿色不代替它们。

当前实例提供 `GET/POST /api/admin/v2/control`，operator 可按版本和稳定操作键执行 `open/draining/closed`；Worker、重连及已分配Run继续处理，排空前不能直接停止进程。旧实时观察 `/api/admin/realtime/*` 仅在 legacy 模式注册，V2 使用 `/api/admin/v2/observations/{entity}`。见 [R4实施](design/v2-r4-implementation.md)。
