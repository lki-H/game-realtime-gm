# 版本发布说明

> 文档角色：已完成能力、修复、验证与限制的版本台账
> 权威级别：L2（阶段发布记录）
> 状态：一期与 V2 本地验证记录已建立
> 适用范围：Day27-Day35 与后续公开版本
> 事实来源：代码、测试、验收记录与性能报告
> 最后更新：2026-10-11

## M7：2026-10-11 发布收口

补齐依赖就绪探针、主监听与连接池预算、真实 HTTP 身份和匹配竞态回归；修复七项社交、邀请、启动和 Unity 缺陷。固定构建持续运行 598 局/1801.71 秒，逐局失败和资产差异均 0；追加修复后的 92 项真实存储 Windows/Linux race 通过。长运行与追加修复分别留证，发布状态和精确远程 CI 见 [M7 发布基线](testing/m7-release-acceptance.md)。本次不改编号 SQL，不迁移开发数据，不部署公网，跨机器延期。

## R5/M6：2026-10-10 本机独立 RPC/MQ 实验

全面复核修复JWT过期WS、坏outbox批次阻塞、加载失败Party回队列、非房主成员拉黑绕过、客户端迟到身份响应、任务尝试终态和R5 quorum/过期重投。新增回归与GM通知观察，真实主集成/race、React浏览器/构建、Unity编译、R5验收与远程CI通过；已随PR #3发布到main，见 [复核记录](testing/20261010-full-review.md)。

R5/M6包括固定Go/C# Proto、只读结果/排行RPC、恢复副本outbox桥接、最小权限账号、confirm/ACK/重试/死信/幂等报表、进程与存储重启、主链路隔离及启停回退。main提交bfd6f284bc04d91f5354ec5382f35c24e0c91dfd的主项目run38055734806和R5 run38055734817全部成功；主资产事务不依赖实验，开发数据未迁移。完整发布证据见 [R5发布基线](testing/r5-release-acceptance.md)。

## R4：2026-10-08 本机维护、切换与回滚

