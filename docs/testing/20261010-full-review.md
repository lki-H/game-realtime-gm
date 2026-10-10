# 2026-10-10 项目全面复核与修复

> 文档角色：本轮发现、修复、实际验证与限制
> 权威级别：L2（本机证据）
> 范围：当前源工作区 V2、legacy 回归、React GM/PVE、Unity 控制面、R5/M6及技术文档
> 状态：已修复八类问题，最终相关复核未发现新增可复现错误

## 1. 本轮范围与方法

本轮对照实际源码、协议、规则配置和测试，先添加正确期望的回归复现，再修改；仅使用独立测试数据库和容器，不写开发数据。本文为公开去敏摘要，原始日志、凭据和内部协作资料不进入发布范围。

## 2. 已复现并修复

| 编号 | 原问题 | 根因与修复 | 验证 |
| --- | --- | --- | --- |
| REV-01 | 已建立WebSocket在JWT过期后仍可执行命令 | `pve.Authorize`只校验subject/数据库状态，没有再校验JWT时间。现在强制有效exp，拒绝未来nbf；命令会话失效返回40321，定期检查也关闭连接 | 真实过期WS不能创建Party，缺exp/未来nbf拒绝 |
| REV-02 | 一条坏outbox令全部后续通知停止 | Worker遇到payload格式错误退出整个批次。现在逐条验证收件人/type/data，将坏行标needs_repair并继续；排空/指标计入坏通知，GM新增无正文的通知观察页 | 坏JSON数组与健康通知同批测试，健康行仍发布，坏行阻止ready-to-stop；真实GM页面 |
| REV-03 | 加载失败后好友票据回队列，房间仍显示in_run | releaseTicket只恢复票据/活动锁，漏改Party。回队列明确设置queued，取消后open | 真实加载失败/房间状态/取消专项 |
| REV-04 | 接受邀请可绕过房间非房主成员的拉黑 | 原加入逻辑只检查房主。现在检查所有现有好友成员，与已有匹配阶段检查一致，不中止已开始Run | 三人关系的真实拒绝回归 |
| REV-05 | 旧登录响应可恢复已关闭的PVE网页会话，Unity旧HTTP查询可更新新会话 | 网页token前后同为空不能识别logout。增加session generation；Unity复核登录/查询身份并处理40321，网页禁止同时状态变更及覆盖尚不确定的operation | Chrome动态延迟响应复现修复前accepted/恢复token，修复后rejected/空token；Unity实际编译构建 |
| REV-06 | R5重复进程崩溃超过quorum投递预算可能丢消息 | 主reports队列缺DLX。新增至少一次dead-letter到r5.dead；使用Reject重投测试默认预算保护，explicit NACK在4.3不增加失败计数 | 真实RabbitMQ超过20次failed redelivery进入死信，普通与race通过 |
| REV-07 | R5已经提交的消息超过时效后重投，被误当过期新消息隔离 | 时效校验发生在receipt去重前。仅对已经applied且完整fingerprint相同的过期消息ACK去重，未确认或变更消息仍dead | 真实MQ/SQL确认、过期精确重投ACK、改时间冲突dead |
| REV-08 | 不兼容任务/永久退出玩家的尝试记录在终止后仍active | Run Save原来只更新provisional，漏掉active尝试。终态或该玩家left时关闭active/provisional，已completed保持 | 实际退出后队友Run继续running；不兼容任务在结束后closed |

此外修正文档遗漏：OpenAPI加入outbox观察实体，演示与客户端类型说明不再写“尚未实现”，接手入口包含R5；没有改已应用SQL或规则版本。

## 3. 实际验证

- Windows主后端 `go test ./...`、`go vet ./...`通过。
- V2完整真实MySQL/Redis隔离集成127.842秒通过；增加REV-08后完整重跑114.827秒通过。Linux完整真实集成race99.945秒通过，追加REV-08后的七项专项race5.408秒通过，无race报告。首轮race命令配置了错误地址及挂载范围，测试在连接/文件读取处失败；改为本实验容器地址、挂载项目根后重新通过，不是修弱业务断言。
- React GM/PVE TypeScript检查、Vite生产构建通过；npm官方registry生产依赖audit均0漏洞。
- agent-browser真实后端验证：玩家登录、WS连接、任务目录、创建好友房间；GM operator登录、通知页真实数据和分页。无控制台JS错误。Chrome DevTools额外验证延迟登录/未知operation保护；Vite模块热更新使旧refs失效，重新snapshot后完成。
- Unity 6000.3.25f1后台构建成功，总产物约90.8MB；产物和原始日志保留在仓库外受限证据目录。
- R5独立模块test/vet/mod verify通过；真实MySQL/RabbitMQ、Linux race、进程崩溃/ACK恢复、重建和主结算故障隔离复跑通过，含REV-06/07；原始summary和日志保留在仓库外。
- YAML、本轮文档链接、格式与工作区修改范围核对通过。

## 4. 保留的限制

这表示当前可检查范围的最后一轮没有新增可复现问题，不是“证明任何环境永远无Bug”。跨机器、远程新CI、真实游戏服/DS、生产TLS/mTLS、HA、商业容量和M7长期soak仍不在本轮执行。Unity本轮完成新的编译/Windows构建，未再次启动四个Player混合来源脚本；该脚本因本轮没有向独立进程提供内部事件凭据而在前置校验退出，不算Player PASS。此前R3完整Player证据保留。

普通主测试中的实际旧R3二进制回退在未配置两个binary路径时SKIP；本轮R5完整脚本另完成停止实验后主服务继续结算的回退。不能把后一种证据称为重新执行R3旧binary回滚。

## 5. 数据、清理与下一步

私有日志、修复前文件hash基线、合成数据和Unity产物保留在仓库外受限目录。测试服务/浏览器会话/Vite/容器网络卷/临时凭据清理，原开发卷保留。本文记录本机复核，发布和远程CI状态以对应发布记录为准。

通知needs_repair只支持观察和停止判定；运维者需核对原notification操作/来源修复payload后恢复pending，无法恢复时明确记录并停止该条，不提供GM直接写正文或跳过资产校验的按钮。R5修改队列参数后旧实验卷应reset并重新初始化，不对旧持久队列直接用不同参数重声明。

下一步按用户指令整理本轮修复/R5发布或进入M7综合验收，既定跨机器和生产门槛保持延期。
