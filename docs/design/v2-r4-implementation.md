# R4 维护、切换回滚与 legacy 退役

> 文档角色：本机 R4 实施边界与操作说明
> 权威级别：L2
> 状态：本机实现与验证完成；跨机器验收延期
> 最后更新：2026-10-08

用户授权推进维护排空、隔离切换回滚和分批退役，跨机器验收以后再执行。本轮只调整本机开发路线，不把延期项记录为通过，不部署共享或公网服务。

## 1. 维护准入

`day40_v2_retirement.sql` 新增 `pve_service_control` 和 `pve_control_operations`。沿用编号 SQL、checksum 与数据库排他锁，执行 `go run ./cmd/tools/v2_migrate -stage r4` 顺序验证 day37—40；不修改已应用迁移字节，不搬迁旧历史，不删除资产。

| 状态 | 新匹配和新 Run | 已有活动 | 进入方式 |
| --- | --- | --- | --- |
| `open` | 允许 | 正常运行 | 初始化或 operator 恢复 |
| `draining` | 拒绝 | 已分配 Run、可信事件、重连、结算与 Worker 继续 | operator 提交版本化维护请求；未分配票据取消，候选中止，清准备 |
| `closed` | 拒绝 | 历史与局外查询可用 | 必须先 draining，且所有检查计数为零 |

状态在 MySQL 持久化，进程重启只 `INSERT IGNORE` 初始化，不重新打开准入。匹配和 Run 创建事务取得控制行共享锁，维护变更取得同一行排他锁，并按匹配 Lane 顺序加锁；数据库故障不绕过检查。

取消票据与清准备保留历史记录，通知通过 outbox。维护期间加载失败不会将正常来源重新排队。已分配 Run 不因为进入维护而伪造失败或成功，仍按可信事件和原时限结束；故障结算继续需要修复。

## 2. HTTP 管理入口

`GET /api/admin/v2/control` 允许已认证且实时复核的管理员查看维护状态、版本、理由、更新时间及一致快照中的在途计数。包含 queued/proposed 票据、pending 候选、活动/ending Run、pending/retryable_failed/needs_repair 工作、活动锁和待发布 outbox；`ready_to_stop` 仅在 draining 且计数全部为零时为 true。

`POST /api/admin/v2/control` 仅实时 operator 可操作。请求示例中的 token 由操作者登录取得，不写入文件或命令历史：

```json
{
  "operation_id": "maintenance_example_001",
  "admission_state": "draining",
  "expected_version": 1,
  "reason": "本机维护与切换"
}
```

先 GET 获取当前版本，再 POST。重试同一意图保持 `operation_id`、版本和内容不变；返回原回执，不重复维护或审计。新意图使用新键和最新版本，同键不同内容及旧版本返回 409。理由必填且最多 512 字节；请求按入口限流。状态、幂等回执和 `admin_operation_logs` 同事务，审计失败不提交维护变更。旧 operator JWT 在角色降级后不能操作。

排空完成后再提交 closed，确认后停止进程。HTTP 健康响应不是排空证据。该开关只控制玩法准入，社交、历史和任务查询仍可用；进程停机本身仍由受控宿主机完成。

## 3. 默认与退役

默认 `GAMEPLAY_MODE=v2`；旧模式必须显式设置 `legacy`。V2 不装配旧房间、任务、匹配 Worker 或结算写服务，旧装配集中在 `router/legacy.go`；V2 WS 对旧命令返回 40971。

仍被历史查询、学习回归和 `ws_bot` 引用的旧包与测试保留。它们只服务显式 legacy 回归，不属于当前 PVE 链路。legacy 进程在持有同库排他锁后检查 V2 活动、票据、候选、待结算及通知均空闲；否则拒绝启动。仅切环境变量不是合法的运行中回滚。

保留 `/api/me/mission-records`、历史排行和 GM 结算查询，不把旧 mission 转成 Run。V2 下历史观察不再读取过期的 legacy 内存与匹配队列；需要当前 PVE 状态时使用 V2 观察接口。

R4复核增加：旧战绩和GM结算时间按原Asia/Shanghai解释；管理员重试在操作行锁后同事务复核operator；数据库锁连接失效触发服务停止；活跃Run维护批次按 `updated_at,id` 轮转，避免后续Run长期饥饿。

## 4. 旧 Redis 清理

`go run ./cmd/tools/pve_retire` 默认只检查，输出目标库、Redis DB、维护快照和旧匹配 key 数量，不打印密钥或玩家记录。使用显式 `DB_*` 和 `REDIS_*`；工具要求 V2 模式，且同库玩法服务已停止，避免另一写实例继续生产旧缓存。

只枚举 `matchmaking:queue:*`、`matchmaking:ticket:*`、`matchmaking:player:*`、`matchmaking:timeouts`，SCAN 每批 100，最多 10000 个去重 key；超过上限拒绝并另行安排分批清理。执行 `-apply` 前需要环境变量 `PVE_RETIRE_CONFIRM=I_UNDERSTAND_LEGACY_CACHE_REMOVAL`、已排空或 closed、无 V2 待处理事实。逐批 UNLINK，失败后检查实际数量再重试。

不使用 FLUSH/KEYS，不删除 `v2:*`、`online:*`、历史榜单或其他命名空间。无旧 key 也属于有效检查结果，不为演示而在开发库添加测试数据。

## 5. R3 实际版本回退

R3 基线没有维护开关，所以回退时不能只依赖 day40 控制行。先在 R4 进入 draining 并完成结算，closed 后停止；记录已发布规则原状态，把准入规则标记为 disabled，再启动审核过的 R3 二进制，显式 `GAMEPLAY_MODE=v2`、归档关闭、内部事件默认关闭。

R3 会保留相同 MySQL Run、结果、奖励唯一键、账本与 schema，disabled 规则阻止新匹配分配和 Run 创建。不要删除 day40 表、迁移账本或恢复旧备份覆盖活动库，不回退为 legacy 发奖。回到 R4 时保持 closed，核对事实、余额、流水、pending 和旧历史，再按记录恢复规则，最后由 operator 恢复 open。

`deploy/verify-r4.ps1 -RollbackBackendPath <已审核R3工作区的backend目录>` 构建实际 R3/R4 二进制，在新隔离库验证有奖励的四人 Run、默认 V2、旧 finish 拒绝、历史结果查询、重复事件与结算去重，退出时清理专用容器、测试卷和临时二进制，恢复脚本环境变量。短时演练在完全排空后停止测试进程，不把强停进程当排空手段。

## 6. 完成边界

本机切换与退役、实际二进制回退、Go回归和数据迁移分别留存证据。跨机器验收、共享观察窗口、彻底删除旧学习包和真实游戏服仍是独立事项；远程发布/CI单列于 [R4发布记录](../testing/r4-release-acceptance.md)，不把本机验证描述为生产验收。
