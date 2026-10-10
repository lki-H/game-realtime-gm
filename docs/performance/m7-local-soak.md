# M7 本机 PVE 持续运行基线

> 文档角色：负载、耗时、资源样本与性能边界
> 权威级别：L2（实验记录）
> 状态：本机三十分钟持续运行通过
> 适用范围：Windows 主服务与本机 Docker MySQL/Redis
> 事实来源：pve_verify、进程/pprof、MySQL/Redis 原始样本
> 最后更新：2026-10-10

## 环境与版本

Windows，MECHREVO Jiaolong15K，AMD Ryzen 5 7535H（6 核/12 线程），RAM 15.24 GiB。主 Go 1.25.6，Linux 测试镜像 golang:1.27.1-bookworm；MySQL 8.4.11、Redis 7、Docker Engine 29.8.2/Compose 5.5.1、Node 22.19.0。所有玩家和数据库均为合成隔离数据。

R5 main bfd6f28 是发布基线。持续运行使用该业务树加 M7 回环监听/初版就绪探针，Windows server.exe SHA256 `fe09f6a90c3581d0afe70dcc9807ac14336273444ccf63d742529d3f4bec2c51`；就绪预算进一步补强后的最终构建与故障/race 验收另外记录。持续运行期间没有更换服务二进制。

## 负载与采样

`pve_verify -mode soak -duration 30m` 每轮四个玩家：真实注册后的登录、四个 WebSocket、整局确认、同 Run 重连、内部测试事件、共同结束、逐人 Worker 结算及 Reward/Ledger/Balance 对账，轮间休息一秒。另有四条 WebSocket 持续保持 20 分钟，30 秒轮询个人活动；中途运行两个独立四 Player Unity 剧本和网页 GM/PVE。

每约 30 秒记录 UTC、elapsed、working set/private bytes、线程/句柄/CPU累计、goroutine、MySQL Threads_connected、Redis connected_clients/used_memory。pprof/metrics 仅回环，指标 token 不记录。编译/浏览器等并行工作也占用同一机器，因此结果含本机干扰。

## 结果

| 项目 | 实测 |
| --- | --- |
| 执行区间 | 2026-10-10 13:54:50—14:24:52 UTC（本机 Asia/Shanghai 21:54:50—22:24:52） |
| 完整持续时间/轮数 | 1801.71 秒 / 598 局 |
| 失败/对账 | 0 局失败；每局三项对账差异均为 0 |
| 整轮延迟 | p50 1.978 秒；p95 2.085 秒；max 4.086 秒 |
| 持续连接 | 独立四条连接保持 1203.233 秒，提前断开 0 |
| 采样 | 59 个约三十秒样本 |
| working set | 21.98—27.80 MiB；五分钟后的区间 26.03—27.80 MiB；最后 26.47 MiB |
| goroutine/句柄/线程 | 24—50 / 231—286 / 13—18；末次 goroutine 34 |
| MySQL 连接 | 采样最大 Threads_connected 9（包括健康检查/采样/验证器，不等于单服务池全部） |
| Redis 内存/连接 | used_memory 1,262,112—1,357,688 bytes；采样最大 connected_clients 5 |
| 独立客户端 | 普通四 Unity Player 与好友双排混排各一局，均同 Run、重连成功、settled、进程退出 0 |

延迟是整轮四人业务含等待的时长，不是单接口 RTT；不换算为商业 QPS 或 CCU。随机采样遇到局间连接变化会导致 goroutine/连接波动；四条持有连接结束后仍有最后几分钟整局循环。内存、协程和句柄没有表现为随完成局数持续增长；有限样本不能证明永久不存在泄漏。

故障注入前的备份包含 17 名合成玩家、601 个 Run、2407 条 reward grants 与 2407 条 V2 ledger。598 局之外还有两个 Unity 剧本及一个故障修复剧本，故两组计数不同。备份约 14 MiB，恢复后 53 张表 CHECKSUM 全部一致；恢复新库，不覆盖源库。SHA256 `45372b77174439a3209c79b8227e9b8620f235216b0f18748a37a4419305f62c`。

初次就绪补强构建server-final.exe SHA256 `3add802d61f89f5b5810b51b8f5e699037b0c317f8793f2114bba4df78ae3842`。就绪预算分别通过Windows/Linux真实依赖与不回复连接测试；Linux主进程随后验证Redis重建/故障恢复、MySQL所有权失联后退出/重启、SIGTERM退出0及WebSocket关闭。SIGTERM实际约0.501秒，数据库命名锁释放。

最后补齐显式MySQL/Redis连接预算，Windows server-budget.exe SHA256 `4b41420c40cd54073d3fef9c43d7996009839740728b1d8881a3a153785d2c3f`。每种依赖十二路真实并发，配置硬上限2，实际SQL/Redis连接不超过2且各等待10次；最后构建的就绪、真实R3二进制回退、Linux race与镜像启动/退出均专项通过。没有再次对最后这次预算配置运行三十分钟；长运行数字属于开头固定的二进制，业务链路未改，最终配置证据来自追加专项。

资产与历史事实会随完成局数增长，这是正常持久化量；判断泄漏以进程资源、连接/协程回落与 Worker 积压为依据，不能要求 MySQL 行数不增长。单机有限负载只能建立可复现基线，不证明公网/弱网、多实例或容量上限。
