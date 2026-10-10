# R5/M6 独立 RPC 与消息实验计划

> 文档角色：R5 独立实验的范围、顺序、边界和验收
> 权威级别：L2
> 状态：R5/M6 独立业务实验已实现并通过本机验收
> 最后更新：2026-10-10

R4已有本机实现与验证记录。跨机器验收和共享环境观察窗口继续延期。R5在独立 `experiments/r5-m6` 模块中实现并通过真实本机验收；主资产事务和现有HTTP/WS继续由原单体负责，配套可靠性修复见 [全面复核](../testing/20261010-full-review.md)。操作见 [独立模块](../../experiments/r5-m6/README.md)，结果见 [R5验收](../testing/r5-m6-acceptance.md)。

## 1. 实验选择

第一项选择只读 gRPC 服务，查询排行榜/战绩或已结算 Run 结果。它不接收玩家动作，不创建 Run，不写 `player_assets`、`asset_ledger`、`pve_runs` 或 `pve_run_events`。

第二项选择 RabbitMQ 消费实验，链路固定为：

```text
V2 MySQL 事务 outbox -> 实验发布者 -> RabbitMQ -> 幂等报表消费者
```

消费者只写独立实验投影表，不能把消息确认、重投或死信当成发奖依据。主 Worker 仍负责结算和资产事务；实验失败时可以停掉发布者和消费者，主链路继续工作。现有 outbox 已由主 Worker 消费，实验必须使用只读桥接和独立投递状态，不能争抢、修改主 Worker 的 pending/outbox 消费状态。

## 2. 当前前置检查

| 项目 | 当前状态 | 处理 |
| --- | --- | --- |
| Go 编译链 | 本机 1.25.6 已有，生成器和夹具验证通过 | 继续沿用，不批量升级主后端依赖 |
| `protoc` 与 Go 生成器 | 36.2 / 1.36.12 / 1.6.2 已安装 | D 盘工具目录，固定来源和校验值 |
| Buf | 未安装 | 可选，不作为必需前置；优先使用固定 `protoc` 命令 |
| RabbitMQ | 4.3.6 已通过真实消息、重投、持久重启与权限测试 | 独立 Compose；检查资源已清理，不常驻运行 |
| Docker Desktop | 环境检查时短暂开启，结束后关闭 | 启动脚本按需开启，保留原开发数据和配置 |
| 项目主链路 | R4 已通过 | 不添加 RPC/MQ 依赖到主请求路径 |

本机环境和独立实验均已落地；Buf、原生 Windows RabbitMQ/Erlang 服务、Kubernetes、额外 Unity 模块均非首轮必需。没有改变日常 VPN、全局 PATH 或开发数据库。只读恢复副本、有限 SQL 视图、独立服务身份及按 exchange/queue 授权的 MQ 账号已由初始化工具建立并验证，不能据此推广为生产身份系统。

## 3. 实施顺序

### R5-0：契约与夹具

- 新建独立 `experiments/r5-m6` 模块或明确的实验目录，不放入 `backend/internal/pve` 主领域包。
- 固定 `.proto` 的包名、服务名、字段编号、错误码和 deadline 规则。
- 准备不含真实凭据的排行榜/Run 结果 fixture。
- 定义实验投影表和消息 envelope：`message_id`、`schema_version`、`source`、`published_at`、`payload_hash`。

### R5-1：gRPC 只读服务

- 只提供查询 RPC，例如 `GetLeaderboard` 和 `GetRunResult`。
- 必须验证服务身份、deadline、请求上限、字段兼容和查询权限。
- 下游不可用、超时、返回未知字段和客户端取消都要有可观察结果。
- HTTP/WS 主服务不改成 gRPC 客户端；实验服务可以读取恢复副本或只读数据库账号。

### R5-2：RabbitMQ 投影实验

- 发布者只读取明确类型的 outbox 记录，发送 publisher confirm 后记录发布状态。
- 消费者手动 ACK；业务成功后再 ACK，失败进入有限重试和死信队列。
- `message_id` 和 payload hash 建唯一约束，重复投递不重复写投影。
- 消费者重启、ACK 前崩溃、RabbitMQ 重启、积压和死信都要有测试。

### R5-3：隔离故障验证

- 停止 gRPC 服务时，主 V2 查询和结算仍可用。
- 停止 RabbitMQ 或消费者时，资产、奖励、Run 事实不受影响。
- 重复消费、乱序报表和过期消息不能改动主事实。
- 删除实验 Redis/数据库投影后可以从 fixture/outbox 重建。

## 4. 禁止事项

- 不把 gRPC 放到玩家控制面或实时战斗同步链路。
- 不用 RabbitMQ 替代 MySQL 结算事务、活动锁或事件序号。
- 不让消费者直接修改余额、奖励、Run 终态或任务完成事实。
- 不接入公网，不使用生产凭据，不把实验 RabbitMQ 与现有 Redis/MySQL 共用清理命令。
- 不以实验通过宣称已经完成多实例、真实战斗服、Unity Dedicated Server 或商业容量。

## 5. 完成门槛

实验完成需要同时具备：

1. 固定 `.proto` 与兼容性检查。
2. gRPC deadline、身份、下游重启和只读权限测试。
3. RabbitMQ publisher confirm、手动 ACK、重试、死信和重复消费测试。
4. 主链路与实验链路故障隔离证明。
5. 独立启动/停止/清理命令、日志和指标。
6. 不改主资产事务的回退演练。

R5 实验完成不代表项目整体结束；真实游戏服、Unity DS、多实例和跨机器共享验收仍需独立门槛。

## 6. 2026-10-10 实施结果

R5-0—R5-3与以上六项门槛均通过本机验收：固定Go/C#契约、真实只读查询、独立outbox桥接、confirm/ACK/有限重试/死信/重复冲突、实际消费者提交后崩溃、MQ/数据库重启、晚提交与投影重建、主四人控制面/结算隔离、启停回退及真实存储Linux race。CI另提供契约/单元和真实MySQL/RabbitMQ集成两个作业，发布结论以实际run为准。DS、来源生产化、调度、多实例另按需求推进。
