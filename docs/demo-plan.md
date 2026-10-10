# 演示方案

> 文档角色：当前后端演示脚本与未来客户端验收
> 权威级别：L3（演示规划）
> 状态：后端、React与Unity Windows控制面已实现并本机验证；跨机器延期
> 适用范围：面试、录屏和阶段验收
> 事实来源：当前 API/WS 契约、测试计划与前端规划
> 最后更新：2026-10-10

## 1. 当前可执行演示

默认演示使用 V2、隔离 MySQL/Redis、PVE 网页或 Unity Windows Player，以及 React GM：

1. 执行编号迁移并启动 V2 服务；检查 `/health` 活性与 `/ready` 就绪。
2. 注册/登录四名合成玩家，建立 `schema_version=2` 的 WebSocket。
3. A 创建好友房间并邀请 B；C、D 单排。选择兼容、不兼容和可选个人任务，说明共同目标与个人进度的区别。
4. 准备、整组入队、逐人确认，验证四人获得同一 Run；房主不能结束 Run 或踢参战成员。
5. 在服务机用受限 `pve_event_bot` 提交测试事件；共同目标完成后服务端统一结束，Worker 逐人可靠结算。
6. 断线重连及 HTTP 查询恢复结果；重复提交同一事件或重试原 operation，不增加重复奖励。
7. GM 查看 Party、Proposal、Run、任务、待处理记录和通知；使用 HTTP API 验证封禁/解封与审计。
8. 对照 Reward/资产流水/余额及 Redis 投影；演示一次故障恢复和明确的失败原因。

四个真实 Player 脚本为 `deploy/verify-r3-unity.ps1`，混排使用 `-MixedParty`；自动后端闭环和持续运行使用 `backend/cmd/tools/pve_verify`。测试事件凭据仅保留在服务机父进程，不能输入网页或 Unity。

## 2. 必演失败场景

- 玩家/管理员 token 混用。
- 非房主修改好友房间方案、旧准备版本、匹配中名单变更。
- 匹配重复排队、候选拒绝/超时、加载失败和原来源回退。
- 普通玩家访问内部事件、旧来源事件、同 ID 不同内容、重复发奖。
- 不兼容个人任务参战但不推进，主动退出停止后续贡献。
- 重复封禁或解封未封禁玩家。

## 3. 演示数据

- 使用固定前缀的本地测试账号和任务 ID，便于清理。
- 演示前记录初始余额、结算数和排行榜；演示后对照增量。
- 不使用真实个人信息、真实密码或生产数据。
- seed 和清理规则见 [演示数据方案](data-seed-plan.md)。

## 4. 当前 GM 观察窗

- 已实现管理员登录、六类 V2 只读观察、分页/刷新，以及 operator 受控重试和确认。
- 玩家管理、封禁/解封、审计及 legacy 历史查询的后端 API 已实现；当前 React 页面不宣称已覆盖所有管理 API。
- loading/empty/error/token 失效均有明确状态。
- 展示 request ID 与 observed_at，不把前端轮询称为实时全服监控。

## 5. 当前 Unity 控制面

- Windows Player 通过真实 HTTP/WS 执行登录、Party、任务选择、匹配确认、Run 查询、重连和逐人结果。
- 四人单排与好友双排混排有本机验收；它验证控制面契约，不模拟真实战斗。
- Unity Editor 场景交互、跨机器、移动端、弱网和真实 DS 继续按明确范围另验。

## 6. 录屏与截图

- 开场：README、架构图和能力边界。
- 主体：成功闭环 + 一个鉴权失败 + 一个幂等重试。
- 证据：MySQL 事务结果、Redis key、测试命令和性能基线摘要。
- 结尾：明确单实例、MySQL 事实与 Redis 投影、独立 RPC/MQ 实验，以及跨机器/真实战斗服/生产容量边界。

录制前清理 token、密码、绝对路径、账号备忘、原始日志/profile 和私人材料。

## 7. 验收来源

功能正确性按 [测试计划](test-plan.md)，运行排障按 [部署手册](deployment-runbook.md)，契约按 [OpenAPI](openapi.yaml) 与 [WS 协议](ws-protocol.md)。演示通过不能替代自动测试。

## 8. legacy 历史演示

`ws_bot` 与旧 `squad/mission/finish/settle` 仅在显式 `GAMEPLAY_MODE=legacy`、隔离空闲库中用于一期学习回归。不得把该房主 finish 流程用于 V2 可信完成或公开共享奖励。历史接口不迁移为 V2 Run，不和 V2 同时写同一套资产。
