# ADR 0012：独立只读 RPC/MQ 实验，不拆主资产事务

> 状态：Accepted（本机实验范围）
> 日期：2026-10-10
> 前置：R0—R4可靠单体与本机客户端已有验收，R5/M6已通过真实隔离验证

## 背景

需要训练跨进程契约、服务身份、deadline、可靠投递和幂等报表。当前主Worker已消费pending/outbox，奖励和余额必须继续由MySQL事务保证，不能引入另一个消费者争抢主状态，或让MQ消息成为发奖依据。

## 决策

建立独立 `experiments/r5-m6` Go模块，使用Protobuf/gRPC提供已settled Run与奖励榜只读服务，使用RabbitMQ提供独立报表投递。运行数据来自合成备份恢复副本和有限只读视图；Reader仅SELECT，Projector只写独立数据库。Bridge只读主outbox并记录独立扫描/投递状态；Publisher确认后标published，Consumer事务提交后ACK，有限重试和死信不回写主资产。

主模块不依赖gRPC/MQ，HTTP/WS和原资产/Run状态机保持原样。停止独立角色与专用依赖即可回退。所有端口仅本机；固定契约、版本、权限、重启和故障隔离由可重复脚本验证。

## 取舍

保留重复扫描、独立投递/回执存储和多进程启停的实验成本，换取可解释的服务与消息边界。不采用MQ重写结算、不拆主数据库、不增加服务发现/Kubernetes，也不把单节点quorum队列称为高可用。

完整验收和发布状态见 [R5证据](../testing/r5-m6-acceptance.md)，运行说明见 [独立模块](../../experiments/r5-m6/README.md)，威胁边界见 [安全模型](../design/r5-m6-threat-model.md)。本机结果与远程CI分别记录。

## 后续触发条件

真实业务出现独立扩缩容、发布、团队所有权或共享环境需求后，再决定是否接入生产数据/身份并建立TLS/mTLS、容量和服务生命周期。引擎/DS、调度、多实例继续独立决策，不替代既有Unity/UE/C++长期ADR。
