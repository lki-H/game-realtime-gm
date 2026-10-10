# R5/M6 独立 RPC 与消息实验验收

> 文档角色：实现、真实检查、回退和未覆盖范围
> 权威级别：L2（本机验收记录）
> 日期：2026-10-10，Asia/Shanghai
> 状态：R5-0—R5-3 本机验收与首轮远程 PR CI 通过，发布版本见单独记录

## 1. 交付范围

本次按照用户“独立把 R5 全部完成”的授权实施已确认的原 M6 独立实验，而不是同时接入真实战斗服或改造主服务为微服务。交付在 `experiments/r5-m6` 独立 Go 模块；主后端业务源码、go.mod/go.sum、day37—40、开发数据库和玩法模式没有修改。

核心交付：固定 Protobuf 契约与 Go/C#生成物、只读结果/奖励榜 RPC、独立 outbox 桥接、RabbitMQ可靠报表、最小权限初始化、独立查询客户端、认证指标、日志、重启/清理脚本、单元与真实集成/race。操作入口见 [模块说明](../../experiments/r5-m6/README.md)，安全边界见 [威胁模型](../design/r5-m6-threat-model.md)。

## 2. 真实环境与证据

- Go：Windows本机1.25.6；Linux race 使用已有官方 `golang:1.27.1-bookworm`。
- MySQL 8.4.11、Redis 7、RabbitMQ 4.3.6-management 固定摘要，独立 `gm-r5-verify` Compose，均只绑定本机。
- 主服务在隔离容器跑真实HTTP/WS/内部事件；四名合成玩家真实匹配、重连、共同目标和自动结算。R5读取其一致性备份恢复副本，没有读取开发库或使用真实玩家资料。
- 恢复库 `game_realtime_v2_r5_restore`；报表库 `game_realtime_r5_reports`；Reader 仅SELECT四个有限视图，Projector仅独立库CRUD；MQ publisher/consumer分别最小权限。
- 原始验收证据和合成备份保留在仓库外受限目录。合成备份的校验值记录在私有证据中，不公开提交完整日志或凭据。
- 最终真实Go测试5.319秒，包含新增晚提交回查及全报表重建；真实存储Linux race 5.811秒，退出0，无race报告。Windows主后端普通测试/vet、独立模块test/vet/mod verify均通过。

## 3. 验收矩阵

| 门槛 | 实际检查 | 结果 |
| --- | --- | --- |
| R5-0 契约 | 固定包/字段编号、类型、嵌套消息、RPC输入输出和非streaming；source hash、描述符/Go同步；未知字段保留；再生成检查 | PASS |
| Go/C#夹具 | 实际同一Reporting Proto生成、编译与消息序列化；C# .NET10控制台 | PASS；不代表Unity RPC接入 |
| RPC身份与授权 | 无身份/错误token/无scope拒绝；player白名单与实际Run参与关系；榜单/包体/并发有界 | PASS |
| Deadline与错误 | 缺失/超长deadline拒绝；超时/取消、下游不可用有明确gRPC code；RPC重启后独立客户端恢复 | PASS |
| 只读数据库 | Reader和Projector尝试写原余额/Run/outbox、读原players均被数据库权限拒绝 | PASS |
| MQ最小权限 | publisher/consumer无configure；publisher不能consume；consumer不能发布源events | PASS |
| 来源桥接 | 读取真实主settled Run outbox；主published状态不变；回查捕获低ID晚提交，独立水位/投递去重 | PASS |
| Confirm | 持久消息confirm成功后标published；mandatory无路由拒绝；失败3次进needs_repair | PASS |
| ACK与重复 | 事务提交后ACK；重复/并发只生成一个报表；同ID不同payload/完整fingerprint冲突进死信 | PASS |
| 有限重试/死信 | transient3次、dead后停止；报表库失联时MQ header预算仍有限；非法/过期消息死信，确认转投前不ACK | PASS |
| 乱序与重建 | 不同Run乱序、同Run别名消息不重复；清独立投影后从来源恢复投递与完整报表 | PASS |
| ACK前实际进程崩溃 | consumer先commit后测试专用故障exit42；重启自动重投，duplicates指标增加，不重复报告 | PASS |
| Broker持久重启 | 停consumer产生积压，RabbitMQ停止/重启后confirmed消息仍保留，consumer恢复 | PASS |
| SQL重启 | 数据库停止时RPC拒绝虚构数据；数据库重启后连接池恢复 | PASS |
| 主链路隔离 | 实验运行、RPC停、consumer停、broker停、实验全停六种状态，主四人结算继续，资产对账每轮差异0 | PASS |
| 独立启动/回退 | 主服务停时RPC仍可查恢复副本；手动start/stop保留卷再启动查询成功；最后reset不影响开发卷 | PASS |
| 观察与日志 | RPC状态/耗时，消息/Run/hash，publisher失败、retry/dead/duplicate/worker指标；指标需独立身份 | PASS |
| Race | 真实MySQL/RabbitMQ Linux race含并发去重、RPC和恢复；测试代理仅 `_test.go` | PASS |

