# 测试计划、用例与需求追踪

> 文档角色：测试范围、用例、证据、缺陷与进出场规则
> 权威级别：L1（测试事实源）
> 状态：一期基线已建立，持续补充
> 适用范围：当前 Go 单体服务与本地/测试环境
> 事实来源：需求、自动测试、Day27-Day35 验收与 Day34 性能记录
> 最后更新：2026-10-10

## V2/R4发布验证

`backend/tests/v2` 使用独立MySQL/Redis和 `PVE_INTEGRATION=1` 验证迁移、Party、proposal、Run、任务、事件、结算、Worker、资产对账和投影重建。GitHub Actions另外运行Go普通测试/vet、Linux `go test -race`和双前端干净构建；Unity Player、GM修复、备份恢复和soak作为单独证据，不混作CI覆盖。跨机器、长时弱网和商业容量仍未验证。

R4新增维护/排空、并发控制/修复回执、权限撤销竞态、legacy时间、101个Run轮转、数据库锁丢失及缓存白名单回归。CI设置R4/R3二进制路径，实际进程经HTTP重放及Worker恢复验证资产，不接受回滚测试SKIP作为证据。清单见 [R4复核](testing/r4-full-review-20261008.md)。

## 1. 测试目标

验证身份边界、状态机、幂等资产事务、Redis 投影、GM 观察和并发保护；同时保留可复现环境、命令、结果与局限。前端规划不替代后端测试。

## 2. 环境

| 环境 | 用途 | 当前边界 |
| --- | --- | --- |
| Windows 宿主机 | 普通测试、vet、服务启动和接口验证 | 本机无 GCC，不能原生 race |
| Docker MySQL 8.4 / Redis 7 | 集成与数据对照 | 单容器开发配置 |
| 官方 Linux Go 镜像 | `go test -race ./...` | 只覆盖测试执行路径 |
| 本机 `ws_bot` | 多连接短时冒烟和基线 | 回环网络，不代表容量 |

## 3. 测试层级

| 层级 | 目标 | 当前工具 |
| --- | --- | --- |
| 静态 | 格式差异、编译问题和 vet 诊断 | `gofmt -d`、`go test`、`go vet` |
| 单元 | Manager 状态机、校验、排序、Request ID | Go test |
| 集成 | MySQL/Redis 数据边界和事务结果 | Docker + CLI + API |
| HTTP 接口 | 状态码、业务 code、鉴权、分页/筛选 | PowerShell/curl/Apifox |
| WebSocket | envelope、广播、断线/重连、状态流转 | Apifox WS、`ws_bot` |
| 并发 | 共享 map、锁、重复连接和 race | 单元测试 + Linux race |
| 性能 | 短时连接/echo 基线与 pprof | `ws_bot`、pprof |
| 安全 | 身份混用、日志泄露、输入与资源边界 | 自动测试 + 手工用例 |

## 4. 工程证据等级

| 等级 | 定义 | 本项目示例 |
| --- | --- | --- |
| E0 | 知识或设计记录 | 需求、ADR、安全设计 |
| E1 | 可重复运行原型 | Go 服务与 Docker 依赖 |
| E2 | 自动验证 | 单元测试、vet、race |
| E3 | 数据报告 | Day34 `ws_bot`/pprof 基线 |
| E4 | 测试环境工程演练 | 未来故障、发布、恢复演练 |
| E5 | 真实生产经验 | 当前没有，不能宣称 |

E1-E4 报告应记录机器、代码版本、配置、数据量、命令、持续时间、结果和局限。

## 5. 自动测试清单

当前测试覆盖：

- 配置布尔值读取。
- Request ID 合法值保留与非法值替换。
- AccessLog 不记录 query。
- WebSocket Manager 只注销当前 connection、连接状态查询。
- 小队断线清 ready、队长转移、重连、统计、离队与解散。
- 任务正常/取消路径、非法迁移、重复活跃任务和 ID 唯一性。
- 匹配状态迁移、标识符和 timeout member 编解码。
- 结算请求键、参与者、finished 校验、分数和余额边界。
- 排行 member 编解码、同分排序和 Redis hash tag。
- GM 结算筛选 SQL 片段。
- pprof 路由注册。
- `ws_bot` 参数和指标快照。

这些测试尚未完整覆盖真实 MySQL/Redis 集成、全部 Handler 响应和 WebSocket 全业务端到端。

