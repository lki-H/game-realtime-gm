> 状态：随实现更新
> 适用范围：项目实战 / 求职展示
> 最后更新：2026-07-12

# 工作实战版数据库设计

## 当前已实现表

### players

保存玩家账号、昵称、状态和封禁信息。

核心字段：

- `id`
- `username`
- `password_hash`
- `nickname`
- `status`
- `banned_reason`
- `banned_at`
- `banned_by_admin_id`
- `created_at`
- `updated_at`

### admins

保存管理员账号、展示名和角色。

核心字段：

- `id`
- `username`
- `password_hash`
- `display_name`
- `role`
- `created_at`
- `updated_at`

### admin_operation_logs

保存 GM 操作审计记录。

核心字段：

- `admin_id`
- `admin_username`
- `admin_role`
- `action`
- `target_type`
- `target_id`
- `detail`
- `ip`
- `user_agent`
- `created_at`

## 后续规划表

Day28-Day35 后根据实际实现补齐：

- `mission_instances`
- `mission_participants`
- `matchmaking_tickets`
- `settlement_records`
- `reward_records`
- `leaderboard_records` 或 Redis ZSet 旁路持久化记录

## Redis 数据

当前已使用：

```text
online:player:<id>
```

后续规划：

- 匹配 ticket。
- 匹配队列。
- 排行榜 ZSet。
- 临时任务状态。

## 设计原则

- PostgreSQL 保存长期可靠数据。
- Redis 保存实时、高频、可重建状态。
- 核心奖励和结算不只依赖 Redis。
- 演示 seed 只服务本地开发和展示，不是生产初始化方案。

## 当前不做

- 不做复杂分库分表。
- 不做 Write-Back 大宽表。
- 不把 Redis 当长期可靠数据库。
