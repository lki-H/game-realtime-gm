# Go游戏后端基础与可靠通信实验

本仓库保留游戏业务后端的演进历史，提供有限维护的V2／GM基础和独立RPC／消息实验。可玩地下城及当前简历主展示入口为 [Dungeon Contract](https://github.com/lki-H/dungeon-contract)，它从本仓库401247d基线派生，随后加入独立Unity战斗权威与完整试玩流程。

## 当前维护范围

| 部分 | 职责与边界 |
| --- | --- |
| V2／GM后端 | 登录、社交房间、招募匹配、Run、任务、逐人结算、资产账本、维护与观察；模块化单业务进程 |
| R5独立实验 | 恢复副本只读gRPC、独立RabbitMQ投递及幂等报表；主业务不依赖它发奖 |
| legacy | 显式历史回归及兼容查询，不继续扩展玩法 |
| Unity／Web | 本仓库的控制面与验证窗口；可玩战斗在后继Demo |

后续只处理正确性、安全、依赖和可复现性，不并行发展第二套游戏玩法。两库的公共缺陷逐项核对、分别验证，不共享PASS。[维护与共享修复清单](docs/maintenance/README.md)解释这次收口。

## 安全本机启动

需要PowerShell 7.4或更新版本、Docker Linux引擎和Go。go.mod的1.26.0是模块最低要求；CI／Docker固定1.27.3是本项目当前验证工具链，二者不是同一个声明。本机运行也应安装1.27.3，避免用尚未修复已知安全问题的较低版本构建；入口的GOTOOLCHAIN=local不会自动下载安装工具链。完整步骤见[启动指南](docs/pve-release-guide.md)。

```powershell
./deploy/local.ps1 -Action Prepare
./deploy/local.ps1 -Action ServicesStart
./deploy/local.ps1 -Action Initialize
./deploy/local.ps1 -Action Check
./deploy/local.ps1 -Action CreateAdmin -Username demo_operator -PasswordFile '<仓库外受限密码文件>'
./deploy/local.ps1 -Action Server
```

默认Isolated新环境仅开放本机HTTP18080、MySQL33306、Redis36379，使用独立随机凭据与独立卷，不采用原开发库或Dungeon Contract存储。初始化不导入固定管理员seed。CreateAdmin拒绝覆盖已有账号。

停止前台Go服务后，`./deploy/local.ps1 -Action ServicesStop`只停止该入口拥有的存储并保留卷。端口占用、部分凭据或归属不明时停止，不删库、不换密码修复。Existing只接受显式配置的Check／Server；Check是只读查询，Server会正常恢复并处理业务，不能把它当只读检查。

## 后端与实验验证

```powershell
cd backend
go test -p 1 ./...
go vet -p 1 ./...
go mod verify
```

普通测试中的显式集成跳过不是存储通过。现有CI继续执行真实MySQL／Redis race和固定旧版二进制回退，不把旧历史成绩迁给新提交。

R5是另一个Go模块，入口、共享Go／C#消息样例、角色配置、只读inspect和单条人工重投见[实验说明](experiments/r5-m6/README.md)。gRPC读取恢复副本，MQ写独立报表，两者不是同一查询链路。它不等于主业务微服务化、实时CDC或集群高可用。

## 架构与证据导航

- [项目简介](docs/project-introduction.md)、[架构](docs/architecture.md)、[数据流](docs/data-flow.md)
- [HTTP契约](docs/openapi.yaml)、[WebSocket](docs/ws-protocol.md)、[当前安全控制](docs/security-design.md)
- [测试与验证范围](docs/test-plan.md)、[启动和维护](docs/pve-release-guide.md)、[公开索引](docs/public-index.md)
- [历史M7发布](docs/testing/m7-release-acceptance.md)、[历史R5验收](docs/testing/r5-m6-acceptance.md)

本次代码和后续CI结果见[维护记录](docs/maintenance/README.md)。历史本机数据仍属于当时构建，不外推商业并发、跨机器稳定性或生产经验。没有将受控Bot事件称为真实战斗。

## 许可与公开边界

自有代码和文档采用[MIT](LICENSE)，第三方与生成代码的运行库条款见[依赖说明](THIRD_PARTY_NOTICES.md)，问题报告见[安全策略](SECURITY.md)。不提交private、真实账号、数据库备份、缓存、引擎或玩家二进制。原编号SQL和v1线协议保留原字节；目录与文档整理不重写历史提交。
