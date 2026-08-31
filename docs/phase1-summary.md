# 一期成果与工程证据

> 文档角色：Day27-Day35 阶段复盘与证据摘要
> 权威级别：L2（已完成阶段记录）
> 状态：一期已完成
> 适用范围：求职展示、阶段复盘与后续验收基线
> 事实来源：代码、测试、Day27-Day35 验收与 Day34 性能记录
> 最后更新：2026-08-31

## 阶段目标

一期将基础账号与 GM 后台扩展为可验证的共斗 PVE 业务后台闭环，同时明确它不是完整战斗服。

## Day27-Day35 交付

| Day | 已完成能力 | 主要证据 |
| --- | --- | --- |
| 27 | 小队状态广播、统一 WS 错误、消息大小限制 | Handler/消息协议、小队测试 |
| 28 | 任务会话状态机、断线/重连与队长转移 | mission/squad tests |
| 29 | Redis 匹配 ticket、取消、超时和清理 | matchmaking tests、Redis 对照 |
| 30 | 任务结算、服务端计算、nonce | schema、settlement 代码 |
| 31 | 幂等资产强事务、余额和流水 | UNIQUE、`FOR UPDATE`、事务测试 |
| 32 | Redis 最佳分排行、同分顺序、战绩查询 | leaderboard tests、HTTP API |
| 33 | GM 实时摘要、玩家观察、结算与榜单 | observation tests、Request ID 日志 |
| 34 | `ws_bot`、pprof、Linux race、性能基线 | `performance/day34-baseline.md` |
| 35 | README、架构、数据流和一期边界收口 | 公开文档与验收记录 |

## 验证证据

- Go 单元测试覆盖配置、Request ID、访问日志、WebSocket Manager、小队、任务、匹配、结算、排行、观察和 `ws_bot` 指标。
- `go vet ./...` 通过。
- 官方 Linux Go 镜像内 `go test -race ./...` 通过；只覆盖自动测试执行到的路径。
- Day34 本机 8 客户端冒烟与 20 客户端、10000 echo 短时基线均记录为 0 失败。
- AccessLog 测试与实际日志确认只记录 path，不输出 WebSocket query token。
- 原始 profile、goroutine 和运行日志保存在仓库外，没有作为公开材料提交。

## 发现并修复的问题

Day34 真实冒烟发现小队离队后成员切片未写回，补充修复与回归测试；同时将 `ws_bot` 高频等待改为静默 timer，避免压测日志淹没结果。这些属于测试环境工程证据，不是生产事故经验。

## 当前限制

- 单机、单进程、回环网络，客户端与服务端共享一台机器。
- 小队和任务会话重启后清空。
- 匹配没有成功撮合与 `matched` 状态。
- Redis 排行榜没有自动重建或赛季管理。
- 没有正式 React/Unity 客户端、云部署、CI/CD、弱网或长时间 soak。
- 没有登录限流、token 撤销、刷新 token 和正式 Secret 管理。
- Day34 数据不是容量上限、SLO、商业 CCU 或生产性能证明。

## 一期结论

项目已经具备可运行、可测试、可追踪、可解释的 Go 游戏业务后台闭环。后续工作应优先补真实工程缺口和展示客户端，而不是把当前服务重新命名为微服务或战斗服。

详细测试边界见 [测试计划](test-plan.md)，架构边界见 [当前系统架构](architecture.md)。
