# 备份与恢复方案

> 文档角色：本地/测试数据备份、恢复与演练规则
> 权威级别：L1（恢复流程事实源）
> 状态：手工流程；尚无自动灾备
> 适用范围：Docker MySQL/Redis 开发环境
> 事实来源：当前 Compose volume、MySQL schema 与数据职责
> 最后更新：2026-08-31

## 1. 目标与边界

MySQL 保存账号、审计、结算和资产事实，是当前必须备份的数据源。Redis 保存在线、匹配和排行榜投影；当前没有正式 Redis 恢复承诺，且多数状态可过期或从 MySQL/客户端重建。

本项目尚未定义业务 RPO/RTO，没有异地副本、自动备份、自动故障转移或跨区域灾备。以下命令是本地/测试演练，不是生产方案。

## 2. 文件规则

- 备份放在仓库外，例如当前用户目录下的 `game-realtime-backups`。
- 文件名记录环境、UTC/本地时间和代码版本。
- `.sql`、`.sql.gz`、`.dump`、RDB/AOF、日志和 profile 不提交 Git。
- 分享前检查账号、IP、User-Agent、审计 detail 等个人/敏感数据并脱敏。
- 备份应有访问权限控制和校验哈希；当前未实现自动加密与保留清理。

## 3. MySQL 逻辑备份

创建仓库外目录：

```powershell
$backupDir = Join-Path $env:USERPROFILE "game-realtime-backups"
New-Item -ItemType Directory -Force -Path $backupDir | Out-Null
$stamp = Get-Date -Format "yyyyMMdd-HHmmss"
$backup = Join-Path $backupDir "game_realtime-$stamp.sql"
```

导出一致性逻辑备份：

```powershell
docker exec game_realtime_mysql sh -lc `
  'mysqldump -u"$MYSQL_USER" -p"$MYSQL_PASSWORD" --single-transaction --routines --triggers "$MYSQL_DATABASE"' |
  Set-Content -LiteralPath $backup -Encoding utf8
```

`--single-transaction` 适用于当前 InnoDB 表，减少导出期间锁表影响。MySQL 可能输出命令行密码警告；不要记录真实凭据或公开终端全文。

校验文件：

```powershell
Get-Item -LiteralPath $backup | Select-Object FullName,Length,LastWriteTime
Get-FileHash -LiteralPath $backup -Algorithm SHA256
Select-String -LiteralPath $backup -Pattern "CREATE TABLE.*players","CREATE TABLE.*mission_records"
```

零字节、缺少关键表定义或命令非零退出都视为备份失败。

## 4. MySQL 恢复演练

恢复应优先在隔离的新测试数据库/新 volume 进行，不直接覆盖唯一工作副本。

1. 记录备份 SHA256、源代码版本和 MySQL 版本。
2. 启动隔离 MySQL 或创建明确的测试数据库。
3. 导入 dump：

```powershell
Get-Content -LiteralPath $backup -Raw |
  docker exec -i game_realtime_mysql sh -lc 'mysql -u"$MYSQL_USER" -p"$MYSQL_PASSWORD" "$MYSQL_DATABASE"'
```

4. 验证表、行数和关键一致性：

```sql
SHOW TABLES;
SELECT COUNT(*) FROM players;
SELECT COUNT(*) FROM mission_records;
SELECT COUNT(*) FROM reward_records;
SELECT COUNT(*) FROM asset_ledger;

SELECT COUNT(*) AS players_without_assets
FROM players p
LEFT JOIN player_assets a ON a.player_id = p.id
WHERE a.player_id IS NULL;

SELECT COUNT(*) AS ledger_balance_mismatches
FROM asset_ledger
WHERE balance_after <> balance_before + delta;
```

5. 启动 Go 服务并验证登录、玩家资产、GM 结算查询和排行榜边界。
6. 记录恢复开始/结束时间、失败步骤、结果和局限。

当前没有自动 migration version 表，恢复后执行增量 SQL 前必须人工确认 dump 对应结构版本。

## 5. Redis 持久化边界

Compose 使用 Redis 官方镜像与命名 volume，但没有显式配置 AOF、RDB 周期、复制或 Sentinel。volume 不是独立备份；容器/磁盘损坏仍可能丢失数据。

当前 Redis 数据处理：

| 数据 | 丢失后的业务处理 |
| --- | --- |
| online key | 玩家 HTTP/WS 心跳重新建立 |
| matchmaking ticket/queue | 当前原型允许丢失，客户端重新排队 |
| leaderboard | 理论上可从 MySQL settled 记录重建，但当前没有自动工具 |

如需做本地 RDB 实验：

```powershell
docker exec game_realtime_redis redis-cli SAVE
$redisBackup = Join-Path $backupDir "redis-dump.rdb"
docker cp game_realtime_redis:/data/dump.rdb $redisBackup
Get-FileHash -LiteralPath $redisBackup -Algorithm SHA256
```

`SAVE` 会同步阻塞 Redis，只适合小型本地演练。不要把这个命令当作生产备份策略。RDB 恢复应在隔离容器/volume 中验证，不能在运行中的唯一 Redis 上直接覆盖 `/data`。

## 6. 恢复优先级

1. 先恢复 MySQL 事实与资产数据。
2. 校验玩家、结算、奖励、余额和流水一致性。
3. 启动 Go 服务，允许在线和匹配状态自然重建。
4. 对账或重建排行榜；在工具未实现前，明确榜单可能为空/滞后。
5. 再执行接口和 WebSocket 冒烟。

不要为了恢复 Redis 排行直接修改 MySQL 资产事实，也不要从客户端提供的分数重建可信结果。

## 7. 演练记录模板

```text
环境：
代码版本：
MySQL/Redis 版本：
备份文件与 SHA256：
备份开始/结束：
恢复开始/结束：
数据量：
执行命令：
一致性查询结果：
接口冒烟结果：
失败与修复：
实际 RPO/RTO 观察值（仅本次演练）：
局限与后续动作：
```

单次观察值不是正式 RPO/RTO 承诺。
