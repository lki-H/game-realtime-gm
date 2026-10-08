# R4 Legacy 退役与回滚计划

> 文档角色：R4 进入条件、退役对象、切换顺序、回滚和验收
> 权威级别：L2（本机实施与共享环境后续门槛）
> 状态：R4 本机实施与隔离回滚已完成；共享/跨机器门槛延期
> 最后更新：2026-10-08

## 1. R4 目标与边界

R4 将 V2 设为本机受控环境的默认玩法，停止 legacy 的业务写入口，同时保留历史查询、资产账本、审计和可回滚版本。R4 不迁移一期历史为 Run，不删除证据，不引入多实例、真实战斗服、Unity Dedicated Server、gRPC 或 MQ。跨机器验收仍是共享/正式环境切换前的独立门槛；本轮只完成本机切换、排空和回滚证据。

## 2. 进入条件

本机R4已按授权实施；以下仍用于共享环境正式切换和彻底删除兼容代码，不能以本机结果代替：

1. 当前R4发布PR与main的实际GitHub Actions保持绿色；记录提交、实际构建镜像及迁移校验值。固定R3兼容回滚SHA和当前验证见 [R4发布](../testing/r4-release-acceptance.md)。
2. 【共享切换门槛，延期】至少两台测试机完成 Unity 登录、组队、匹配、重连、任务和结算；弱网/断线结果可从 HTTP 快照恢复。
3. 所有仍支持的客户端只使用 `schema_version=2`、`v2.*` 和 `/api/v2/*`，没有客户端依赖 `mission.finish` 或 `settlement.create`。
4. 开发库与恢复副本均完成day37/day38/day39、备份校验和资产/奖励对账；回滚版本必须支持现有V2事实和同一schema。恢复副本只用于演练，不以旧备份覆盖有新奖励的活动库。
5. R4 变更有明确停写窗口、观察窗口、负责人和回滚触发条件；不在有活动 Run、pending settlement 或 needs_repair 时直接切换。

## 3. 退役清单

| 对象 | 处理 | 保留内容 | 验收 |
| --- | --- | --- | --- |
| legacy WebSocket 写命令 | 关闭 `squad.create/join/leave/ready`、`mission.create/ready/start/finish/cancel`、`matchmaking.enqueue/cancel`、`settlement.create`；其旧 `*.me` 查询另行兼容评审 | 持久历史查询和隔离legacy回归；兼容回滚版本不自动重开非可信奖励入口 | 已退役命令返回明确错误，V2仍可用；历史查询继续受参与者权限保护 |
| legacy 内存 `squad/mission/matchmaking/settlement` 装配 | 客户端迁移且回滚窗口结束后，再删除无引用装配；先保留测试和学习代码 | 迁移前版本、历史测试卡和文档证据 | `rg`、编译、测试确认没有运行依赖 |
| 旧 Redis key/队列 | 只处理 `matchmaking:queue:*`、`matchmaking:ticket:*`、`matchmaking:player:*`、`matchmaking:timeouts`；先清点、再停止写入、观察、逐key清理 | `v2:*`、`online:*`和历史 `leaderboard:{mission_id}:scores/players`全部保留 | SCAN计数/抽样及写入观察通过，V2仍可重建；不使用FLUSH或KEYS全库阻塞查询 |
| legacy 默认配置 | `GAMEPLAY_MODE` 从 `legacy` 切为 `v2`，先在测试环境，再在开发环境 | 显式 `GAMEPLAY_MODE=legacy` 回滚配置 | 启动日志、路由和写入模式与配置一致 |
| 旧文档/公开说明 | 更新当前入口、README、API/WS契约和发布说明；Day学习记录不批量改写 | 历史 Day、交接、回归卡和内部证据 | 文档不把退役规划写成已完成删除 |
| 一期历史数据 | 不删除、不迁移成 V2 Run | `mission_records`、奖励、账本、审计及历史查询资格 | 备份恢复后余额和历史摘要一致 |

代码删除候选为 `internal/router` 的legacy WS注册/超时Worker、`internal/handler/ws.go` 中对应写分支、旧 `squad/mission/matchmaking`装配。`internal/leaderboard`、历史Observation、`internal/model/settlement.go`等仍有HTTP历史查询用途，不能按包名整体删除。`settlement`与 `ws` helper要先逐引用核对。回归工具 `cmd/tools/ws_bot`仍使用旧协议：保留为显式隔离legacy工具，或先替换再退役，不能与新客户端同时误称V2验证。

