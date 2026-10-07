# Unity PVE 控制面

使用 Unity 6.3 LTS `6000.3.25f1`。在 Hub 添加包含 `Assets/Packages/ProjectSettings` 的本目录；包清单锁定 Newtonsoft JSON 3.2.1、uGUI 2.0.0。打开空场景后 Play，`PveDemoPanel` 自动创建操作界面。

该客户端验证账号登录、好友房间、个人任务、匹配候选、HTTP快照及断线重连，不执行战斗模拟。普通客户端不能发送受信loaded/kill/finish或奖励金额。事件Bot使用独立loopback监听器。令牌只在客户端内存中，退出进程后需要重新登录。

构建Windows Player：

```powershell
& '<Unity Editor目录>/Unity.exe' -batchmode -quit -projectPath '<工程副本目录>' -executeMethod PveDemoBuild.Windows -pveBuildOutput '<仓库外输出目录>/PveDemo.exe' -logFile '<仓库外输出目录>/build.log'
```

构建方法会生成 `Assets/Scenes/PveControl.unity`，建议在工程副本执行。初始化需要有效的Hub许可证；不把Unity账号密码传入命令行。`Library/Temp/Logs/UserSettings` 和构建产物不提交。

`deploy/verify-r3-unity.ps1` 要求PowerShell 7.4+、隔离V2服务、测试数据库端口23306，以及父进程中的独立事件凭据。它注册临时账号、启动四个Player并提交Bot事件，可加 `-MixedParty` 验证好友双排与两个单排。Player子进程清除事件、指标、数据库和服务密钥环境变量；测试凭据只在各自进程环境中传入。本机两种四人剧本已验证，跨实体机器尚未验收。

运行规则与迁移见 [PVE发布指南](../../docs/pve-release-guide.md)，真实验收边界见 [R3发布记录](../../docs/testing/r3-release-acceptance.md)。
