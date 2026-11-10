# 本地部署、运维与排障手册

本文是当前运行入口。2026-11-10维护收口后，新安装统一采用`deploy/local.ps1`；原手工schema／seed和固定容器步骤不再作为新安装指南。历史命令与当时成绩保留在Git基线`401247d`及各阶段验收中，不据此改动已有数据。

## 1. 运行方式

本项目面向本机与隔离测试，不提供公网生产部署。Go模块最低1.26.0；CI、Docker及建议本机构建工具链为1.27.3。此安全升级已获明确授权，原工具链的测试成绩不迁给新版本。

- `Isolated`：独立身份、随机凭据和命名卷；HTTP18080、MySQL33306、Redis36379均绑定loopback。
- `Existing`：明确配置现有本机安装，只允许Check和Server；不初始化、不生成凭据、不管理其容器或卷。
- R5：使用单独项目、合成恢复副本与报表库，按角色连接依赖，不接入主发奖链路。

MySQL保存账号、事实、权限、奖励和账本；Redis是可重建投影。`/health`证明进程活性；`/ready`在共同预算内检查依赖及进程生命周期，不代表维护状态允许新匹配。

## 2. 新安装与停止

准备PowerShell7.4、Docker Linux引擎和Go1.27.3，核对：

```powershell
go version
docker version
docker compose version
```

从仓库根目录顺序执行：

```powershell
./deploy/local.ps1 -Action Prepare
./deploy/local.ps1 -Action ServicesStart
./deploy/local.ps1 -Action Initialize
./deploy/local.ps1 -Action Check
./deploy/local.ps1 -Action CreateAdmin -Username demo_operator -PasswordFile '<外部受限单行密码文件>'
./deploy/local.ps1 -Action Server
```

Prepare只创建仓库外受限状态，不启动容器。Initialize仅对该入口拥有的新空库应用原schema与day37—40，失败状态保留，不自动重试；不导入固定管理员seed。CreateAdmin拒绝覆盖已有账号。

Server在前台运行，确认`http://127.0.0.1:18080/ready`，按Ctrl+C结束该服务。停止依赖：

```powershell
./deploy/local.ps1 -Action ServicesStop
```

该动作只停止本入口验证为自有的资源，保留卷。日常重启不使用`down -v`、prune或删秘密文件。端口冲突、归属不明、部分凭据和密码不匹配均停止核查，不自动换端口、杀用户程序、接管卷或重建数据。详细流程、外部状态位置和前端5173／5174代理见[本机启动指南](pve-release-guide.md)。

## 3. 显式配置与安全边界

| 变量 | 裸服务行为 | 启动器行为 |
| --- | --- | --- |
| APP_HOST／APP_PORT | 默认127.0.0.1／8080，HOST须有效IP | Isolated固定127.0.0.1／18080；Existing要求明确本机地址 |
| DB_HOST／DB_PORT／DB_USER／DB_NAME | 默认本机历史名称，DB_PASSWORD为空则启动前拒绝 | Isolated独立身份；Existing读取显式配置和原外部密码文件 |
| JWT_SECRET | 无开发回退；32—4096字节，独立于存储密码 | Isolated生成；Existing必须引用原受限文件 |
| REDIS_ADDR／REDIS_DB | 默认localhost:6379／0；旧无密码环境仍可显式配置 | Isolated独立密码和端口；Existing明确引用或声明无密码 |
| DB_MAX_OPEN_CONNS／DB_MAX_IDLE_CONNS | 默认20／10，检查连接预算 | 不绕过服务校验 |
| REDIS_POOL_SIZE／REDIS_MAX_ACTIVE_CONNS／REDIS_POOL_TIMEOUT_MS | 默认10／20／1000，有界连接与等待 | 不绕过服务校验 |
| PPROF_ENABLED／PVE_TEST_EVENTS_ENABLED／PVE_METRICS_ENABLED／PVE_ARCHIVE_ENABLED | 默认关闭，诊断和服务凭据独立 | 启动器清除继承的启用标记，避免无意开放 |
| GOTOOLCHAIN | 由直接调用环境决定 | 固定local，旧Go不会自动下载，需先安装建议修复版本 |

不要打印环境全集、DSN、Authorization、JWT、SQL数据正文或WebSocket query。`docker compose config`可能展开秘密，不将输出放入公开日志。容器内部确需外部路由时显式声明`APP_HOST=0.0.0.0`，不能以此将宿主端口暴露公网。

Existing配置见[示例](../deploy/local-existing.example.json)，配置和秘密应放仓库外；Check仅查询结构、迁移账本checksum和RedisPING。Server会执行Recover／Worker与正常业务写入，同库其他服务必须先停，不应把Server当只读检查。

## 4. 排障、诊断与恢复

- 内存：重任务串行，Windows依赖启动准入2688MiB提交／1536MiB物理，Go准入1792MiB／1024MiB。自有Go进程维持1GiB／512MiB保留线；Compose只做启动准入，不宣称容器持续受保护。
- 端口：先查占用者。保持已选择的安装和端口，不为通过测试改用户系统服务。
- 认证：核对同一安装的原凭据和JWT身份，不输出密钥或刷新已存在密码。玩家JWT与管理员JWT不能互换。
- 迁移：dirty、checksum不符、列或索引缺失须停写核查，禁止修改已应用SQL或靠删账本重跑。Check不能代替迁移修复工具。
- WS：确认玩家会话、V2协议、来源和请求身份；通知丢失用查询恢复，不能据连接成功承诺恰好一次通知。
- 依赖：检查自有容器健康和仅本机连接；日志保存在仓库外，分享前脱敏。不要重新导入固定seed排除认证故障。
- 恢复：先做仓库外备份并在新库验证；按[备份恢复规则](backup-and-recovery.md)使用隔离目标，不能把旧备份覆盖已产生新奖励的活动库。

pprof、独立metrics、测试事件只用于有明确需要的本机诊断，使用各自独立凭据及预算，诊断后关闭。启用这些高级配置需要直接显式配置服务，统一本机入口不会从调用方继承开启标记。原本机监控模板不等于公网可观测平台，Prometheus/Grafana及其秘密须与游戏环境隔离。

## 5. R5、维护与发布

独立RPC／消息实验的准备、角色配置、端口、inspect、单条人工重投、审计未知结果和数据重建见[R5说明](../experiments/r5-m6/README.md)。主服务不配置gRPC／RabbitMQ也能完成结算；报告失败不授权修改主资产。实验`-ResetData`只适用于明确的合成环境，不能清理原业务库来排错。

发布保留固定R3源码SHA的实际二进制回退演练、主／R5真实存储race、两前端、镜像、公开文件及可达安全扫描。备份、DDL、回滚和运维必须有明确目标；代码回退不意味着数据库可倒退覆盖。

本仓库不提供自动生产部署、Kubernetes调度、自动扩缩容、TLS公网入口、蓝绿／金丝雀、跨区灾备或集群高可用。本轮实际提交、CI、历史差异与有限维护分工见[维护记录](maintenance/README.md)和[本轮复核](maintenance/refresh-20261110.md)。