## 6. 需求追踪与核心用例

| 用例 ID | 需求 | 场景 | 预期 |
| --- | --- | --- | --- |
| `TC-OPS-001` | REQ-OPS-01 | `GET /health` | 200、`code=0`、响应头有 request ID |
| `TC-AUTH-001` | REQ-AUTH-01 | 新用户名注册 | 201；玩家和零余额资产行同时存在 |
| `TC-AUTH-002` | REQ-AUTH-01 | 重复用户名 | 409、`40901`；无重复资产行 |
| `TC-AUTH-003` | REQ-AUTH-02 | 正确/错误密码、封禁登录 | 分别 200、40101、40321 |
| `TC-AUTH-004` | REQ-AUTH-03 | 玩家 token 访问 admin、反向混用 | 分别 40311、40301/WS 40331 |
| `TC-PLAYER-001` | REQ-PLAYER-01 | 查询/改昵称/分页搜索 | 字段正确；空昵称 40003；page_size 截断 50 |
| `TC-PRESENCE-001` | REQ-PRESENCE-01 | HTTP 心跳后查询 | online=true，TTL 约 120 秒 |
| `TC-PRESENCE-002` | REQ-PRESENCE-01 | WS 断开后等待 TTL | 停止续期，最终 online=false |
| `TC-WS-001` | REQ-WS-01 | 连接、welcome、ping/pong | 连接 ID、玩家身份和期限正确 |
| `TC-WS-002` | REQ-WS-01 | 同玩家第二连接 | 新连接替换旧连接，旧连接关闭不删新连接 |
| `TC-WS-003` | REQ-WS-01 | 非文本/非法 JSON/无 type/超大消息 | 对应错误或连接关闭，无 panic |
| `TC-SQUAD-001` | REQ-SQUAD-01 | 创建、加入、ready、离队 | 直接响应和其他成员广播一致 |
| `TC-SQUAD-002` | REQ-SQUAD-01 | 断线、队长转移、重连 | offline/ready/leader 状态符合规则 |
| `TC-SQUAD-003` | REQ-SQUAD-01 | 重复加入、满员、不存在 | 40926、40927、40427 |
| `TC-MISSION-001` | REQ-MISSION-01 | 完整任务生命周期 | waiting/ready/running/finished 与时间字段正确 |
| `TC-MISSION-002` | REQ-MISSION-01 | 非队长、成员离线/未 ready、非法迁移 | 40332、40928/40929、40931 |
| `TC-MATCH-001` | REQ-MATCH-01 | enqueue/me/cancel | Hash、player index、queue、timeout index 一致 |
| `TC-MATCH-002` | REQ-MATCH-01 | 重复排队与 30 秒超时 | 40933；终态 timeout 并移出索引/队列 |
| `TC-SETTLE-001` | REQ-SETTLE-01 | finished 任务首次结算 | 记录、奖励、余额、ledger 同事务成功 |
| `TC-SETTLE-002` | REQ-SETTLE-01 | 相同请求/同任务换 key 重试 | 返回已有结果，不重复发奖或广播 |
| `TC-SETTLE-003` | REQ-SETTLE-01 | nonce 重放/key 跨任务/未 finished | 40937、40940、40936，无资产变化 |
| `TC-SETTLE-004` | REQ-SETTLE-01 | 中途 SQL 失败 | 整个事务回滚，余额与流水一致 |
| `TC-RANK-001` | REQ-RANK-01 | 新高分、低分、同分 | 只保留最佳；同分先达到者优先 |
| `TC-RANK-002` | REQ-RANK-01 | Redis 同步失败后幂等重试 | MySQL 不回滚，重试可修复投影 |
| `TC-GM-001` | REQ-GM-01 | 封禁/解封/重复操作 | 状态与审计原子；重复操作 409 |
| `TC-GM-002` | REQ-GM-02 | 审计分页、筛选和详情 | 总数、顺序、筛选、错误码正确 |
| `TC-GM-003` | REQ-GM-02 | 实时摘要和玩家观察 | 跨内存/Redis/MySQL 字段与 observed_at 正确 |
| `TC-NFR-001` | REQ-NFR-01 | 服务重启 | 内存状态清空，MySQL 长期事实保留 |
| `TC-NFR-002` | REQ-NFR-02 | `go test`/vet/Linux race | 命令退出 0，记录环境与覆盖限制 |
| `TC-NFR-003` | REQ-NFR-03 | `/ws?token=...` 访问日志 | 日志仅有 `path=/ws`，无 query/JWT |

