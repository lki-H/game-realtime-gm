# 项目简介

> 文档角色：一页式项目说明
> 权威级别：L1（当前项目定位摘要）
> 状态：一期已完成，持续维护
> 适用范围：协作者、面试官与首次阅读者
> 事实来源：当前实现、架构、契约与一期验收记录
> 最后更新：2026-10-11

## 项目目标

本项目通过一个模块化单体 Go 服务，练习真实游戏业务后台中常见的身份、在线、小队、任务会话、匹配、结算、资产、排行和 GM 观察链路。重点是业务边界、数据一致性、并发安全和可验证证据，而不是堆叠分布式组件。

## 使用者

| 角色 | 当前交互方式 | 目标 |
| --- | --- | --- |
| 玩家 | HTTP API、WebSocket 测试客户端 | 注册登录、在线、小队、任务、匹配、结算、排行 |
| GM/管理员 | 管理员 HTTP API | 玩家管理、操作审计、实时观察与结算查询 |
| 开发者 | Go CLI、Docker Compose、MySQL/Redis CLI、`ws_bot` | 启动、验证、诊断和复现实验 |

React GM 观察窗和 Unity/C# PVE 控制面已作为 V2/R3 展示层加入；它们不改变一期 legacy 能力边界，跨机器和真实战斗服仍延期。

## 当前组成

- Go 服务：HTTP、WebSocket、业务模块和诊断入口。
- MySQL 8.4：长期业务事实、结算与资产事务。
- Redis 7：在线 TTL、匹配状态和排行榜投影。
- Docker Compose：本地 MySQL/Redis 环境。
- 文档与证据：OpenAPI、WebSocket 契约、测试计划、性能基线和运维手册。

## V2 合作 PVE 扩展

- `internal/pve` 提供好友房间、招募、本局来源票据、候选逐人确认、Run、共同目标、可选个人任务、可信测试事件和逐人结算。
- React GM 观察窗和 Unity Windows Player 已完成本机验证；测试事件仍由独立 loopback Bot 提交，不代表真实战斗服。
- 默认已切换为 `GAMEPLAY_MODE=v2`；旧一期回归必须显式设置 `GAMEPLAY_MODE=legacy`，两种模式不在同一实例同时写同一套玩法资产。

## 一期成果

Day27-Day35 完成了小队广播、任务会话状态机、Redis 匹配 ticket、结算记录、资产强事务、排行榜、GM 观察、race/pprof/`ws_bot` 和文档收口。详细证据见 [一期成果与证据](phase1-summary.md)。

## 明确边界

- 单实例、单 Go 进程，不是微服务。
- 小队和任务会话保存在内存，服务重启后清空。
- 匹配只实现 ticket、排队、取消和超时，没有组队撮合成功流程。
- MySQL 是资产与结算事实源；Redis 排行榜是可重建投影。
- `mission_instance` 是业务会话，不是 Dedicated Server 或逐帧战斗模拟。
- 没有正式前端、云部署、CI/CD、Kubernetes、Zinx 或生产 SLO。

## 阅读顺序

1. [需求规格](requirements-spec.md)
2. [当前系统架构](architecture.md)
3. [系统详细设计](system-design.md)
4. [HTTP API](api-overview.md) 与 [WebSocket 协议](ws-protocol.md)
5. [数据库设计](database-design.md)
6. [测试计划](test-plan.md) 与 [部署手册](deployment-runbook.md)