发布已通过 [PR #2](https://github.com/lki-H/game-realtime-gm/pull/2) 合并main，提交`834d7261184a5f970651246f298dcde61f472c39`；[main CI 37762156519](https://github.com/lki-H/game-realtime-gm/actions/runs/37762156519) 后端/双前端、真实存储race、实际R3/R4回滚和镜像构建全部success。完整提交、迁移、备份与回滚SHA见 [R4发布与版本基线](testing/r4-release-acceptance.md)。

- 新增 `pve_service_control` 持久化准入、版本化 operator 控制、排空快照、候选/票据清理、旧匹配 key 只读扫描与分批删除工具。
- 默认 `GAMEPLAY_MODE=v2`；旧一期通过显式 `GAMEPLAY_MODE=legacy` 回归，V2 不装配旧 WebSocket 写入口和旧超时 Worker。
- 本机隔离验证通过：维护期间拒绝新匹配/Run、已有 Run 继续结算、审计失败原子回滚、needs_repair 阻止关闭、重复请求不重复审计、旧 Redis key 不误删 V2/online/榜单 key。
- 实际 R3 二进制回退验证保持历史查询、旧命令拒绝、事件去重和奖励账本一致；资产奖励总额495、发奖记录7、账本错误0。
- R4 day40 已通过恢复副本和本机开发库备份恢复迁移；跨机器验收、共享部署观察窗口、真实战斗服和彻底删除旧学习包仍未完成。

## R3：2026-10-07 本机工程验收

- Unity Windows Player完成真实V2登录、WebSocket、四人匹配、个人任务、重连和逐人结算；混合来源局包含好友双排、两个单排和不激活任务玩家。
- Go新增归档、操作回执恢复提示、WS连接配额、loopback Prometheus指标、R3迁移、隔离验证工具和CI；GM增加分页、失败响应、待修复operator重试界面。
- 验证通过：隔离MySQL/Redis V2回归、Go测试/vet、React构建、官方npm registry生产依赖审计、备份恢复、资产对账、短时soak和Unity Windows构建。
- 未包含：跨机器、真实战斗服、Unity DS、商业容量承诺。默认legacy未切换，R4/R5继续延期。
- 五分钟本机 soak：101轮、0失败、每轮资产对账0，延迟 p50=1.986s、p95=2.083s、max=2.101s；不是商业容量证明。Docker Hub代理已恢复，`game-realtime-gm:r3-local` 本机构建成功。
- 远程CI：R3基线 `r3-ci` 提交 `e286589` 的 [GitHub Actions](https://github.com/lki-H/game-realtime-gm/actions/runs/37598715443) 全部成功；审计修复分支 `r3-integration` 提交 `64a4263` 的 [run 37625710404](https://github.com/lki-H/game-realtime-gm/actions/runs/37625710404) 也全部成功，覆盖 Go、隔离 MySQL/Redis race 和双前端构建。

## V2：2026-10-06 本地实施

- 新增：运行模式隔离、社交/邀请/招募、可靠 Party/票据/候选/Run、可信测试事件、个人任务、有限增援、自动软货币结算、pending/outbox 和 Redis 重建。
- 新增：默认关闭的独立本机事件监听器、`pve_event_bot`、只读 GM API、Prometheus 文本、React 观察页面和 Unity 控制面示例。
- 验证：普通 Go/vet、真实 MySQL/Redis 集成、Linux race、React 构建与真实服务浏览器流程；Unity Windows Player 四人控制面已验收，Unity Editor 场景细节、跨机器和真实战斗服仍未验收。
- 范围：legacy 数据和模块保留，该日期本地记录不表示当时已发布；后续发布见 R3—M7 版本基线，运行入口见 [V2 指南](pve-release-guide.md)。

## Phase 1：共斗 PVE 业务后台闭环

### Day27：小队广播与协议保护

- 新增：`squad.state.changed`，统一 `server.error`，WebSocket 文本/JSON 与 4096 字节限制。
- 验证：创建、加入、ready、离队广播与小队 Manager 测试。
- 限制：单进程内存小队，服务重启不恢复。

### Day28：任务会话状态机

- 新增：任务创建、ready、start、finish、cancel、查询和状态广播。
- 新增：断线清 ready、在线成员队长转移和重连状态恢复。
- 验证：正常/取消/非法迁移、重复活跃任务和小队断线测试。
- 限制：任务会话不是战斗服，不执行固定 Tick、物理、技能或 AI。

### Day29：Redis 匹配 ticket

- 新增：排队、查询、取消、30 秒超时、队列位置、玩家索引和全局超时索引。
- 验证：状态迁移、标识符、超时索引编解码和 Redis 对照。
- 限制：没有 `matched` 状态、真正撮合或跨实例互斥。

### Day30：服务端结算记录

- 新增：finished 任务结算、服务端耗时/分数/固定奖励、nonce 和 MySQL 记录。
- 验证：无效任务/参与者/状态、重复请求和数据表对照。
- 限制：依赖当前进程内存任务会话。

### Day31：幂等资产强事务

- 新增：`idempotency_key`、玩家余额、append-only 资产流水、多人奖励固定锁顺序。
- 修复：重复提交统一返回已有结果，不重复发奖。
- 验证：UNIQUE 约束、`SELECT ... FOR UPDATE`、事务与余额边界测试。
- 限制：没有 Outbox、自动补偿或通用资产查询 API。

### Day32：排行榜与战绩

- 新增：Redis 个人最佳分、同分先达到者优先、Top N、个人排名和 MySQL 战绩分页。
- 验证：member 编解码、排序、hash tag 和接口/Redis 对照。
- 限制：没有赛季、TTL 或自动重建任务；Redis 不是资产事实源。

### Day33：GM 实时观察

- 新增：单实例实时摘要、玩家聚合观察、结算分页/筛选和 GM 榜单。
- 新增：`X-Request-ID` 与只读观察标准日志。
- 验证：筛选构造、身份隔离和跨内存/Redis/MySQL 数据对照。
- 限制：观察不是跨存储原子快照或全服监控。

### Day34：并发、诊断与性能基线

- 新增：`ws_bot` 多玩家流程、可选本机 pprof、安全 AccessLog 和 Docker Linux race。
- 修复：小队离队成员未写回；`ws_bot` 高频 timer 日志过多。
- 验证：8 客户端冒烟、20 客户端 10000 echo 0 失败、race 退出 0、连接关闭后 goroutine 回落。
- 限制：单机回环短时数据，不是容量上限、商业 CCU、SLO 或生产经验。

### Day35：一期收口

- 新增：README、当前架构、数据流、一期边界和展示证据。
- 明确：当前为模块化单体，没有微服务、Zinx、Kubernetes、正式前端或云部署。
- 限制：后续规划不能反向写成一期已实现能力。

## 文档治理版本：2026-08-31

- 新增：文档治理、公开索引、技术可行性、OpenAPI 3.1、安全、测试、运维、备份恢复和版本台账。
- 重构：需求/HLD/LLD/数据库/API/WS 各自成为单一事实源，历史与研究材料降为非权威。
- 行为变化：无。本版本只修改文档，不改变 Go、SQL、Docker、HTTP 或 WebSocket 实现。

## 发布表达规则

### 2026-10-06：R2 合作 PVE 产品模型闭环

- 新增：day38产品表、任务/关卡预览与有限推荐、招募独立准备/来源票据、自愿续组、周期任务与停用保留进度、即时任务pending、damage/heal/rescue受控证据、网页PVE控制面和Unity控制面字段。
- 验证：R2隔离MySQL/Redis产品剧本、普通Go/vet、R2网页生产构建、真实V2服务浏览器登录/任务预览；Unity Editor仍未验证。
- 环境：day38已在本机开发库完成恢复副本验证、迁移和启动冒烟；真实游戏服、装备/护送事实、长运行容量和默认玩法切换留后续批次。

### 2026-10-06：R1 本地迁移与可靠性

- 新增：编号迁移账本/CLI、隔离备份恢复CLI、本人组合活动查询、逐人结果/奖励查询、operator审计重试和基础HTTP/WS限流。
- 修复：首位玩家结算失败阻断后续人、无活动锁遗漏Party、旧释放回调不比较活动owner、候选无限自动重组、事件/事务失败证据缺失。
- 验证：真实隔离MySQL/Redis业务与故障用例、普通Go/vet、Linux race；具体命令和边界见V2验收记录。
- 环境：用户授权后原开发库备份/副本恢复/迁移/对账和V2启动检查通过，默认legacy保持；真实客户端、完整容量/归档、legacy退役和真实战斗服按原后续批次推进。

每条后续记录必须区分：新增能力、修复、实际验证和已知限制。没有真实生产用户、值班、发布或事故时，不使用“生产验证”“线上故障”或“商业级”表述。

### 2026-10-10：M7本机综合验收

- 补齐：GET /ready及共用探针预算、APP_HOST回环监听、显式MySQL/Redis连接预算和非法配置拒绝；新增真实HTTP管理并发、取消/期限竞态与硬连接上限测试。
- 验证：598局1801.71秒0失败/对账0、四条20分钟连接无意外关闭、两种Unity四Player和React流程、53表备份恢复、Linux信号退出/所有权恢复、Windows/Linux实际R3回退及完整R5 RPC/MQ再验。
- 证据：完整存储86项及追加2项预算用例，race无SKIP；详细版本、数据和阶段限制见 [M7验收](testing/m7-acceptance.md)、[性能](performance/m7-local-soak.md) 与 [二期总结](phase2-summary.md)。
- 边界：二期本机范围完成，跨机器延期；OTel、真实DS/生产/HA/容量仍后置。R5 main已发布；M7发布提交、PR与远程检查见 [M7发布基线](testing/m7-release-acceptance.md)。

### 2026-10-10：M7后完整复核（未发布）

- 修复七项：好友重复pending通知、同一对玩家反复申请绕过额度、拉黑/接受并发冲突、无效邀请、单连接池启动卡死、Unity报告读取竞态、构建覆盖已有场景。
- 验证：当前原生/Linux真实存储92项与实际R3回退、普通/单元race/vet、双React构建与生产依赖审计、新Unity构建保留场景和混合四Player、11局短时资产对账0。
- 范围：已应用SQL、主依赖、开发数据与私人VPN保持；更新需求/安全/WS/运行/交接与当前私人源快照。详情见 [最新复核](testing/post-m7-audit-20261010.md)；M7和本轮追加修复均未提交发布。

### 2026-10-07：完整项目审计复核

- 修复：可信事件拒绝重放、个人任务隐私裁剪、管理员实时账号校验、Worker持久化确认、WS生命周期、规则/迁移边界和前端/Unity恢复流程。
- 验证：源Go全量测试/vet、隔离MySQL/Redis Docker race、双前端构建与依赖审计、Unity普通/混合四人Player闭环均通过。
- 限制：跨机器、真实战斗服、生产环境与R4退役继续延期；本轮 Docker 新镜像已完成构建和 `/health` 启动冒烟。
