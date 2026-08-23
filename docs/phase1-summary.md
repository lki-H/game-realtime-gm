# 一期成果总结

> 状态：一期完成
> 最后更新：2026-08-23
> 适用范围：公开项目文档 / 项目复盘

## 一期目标

一期目标是完成一个可运行、可测试、可解释的单体 Go 游戏业务后端。

它覆盖账号、权限、实时连接、小队、任务会话、匹配 ticket、幂等结算、资产、排行榜和 GM 观察，但不包含逐帧战斗模拟。

## 技术栈

- Go 1.25。
- Gin。
- `database/sql` + MySQL Driver。
- MySQL 8.4.11。
- Redis 7。
- Gorilla WebSocket。
- JWT + bcrypt。
- Docker Compose。

## 能力矩阵

| 能力 | 主要代码 | 数据位置 | 验证证据 |
| --- | --- | --- | --- |
| 玩家注册登录 | `handler/auth.go`、`auth` | MySQL | API 测试、单元检查 |
| 玩家/管理员权限隔离 | `middleware/auth.go` | JWT claims | 403/401 真实流程 |
| GM 玩家管理与审计 | `handler/admin*.go` | MySQL | 封禁、解封、日志查询 |
| WebSocket 生命周期 | `handler/ws.go`、`ws` | 内存 + Redis TTL | 连接替换、ping/pong、测试 |
| 小队状态 | `squad` | Go 内存 | create/join/ready/leave、广播测试 |
| 任务会话状态机 | `mission` | Go 内存 | 合法/非法迁移测试 |
| 匹配 ticket | `matchmaking` | Redis | enqueue/me/cancel/timeout |
| 幂等结算 | `settlement` | MySQL | 三层唯一约束、重复请求 |
| 玩家资产 | `settlement`、`model` | MySQL | FOR UPDATE、事务、ledger |
| 排行榜 | `leaderboard` | Redis + MySQL | 同分先到、Top N、战绩 |
| GM 实时观察 | `observation` | 内存 + Redis + MySQL | 摘要、玩家上下文、筛选 |
| 工程诊断 | `diagnostics`、`ws_bot` | 本机 | pprof、race、性能报告 |

## 核心工程决策

### 保持单体

一期使用单体 Go 进程，便于学习、调试、测试和解释。当前规模没有证据要求拆分微服务。

### MySQL 与 Redis 分工

```text
MySQL：长期事实、事务、唯一约束、资产和审计
Redis：在线 TTL、匹配状态、排行榜查询投影
内存：当前连接、小队和任务会话
```

### 任务会话不是战斗服

`mission_instance` 只表示任务业务与结算生命周期。当前没有固定 Tick、物理、技能、AI、预测或延迟补偿。

### 资产强事务

任务记录、奖励记录、余额和流水在同一 MySQL 事务中提交。核心资产不使用纯异步 Redis 写回。

### Redis 排行榜是投影

排行榜在 MySQL 结算成功后 best-effort 更新。Redis 失败不能回滚已经成功的资产事务，投影允许重建。

### 只读观察不污染危险操作审计

Dashboard 和实时观察使用标准日志。封禁、解封等危险写操作继续写 `admin_operation_logs`。

## 测试与验证

### 常规检查

```powershell
go test ./...
go vet ./...
docker compose config
```

### Race

Windows 本机没有 GCC。使用官方 Go Linux Docker 镜像执行：

```powershell
docker run --rm -v "${PWD}:/workspace" -w /workspace golang:1.25-bookworm go test -race ./...
```

全部包通过。Race 只覆盖测试实际执行到的路径。

### Day34 本机基线

```text
20 个 WebSocket 客户端
5 个四人小队
10000 次 debug.echo
所有 stage failure=0
echo Average=106us
echo P95=611us
echo Maximum=2.513ms
hold goroutine=70
cleanup goroutine=8
```

这些数字来自同机回环网络，只用于后续同环境对比。

## 真实问题与修复

| 问题 | 根因 | 修复 |
| --- | --- | --- |
| 旧连接断开可能影响新连接 | 只按 player_id 注销 | connection_id 条件注销 |
| 排行榜同分顺序不稳定 | 只使用普通分数排序 | 反向时间 member 编码 |
| 重复结算可能重复发奖 | 缺少完整业务约束 | 三层唯一约束 + 事务 |
| 离队成功后成员仍残留 | `nextMembers` 未写回 | 写回成员切片并补回归测试 |
| WS query token 进入访问日志 | Gin 默认 URI 日志 | 自定义只记录 path 的 AccessLog |
| 高频 echo 产生海量日志 | 原始 payload 和 interval 全量日志 | 删除 payload 日志，interval 静默等待 |

## 当前边界

- 单实例 Go 服务。
- 小队和任务会话重启后清空。
- 匹配没有 matched 撮合算法。
- 没有完整资产查询和补偿 API。
- Redis 排行榜没有自动重建任务。
- 没有 React GM 页面或 Unity Demo。
- 没有公网部署、弱网、soak 或多实例验证。
- Day34 性能结果不是生产容量。

## 一期完成标准

- [x] 主要业务模块形成闭环。
- [x] HTTP 和 WebSocket 契约可验证。
- [x] MySQL、Redis 和内存职责清楚。
- [x] 核心资产具备事务和幂等保护。
- [x] 管理员权限和危险操作审计存在。
- [x] 单元测试、vet 和 Docker race 通过。
- [x] 有自动多玩家工具和性能基线。
- [x] 测试数据已清理。
- [x] 架构、数据流、API 和性能证据可阅读。
- [x] 已知限制没有包装成已实现能力。

## 文档入口

- [项目入口和快速启动](../README.md)
- [当前系统架构](architecture.md)
- [关键业务数据流](data-flow.md)
- [API 与 WebSocket](api-overview.md)
- [性能和 race 证据](performance/day34-baseline.md)
