# M7 本机综合验收

> 文档角色：二期退出标准、执行证据与限制
> 权威级别：L2（日期化验收记录）
> 状态：M7本机范围PASS；二期本机范围完成，跨机器延期
> 适用范围：单实例、独立测试库、本机 React 与 Unity Windows
> 事实来源：源代码、真实 HTTP/WS、MySQL/Redis、Linux race 与实际进程
> 最后更新：2026-10-10

## 版本与证据边界

R5 基线为 main `bfd6f284bc04d91f5354ec5382f35c24e0c91dfd`（树等同 `c5d60ef`）；对应历史 main CI 为 38055734806、38055734817。M7 和后续修复已通过 PR #4 发布；精确 head、合并 main 与两套新 CI 见 [M7发布基线](m7-release-acceptance.md)。本机验收使用当时的磁盘源码，不用源分支历史 HEAD 或 R5 CI 证明新增实现。

本机私有证据、版本快照、日志、资源样本、截图与合成备份仅在仓库外受限保留。本文只写去敏结果。跨机器明确延期，OTel 后置；真实战斗服、生产、多实例和商业容量不纳入本轮。

## 执行顺序与隔离

1. 独立 Compose `gm-m7-verify`，MySQL 127.0.0.1:23306/`game_realtime_v2_test`，Redis 127.0.0.1:26379。迁移沿用 day37—40，保持字节不变。
2. 主服务仅 127.0.0.1:8080；事件 8090、指标 8091、pprof 6060 仅回环，凭据独立随机。合成账号密码通过环境传递，临时保存为当前 Windows 用户 DPAPI 加密文件，目录 ACL 限当前用户/SYSTEM。
3. `pve_verify -mode prepare` 后执行 `-mode soak -duration 30m`；每轮四人登录、确认、重连、测试事件、结算和三项资产对账，每 30 秒采集资源。同时执行独立账号的 Unity 普通/混排、网页操作及四条 20 分钟持续连接。
4. 持续运行结束后停止写入，再执行故障恢复、维护排空、停服备份、恢复到新库并比较；清空专用 Redis 投影后重启恢复。
5. 停止演示服务再跑全量存储测试与当前/R3 真实二进制回滚。Linux 存储 race 使用同一独立依赖及明确容器地址；不让清表测试与 soak 并行。
6. Linux 主进程 SIGTERM、就绪失联/恢复和数据库所有权释放；独立 R5 RPC/MQ 完整重验。最后清理本轮进程、容器、网络、卷和临时凭据。

## 二期退出矩阵

| 门槛 | 验收入口 | 当前结果 |
| --- | --- | --- |
| B01—B10 | legacy 回归卡、V2 规则/生命周期、默认 V2 拒绝旧写、数据库排他所有权 | PASS/限用：B08共享管理并发实测；legacy其他缺陷仍隔离限用 |
| 真实身份/权限 | TestM7HTTPIdentityAndConcurrentAdminTransitions、FourWebSocketsWithInternalHTTPAndResultRecovery | PASS：真实注册/登录/并发封禁/解封/旧HTTP及WS拒绝/新登录 |
| 长连接/竞态 | TestM7CancellationAndConfirmationDeadlineRaces、连接代次/配额/背压回归、30 分钟 soak/20 分钟 hold | PASS：598局零失败，四条持有连接提前断开0 |
| 可靠结算 | IndividualSettlementCanRetryWithoutRepeatingOtherRewards、Worker 重启与 operator 重试 | PASS：真实needs_repair、恢复资产、相同修复请求重放与进程回退 |
| 幂等与账本 | 每轮三项对账、奖励回归、R5 重投和 ACK 前崩溃 | PASS：每轮及故障恢复差异0；RPC/MQ重投与提交后崩溃通过 |
| 来源边界 | InternalEventAuthAndRevokedSessions、旧来源/重复/冲突/缺口；默认关闭的 loopback 事件 | PASS；Bot仍仅测试身份，不宣称真实可信战斗服 |
| 自动客户端闭环 | 真 Unity 普通四人、好友双排混排、pve_verify | 两种 Unity 本轮 PASS |
| Redis/Worker 恢复 | WorkerRestartAssetsAndRedisRebuild、R3 进程恢复与 outbox 异常隔离 | PASS：Redis清空恢复12人奖励，真实Worker重试与outbox隔离 |
| 生命周期/就绪/预算 | ReadinessDependencyFailureAndShutdown、Linux SIGTERM、连接限额/包体/命令限流 | PASS：Redis故障503/活性200，恢复200；SIGTERM退出0/WS关闭/锁释放；硬预算并发验证 |
| 观察与日志关联 | request_id、operation_id、run_id；受限指标；失败重试日志与审计 | PASS：repair-correlation与Worker故障日志可定位；OTel后置 |
| React/Unity | GM 六类观察、玩家预览/建房/方案/准备/招募；真实四 Player | 本轮浏览器与 Player PASS |
| RPC/MQ | experiments/r5-m6/deploy/verify.ps1，独立恢复副本和最小权限 | PASS：本轮完整恢复/权限/confirm/ACK/崩溃/死信/重启/主资产隔离及真实race |
| CI/备份/回滚 | M7合并main两套CI；隔离恢复；当前/R3二进制 | PASS：53表恢复一致；Windows/Linux实际R3进程回退；PR与合并main CI通过 |
| Linux race/性能 | 单元 race、真实存储 race、参数/机器/时长/资源报告 | PASS：完整存储race86项114.010秒，无SKIP；预算后专项6.438秒 |
| 契约与文档 | OpenAPI 本地引用、路由、部署、当前演示、版本事实 | 路由/引用自动测试PASS；M7与技术文档同步，以末检记录为准 |

