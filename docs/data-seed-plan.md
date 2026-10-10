# 演示数据与 Seed 方案

> 文档角色：本地演示数据创建、标识、重置与安全边界
> 权威级别：L3（演示规划）
> 状态：管理员 seed 已存在；业务数据按需创建
> 适用范围：本地/测试演示
> 事实来源：当前 schema、`seed.sql`、注册/业务接口
> 最后更新：2026-08-31

## 数据来源

- `backend/internal/database/seed.sql`：幂等创建本地管理员；任何共享环境必须替换凭据。
- 玩家：优先通过 `/api/register` 创建，确保同时生成 `player_assets`。
- 小队、任务会话：只能通过 WebSocket 流程创建，保存在当前进程内存中，不写伪造 SQL seed。
- 结算、奖励、余额、流水：只能通过服务端 `settlement.create` 生成，不直接 INSERT/UPDATE 伪造资产。
- 排行榜：由 settled 结果同步，不手工 ZADD 作为正式演示结果。

## 命名规则

为一次演示选择唯一前缀，例如：

```text
username: demo0831_player01
mission_id: demo0831_training
request_id: demo0831_req_001
idempotency_key: demo0831_settlement_001
nonce: demo0831_nonce_001
```

前缀便于查询和精确清理，不应包含真实姓名、手机号或邮箱。

## 演示角色

- 两到四名正常玩家：小队、任务、奖励和排行榜。
- 一名临时封禁玩家：验证登录 403、封禁/解封审计。
- 一名本地管理员：查询和危险操作。

不要把演示账号或已知密码用于公网/共享环境。

## 重置原则

1. 优先使用新的唯一前缀，避免覆盖需要保留的学习数据。
2. 清理前先查询受影响玩家、reward、mission 和 ledger 数量。
3. 按关系顺序处理测试记录；资产流水是 append-only 业务数据，本地精确清理只是测试环境例外。
4. 清理 Redis online/match/leaderboard key 时必须限定本次玩家/任务，禁止无审查的 `FLUSHALL`。
5. 不删除 Docker volume 作为日常数据重置方式。
6. 清理命令和结果记录在本地验收日志，不提交账号备忘或 dump。

## 当前不提供

- 一键生产数据生成器。
- 直接写入任务、奖励、余额和排行榜的假数据脚本。
- 包含默认公开密码、token 或真实个人信息的 fixture。