## 4. 切换顺序

1. 在隔离环境应用已验证迁移并恢复 Redis 投影。
2. 已实现并验证停止新enqueue/Run的准入开关与排空策略，保留历史读。`GET/POST /api/admin/v2/control` 由 operator 使用版本化幂等请求控制；existing Run继续由可信来源推进，Worker继续处理pending。
3. 部署同时支持 V2 查询和必要 legacy 历史查询的版本，默认设置为 `GAMEPLAY_MODE=v2`。
4. 验证健康检查、V2 登录、WebSocket、匹配、事件、Worker、结算、资产和 GM 观察指标。
5. 在实际停写前确定责任人、开始/结束时间、日志基线及回滚预算；观察至少覆盖成功/失败/重连和Worker恢复剧本。没有legacy新写入、重复奖励、未解决缺号或needs_repair后，再清理旧匹配key；本计划不擅自定义固定7天或永久保留期限。
6. 观察窗口结束后才删除无引用的 legacy 装配代码；每一批删除单独提交，便于回滚。

## 5. 回滚方案

回滚不是删除 V2 表或恢复旧数据库备份。触发以下任一条件时，停止新写入并回到上一版本：V2 结算重复/资产对账失败、事件处理出现无法恢复的缺号、活动锁无法释放、旧历史查询损坏、关键客户端无法重连，或 `needs_repair` 超过发布前基线。

回滚步骤：

1. 禁止新匹配和新 Run，等待正在处理的 Worker 完成或记录为 pending。
2. 保留当前 MySQL 事实、账本、outbox、审计和故障记录；不要删除 V2 表或执行全库清理。
3. 优先回退到本次合并的、已验证schema兼容的R3基线，并保持 `GAMEPLAY_MODE=v2`、归档关闭及新匹配暂停。不得直接回退到仅有legacy的旧main读取/处理V2结算，也不得自动重启 `mission.finish` 发奖。只有全体V2活动已终结且隔离验证确认后，才单独决定是否恢复legacy本地演示。
4. 重新读取历史、资产和 pending 状态，确认旧版本不会重复发奖；V2 新产生的事实由专门恢复版本读取，不伪造为 legacy mission。
5. 使用发布前备份做恢复副本对账；只有确认事实完整后，才决定是否重新开放队列。
6. 记录回滚原因、操作编号、迁移版本、影响 Run 和修复结果；回滚完成不等于退役条件重新满足。

R4切换前必须记录：发布和回滚commit SHA、服务/客户端版本、MySQL迁移checksum、备份哈希、Redis DB与key白名单、在途票据/Run/pending/needs_repair数量、余额/奖励/流水摘要。切换前后摘要分别对账；只读历史使用同一DATETIME语义，旧奖励不可伪造为Run奖励。

回滚演练至少在新恢复库验证：R3基线能启动，原合法奖励重复查询/事件不增余额，pending重试只处理未完成个人结果，旧历史可查，权限/单实例named lock仍有效。演练不能通过删除资产依据或修改迁移账本制造成功。

## 6. R4 验收清单

- [ ] 跨机器客户端验收通过并留存环境、版本、网络和结果。
- [x] 本机默认 `v2` 启动、V2 路由和写入模式通过检查。
- [x] 本机 legacy 写入口拒绝，历史读接口和显式回归模式可用。
- [x] 本机没有 queued/proposed/loading/pending 遗留未处理记录。
- [x] 本机 Redis 旧 key 白名单清理保护、V2 投影从 MySQL 重建通过。
- [x] 本机资产、奖励、账本、审计和历史查询对账通过。
- [x] 本机 rollback 版本、备份、停写命令和责任人流程已实际演练。
- [ ] 删除每批无引用代码后，Go 测试、V2 测试、CI 和契约检查仍通过。

本机 R4 实施项已按 `docs/design/v2-r4-implementation.md` 完成并验证；跨机器复选项仍未完成，因此不能将 R4 描述为共享/生产切换完成，也不能把删除旧代码描述为 R3 成果。