## 本轮补齐

- 新增 `GET /ready`，一秒共同预算检查 MySQL/Redis，生命周期停止后 503；Redis 使用剩余预算的读写超时，额外验证“建立连接但不回复”的故障，避免只测试端口拒绝。
- 新增 `APP_HOST`，留空保留原监听行为，本机测试显式回环。没有改本机防火墙、VPN或代理。
- 显式配置 MySQL open/idle 20/10、Redis base/hard 10/20与1000ms池等待；无效预算在连接前拒绝。真实测试各十二路并发，限制到两条连接并观察等待，验证PoolSize不是硬上限、MaxActiveConns才是。
- 新增真正 HTTP 注册/登录、玩家/管理员隔离、双管理员并发封禁/解封、旧 HTTP/WS 撤销和新登录回归；补充取消/匹配与最终确认/期限并发不变量。
- 修正演示材料中 legacy finish 流程、前端“规划中”和旧内存事实的默认表述。保留历史记录，默认演示走已实现 V2。

## 复现要点

环境须使用随机 `PVE_TEST_PASSWORD`、`DB_PASSWORD`、`PVE_VERIFY_PASSWORD`、`JWT_SECRET`、事件/指标独立 token，设置 `DB_HOST=127.0.0.1`、`DB_PORT=23306`、`DB_NAME=game_realtime_v2_test`、`DB_USER=pve_test`、`REDIS_ADDR=127.0.0.1:26379`、`REDIS_DB=15`、`GAMEPLAY_MODE=v2`。测试用户名使用唯一合法 `PVE_VERIFY_PREFIX`。内部事件开启仅服务进程保留；Unity 子进程由脚本清除服务密钥环境。

持续运行的登录/握手限流设为每分钟 600，连接配额仍为全局 256/单 IP 64；这是压测配置，不修改默认每分钟 20/30。验收时间必须用实际 elapsed，不能把默认五分钟误记为三十分钟。真实存储测试需要 `PVE_INTEGRATION=1`；当前/R3 二进制必须明确提供 `PVE_R4_SERVER_BINARY/PVE_R3_SERVER_BINARY`。完整复用入口 `deploy/verify-r4.ps1 -RollbackBackendPath <审核过的R3 backend>` 会自行隔离和清理，应在其他测试端口已释放时运行。

## 自动检查与执行纠正

完整Windows存储回归86个顶层用例，133.906秒；Linux真实存储race同样86项，114.010秒，均失败0、SKIP0，包含实际当前/R3二进制回退。追加连接预算后，Windows/Linux重新验证预算、就绪、旧进程回退四项，7.649/6.438秒，无SKIP；普通Go/vet、双前端构建及最终distroless非root镜像启动/停止通过。追加预算的两个新用例与前86项合计88个不同顶层用例，不能把专项重跑算作额外四项新能力。

首次新HTTP回归将注册成功写成200而实际契约为201，修正测试后专项与完整回归通过，未改注册行为。Redis投影由首个Worker tick异步重建，初次验收过早查询，调整为十秒内等待并核对MySQL人数后通过。首次Linux登录shell重置Go PATH改为普通shell；Unity脚本要求工作目录预先存在，创建后两种真实Player剧本均通过。首次记录保留在私有目录，不把初次失败隐藏为“从未失败”。

最终预算构建Windows SHA256 `4b41420c40cd54073d3fef9c43d7996009839740728b1d8881a3a153785d2c3f`；Linux `a55c022320531fa75345b8a3ea576b053678a29ce34194a4748c1597ceb8a8dc`。本机镜像ID `sha256:eae24404e7766c826b4ed05124aa2c7c303b36a844b3479acb03dd36f36f91e3`，没有推送镜像仓库，不用该ID宣称公网已部署。

## R5再验与退出结论

完整R5脚本再次PASS，私有目录 `r5-evidence/f1cc6b2f70f149fca8c7733849738d55`。真实Windows存储测试6.711秒、Linux真实存储race6.660秒；另执行Go/C#契约、查询权限/deadline、发布确认/事务后ACK、重复/冲突/死信、实际进程commit后退出42、broker/SQL/RPC重启、独立投影恢复及主资产隔离。本轮没有把MQ接入主发奖事务。

M7本机验收完成，二期本机范围可收口。跨机器、OTel、真实战斗服/DS、生产身份/部署、多实例、容量上限和旧学习包彻底删除仍按原边界处理。当前88项测试和有限持续运行不证明任意环境永久无Bug。

本轮发布从干净副本同步已验收代码/公开文档，并通过 PR #4 与合并 main 的两套远程 CI；没有覆盖开发数据库。末检确认测试进程、监听、容器/网络/卷和随机凭据已清理，原开发容器保持原先停止状态，Docker Desktop恢复停止。私有日志/摘要/备份扫描没有本轮凭据或JWT命中，临时凭据删除；构建与去敏证据在仓库外保留。相关Markdown/YAML文档的链接、围栏和空白检查通过，最后OpenAPI引用与配置测试重新通过。
