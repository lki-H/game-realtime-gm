# R5 公开发布与版本基线

> 权威级别：L2（版本与远程验收证据）
> 日期：2026-10-10，Asia/Shanghai
> 状态：代码、本机验收与首轮 PR CI 通过；最终合并状态见 PR

## 发布范围

基于 R4 main `834d7261184a5f970651246f298dcde61f472c39`，在独立干净工作区发布 `r5-release`，代码提交 `1157af7f40a4a02153e1c92932c0f44130a177a9`，PR 为 [#3](https://github.com/lki-H/game-realtime-gm/pull/3)。精确白名单包含 R5 独立模块、八类复核修复、生成契约、CI、公开技术文档和去敏验收记录；不包含私有交接、学习历史、日志、备份、凭据、缓存和本机路径。

主资产事务和 HTTP/WS 控制面仍由单体负责。没有迁移开发数据、修改已应用 day37—40 SQL、自动部署公网或上传 Unity 二进制；旧历史与 R3 兼容回滚基线保留。

## 验证记录

| 检查 | 证据 | 范围 |
| --- | --- | --- |
| 发布副本本地检查 | Go/backend 与独立模块 test/vet、模块校验、双前端 npm ci/build/生产 audit、C# 编译、Shell/PowerShell、OpenAPI/链接/敏感扫描通过 | 无依赖 Integration SKIP 不作为真实集成证据 |
| R5 本机完整闭环 | [R5验收](r5-m6-acceptance.md)、[全面复核](20261010-full-review.md) | 真实存储、进程崩溃/重启、重建、隔离及 Windows 构建 |
| R5 首轮 PR CI | [run 38055231551](https://github.com/lki-H/game-realtime-gm/actions/runs/38055231551)，completed/success | Go 契约/race/vet、C#，独立 MySQL/RabbitMQ 恢复副本、最小权限、真实 race 与 broker 停止后主资产对账 |
| 主项目首轮 PR CI | [run 38055231545](https://github.com/lki-H/game-realtime-gm/actions/runs/38055231545)，completed/success | 后端普通验证、真实 MySQL/Redis race、固定 R3 二进制实际回滚、双前端构建及服务镜像构建 |

首轮 CI 属于代码提交 `1157af7`；发布文档补记提交及合并 main 必须重新检查 Actions。最终提交与合并状态以 [PR #3](https://github.com/lki-H/game-realtime-gm/pull/3) 和 [main 的 Actions](https://github.com/lki-H/game-realtime-gm/actions?query=branch%3Amain) 为准，不把首轮 CI 等同于后续提交已通过。

## 运行与回退

R5 模块、固定版本、账号/视图边界与启停命令见 [模块说明](../../experiments/r5-m6/README.md)。Linux/CI 的 `deploy/verify-ci.sh` 在临时独立资源上产生合成结算、恢复副本、真实 MQ/SQL 测试；Windows `deploy/verify.ps1` 另验证多进程崩溃、重启和回退。结束清理专用容器、网络、卷与凭据，不改其他项目资源。

停止四个实验角色即可回退实验，主单体不需增加 RPC/MQ 配置。主玩法回滚保留 [R4回滚约束](../design/v2-r4-retirement-and-rollback-plan.md) 和已审核 R3 SHA `64a4263648849586dfad9013d49a664f76181e55`，不得用旧备份覆盖活动资产。

跨机器、M7 长时运行、真实战斗服、DS、多实例、HA、生产传输保护和商业容量仍独立延期。本次是源码和验证证据发布，不是生产上线。
