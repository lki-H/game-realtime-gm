# 二期成果与收口边界

> 文档角色：二期交付、证据导航和后续边界
> 权威级别：L2（阶段总结）
> 状态：二期本机范围完成，跨机器延期；M7发布状态见版本基线
> 适用范围：单实例、本机学习与可演示作品
> 事实来源：当前源码、编号迁移、R3—R5发布和M7执行证据
> 最后更新：2026-10-10

## 交付

主系统保持 Go 模块化单体：Gin HTTP、WebSocket、MySQL 事实和资产事务、Redis 可重建投影、同进程可靠 Worker。Party 房主只管理局外；整组票据、确认、加载和 Run 权威分开。个人任务可选、可跨局累计并按目标共享，共同目标服务端统一结束，逐人结算与业务键防重。

工程能力包括编号迁移/checksum、会话撤销与管理员复核、HTTP/WS 预算、连接代次与背压、版本化规则快照、可信测试事件水位/去重、逐人 pending/retry/needs_repair、审计修复、Redis 重建、维护排空、归档和保留发奖依据。M7 补齐依赖就绪探针和可限制回环的主监听。

React GM 提供六类 V2 观察与 operator 修复；PVE 网页和 Unity Windows 验证真实控制面。R5 是独立 Go 模块：Protobuf Go/C# 契约、gRPC 只读恢复副本、RabbitMQ 发布确认/事务后 ACK/有限重试/死信/幂等报表。主链路继续使用 MySQL Worker，不依赖实验消息链发奖。

## 验收导航

- [M7 综合验收](testing/m7-acceptance.md)：二期十五项退出门槛与 PASS/限制。
- [M7 持续运行报告](performance/m7-local-soak.md)：实际三十分钟负载、整局延迟、资源与对账。
- [R5 发布基线](testing/r5-release-acceptance.md)：精确 SHA、PR、main 两套真实 CI，保留旧 R3 回退基线。
- [RPC/MQ 实验](testing/r5-m6-acceptance.md)：独立权限、崩溃、重投、下游恢复和主资产隔离。
- [当前演示](demo-plan.md)、[部署](deployment-runbook.md)、[恢复](backup-and-recovery.md) 和 [测试](test-plan.md)：可操作入口。

## 如何展示

按“好友双排＋两位单排 → 四人同一 Run → 不同/无个人任务 → 原成员重连 → 服务机 Bot 完成共同目标 → 逐人结算 → 重复事件与账本不变”演示。GM 同时观察；另演一次撤销旧会话或 needs_repair 后受控重试。说明 MySQL 是事实、Redis 是投影、网络通知可以丢失但 HTTP 查询能恢复。

能证明的是 Go API、状态机、事务、幂等、可靠后台任务、真实依赖/并发/race、客户端契约、CI 和受控恢复能力。有限本机数据不代表商业容量或生产经历；测试 Bot 不是真实战斗服。

## 明确保留

跨机器已由用户延期，应使用既有两台真实电脑指南另验；不得默认开启 VPN、端口或公网部署。Unity Editor 场景交互和跨 PC 与 Windows Player 批处理验证分别记录。

OTel、共享部署强 Secret 准入/轮换、短期握手凭据、完整 GM 管理 UI、弱网与容量上限、旧学习包彻底删除，在对应部署/产品需求到来时单独规划。真实 DS、调度、多实例/HA和商业化属于后续阶段，不是本机二期门槛。

R5 已发布；M7 新增补丁与文档的精确提交、PR 和远程检查见 [M7发布基线](testing/m7-release-acceptance.md)。本机验收与远程发布分别记录，私人材料保留在原工作区。

M7后完整复核修复七项社交/邀请/启动/Unity验收与构建缺陷；当前代码以 [最新复核](testing/post-m7-audit-20261010.md) 的92项真实存储/race和新Player验证为准，M7长运行记录保留为原固定构建证据。
