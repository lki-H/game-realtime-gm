# M7 发布与版本基线

> 权威级别：L2（发布与远程验收证据）
> 日期：2026-10-11，Asia/Shanghai
> 状态：已合并 main，PR 与合并后 CI 全部通过

## 发布范围

以 R5 main `bfd6f284bc04d91f5354ec5382f35c24e0c91dfd` 为基线，在独立干净工作区使用 `m7-release`。发布 M7 就绪探针、监听与连接池预算、真实 HTTP/匹配竞态回归，以及后续七项社交、邀请、启动和 Unity 缺陷修复；同步公开技术文档、验收与性能证据。

源工作区及既有未提交材料保留。白名单不包含私人交接、学习历史、日志、凭据、备份、缓存、Unity 二进制或本机路径。编号 SQL 未改，不迁移开发数据，不部署公网服务，不调整 VPN、代理或防火墙。

## 验证依据

| 检查 | 证据与边界 |
| --- | --- |
| M7 本机综合验收 | [退出矩阵](m7-acceptance.md)：真实身份、连接、状态机、结算、恢复、实际 R3 进程回退与独立 RPC/MQ |
| 持续运行 | [性能报告](../performance/m7-local-soak.md)：固定构建 598 局/1801.71 秒，失败与逐局资产差异均 0；四条连接 1203.233 秒 |
| 追加修复 | [后续复核](post-m7-audit-20261010.md)：92 项真实存储 Windows/Linux race，失败与 SKIP 均 0，新 Unity 混排与场景保存通过 |
| 发布副本检查 | Go test/vet/mod verify、R5 Go/C#/mod verify、双 React 构建与官方 registry audit、脚本/链接/公开边界检查通过 |
| PR 精确 head | [PR #4](https://github.com/lki-H/game-realtime-gm/pull/4)，head `b02a8845518d0aecf99b658e27e0038550d49d54`；Verify local PVE 与 Verify independent R5 均 success |
| 合并 main | merge commit `905a08556678980af7fa7cb4bad2d1396d0f7e03`；[Verify local PVE](https://github.com/lki-H/game-realtime-gm/actions/runs/38068246794) 与 [Verify independent R5](https://github.com/lki-H/game-realtime-gm/actions/runs/38068246845) 均 completed/success |

长运行数字属于性能报告所列固定构建。后续预算和缺陷修复由专项、完整回归与短时循环验证，未将它们记作再次运行三十分钟。

## 运行与回退

[运行指南](../pve-release-guide.md)、[部署手册](../deployment-runbook.md) 与 [恢复说明](../backup-and-recovery.md) 提供操作入口。保留 R3 兼容回退 SHA `64a4263648849586dfad9013d49a664f76181e55` 和 [R4回滚约束](../design/v2-r4-retirement-and-rollback-plan.md)；不能用旧备份覆盖已有新奖励的活动库。

这是源码、契约与验证证据发布。二期完成范围为单实例本机闭环；跨机器验收延期。真实战斗服、生产身份与部署、多实例、HA、容量上限及 OTel 继续独立规划。

## 最终版本

PR #4 于 2026-10-11（Asia/Shanghai）合并。主项目合并后 CI 的真实后端集成/race、双前端构建和服务镜像构建均通过；R5 合并后真实 MySQL/RabbitMQ race、C# 契约和主资产隔离也通过。CI 本地镜像 ID 为 `sha256:9a8cae397af6729a8b55f803e1ab0291e6720732e0ee619f01499c16b22cfda5`，未推送镜像仓库或部署服务。