## 7. 标准命令

源代码静态与单元验证：

```powershell
cd .\backend
$goFiles = Get-ChildItem .\cmd,.\internal -Recurse -Filter *.go | Select-Object -ExpandProperty FullName
gofmt -d $goFiles
go test ./...
go vet ./...
```

Linux race：

```powershell
docker run --rm `
  -v "${PWD}:/workspace" `
  -w /workspace `
  golang:1.25-bookworm `
  go test -race ./...
```

普通测试通过不等于 race 通过；race 通过也只覆盖实际执行路径。

依赖与 schema 启动见 [部署手册](deployment-runbook.md)。接口测试必须使用专用本地账号和唯一前缀，结束后按关联顺序清理测试数据。

## 8. 性能与稳定性

Day34 已建立本机回环 E3 基线：8 客户端冒烟和 20 客户端、10000 echo 短时运行均记录 0 失败，连接关闭后 goroutine 回落。公开证据摘要见 [一期成果](phase1-summary.md)。

后续对比必须保持机器、Go 版本、代码版本、服务配置和 `ws_bot` 参数一致。尚未验证：公网延迟、弱网、长时间 soak、多实例、数据库高负载或容量上限。

## 9. 安全测试

- 玩家/管理员 token 互换和过期 token。
- WebSocket query token 不进入访问日志。
- SQL 特殊字符只作为参数，不改变查询结构。
- 超长昵称、原因、request ID、匹配字段、nonce 和 idempotency key。
- 未登录连接洪泛、登录暴力尝试和 HTTP 大 body 当前应记录为待加固用例，而不是已通过控制。
- pprof 默认关闭且只在明确启用时绑定 loopback。

风险优先级见 [安全设计](security-design.md)。

## 10. 缺陷管理

| 严重度 | 定义 | 处理规则 |
| --- | --- | --- |
| S0 | 资产重复/越权、凭据泄露、数据不可恢复 | 停止发布，先修复并回归 |
| S1 | 核心流程不可用、事务不一致、持续资源泄漏 | 一期/当前版本阻塞 |
| S2 | 有绕行方案的功能错误或边界错误 | 进入近期 backlog并补回归测试 |
| S3 | 文案、日志或低风险体验问题 | 可随版本整理 |

缺陷记录至少包含：环境、代码版本、前置数据、步骤、期望、实际、日志/request ID、严重度、负责人状态、修复 commit 和回归结果。状态使用 `New -> Confirmed -> Fixing -> Fixed -> Verified -> Closed`；无法复现必须记录尝试条件。

## 11. 进出场标准

进入阶段验收前：依赖可启动、schema/seed 明确、测试数据可识别、需求与契约已更新。

完成标准：

- `gofmt -d` 无输出，`go test` 与 `go vet` 退出 0。
- 涉及并发的阶段完成匹配的 race 验证或明确记录未执行原因。
- 正常、边界、鉴权、重复请求与一致性用例通过。
- 没有未处理 S0/S1。
- 文档、OpenAPI、WebSocket 契约与实际路由一致。
- 证据真实标注为 E0-E4，不包装成 E5。

## 12. V2 验收入口

2026-10-10复核增加 `review_20261010_test.go`，覆盖JWT时间、已建立WS过期、异常outbox隔离/排空、加载失败回队列、成员拉黑及不兼容/永久退出任务尝试终态。R5增加poison redelivery死信和过期已确认消息去重；完整复核证据见 [本轮记录](testing/20261010-full-review.md)。

R5 的独立模块验收另见 [RPC/MQ 验收](testing/r5-m6-acceptance.md)，不由主 `backend/go test ./...` 自动覆盖。`experiments/r5-m6/deploy/verify.ps1` 验证真实备份恢复、只读 SQL 权限、服务身份/deadline、confirm/ACK/重投/死信/冲突、晚提交回查、投影重建、实际进程崩溃、broker/数据库重启以及主链路故障隔离；Linux race 包含真实 MySQL/RabbitMQ。普通无依赖单元测试中的 Integration SKIP 必须如实标注。