## 4. 复核中修复的边界

1. 将payload先解析并规范化再校验hash，避免MySQL JSON重排空白造成假冲突；同ID同时检查完整envelope fingerprint。
2. 主outbox原状态由原Worker消费，Bridge使用独立状态并回查低ID晚提交，避免只按自增高水位漏事件。
3. 报表库失联时不依赖它保存重试次数，MQ header作为持久预算，最终死信；转投须确认再ACK。
4. 使用quorum和至少一次TTL死信，检查mandatory return，不能把无路由broker confirm当投递成功。
5. 发布者超过重试预算进入needs_repair，报表故障不能回写主资产或Run。
6. PowerShell JSON自动把ISO日期解析成DateTime，原停止身份字符串比较误拒绝；修复为规范路径/UTC ticks，并实际启停重启通过。
7. 固定Proto输入输出/嵌套类型/source hash和描述符，不只检查方法名称或字段数字；误改契约须先重新生成和验证。
8. Docker末轮清理复核发现Redis镜像在保留环境重启时产生匿名数据卷；根据本轮创建时间、无容器引用及仅dump.rdb内容确认后精确删除，Compose补专用命名Redis卷，后续down -v一起清理，不使用全局prune。

## 5. 清理与发布边界

代码提交、远程CI及最终版本追踪见 [R5发布基线](r5-release-acceptance.md)；本机、PR和合并main证据分别记录。

实验进程、容器、网络、MySQL/RabbitMQ卷和临时凭据已清理，Docker Desktop恢复关闭。合成备份、二进制和去敏日志保留在仓库外受限目录；安装工具与官方镜像供下次使用。原开发/其他项目卷、备份、日常VPN、防火墙和既有未提交修改保留。

`.github/workflows/verify-r5.yml` 提供独立契约/单元race/vet/模块校验、C#构建，以及单独的真实MySQL/RabbitMQ集成race和主链路隔离作业。真实作业显式创建恢复副本和最小权限账号；Windows完整进程验收另由 `deploy/verify.ps1` 提供。

## 6. R5完成的含义

后续全面复核修复 reports quorum 投递超限的 DLX 保护，以及已 applied/fingerprint 匹配的过期重投去重；新增真实 MQ 专项与完整脚本/race 通过。原始日志、构建产物和备份保留在仓库外；最新修复见 [全面复核](20261010-full-review.md)。

完成的是本机 R5/M6 独立实验六项门槛。跨机器与长期共享观察继续按此前决定延期；真实可信战斗服、Unity DS/UE DS、生产身份/mTLS、调度、HA、多实例、商业容量和彻底删除旧学习包继续独立规划。没有将它们提前标记完成，也不需要为完成这次RPC/MQ实验附带实现它们。
