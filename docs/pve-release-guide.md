# 本机启动与维护指南

默认玩法仍为V2，旧legacy只支持显式历史回归。Go模块最低要求1.25.6，当前CI与镜像固定验证1.27.1；不因本机环境方便而自动改工具链。

## 新安装 Isolated

准备PowerShell7.4或更新版本、Go、Docker Linux引擎。首次使用前确认18080／33306／36379未被其他程序占用，调用仓库根目录的入口：

```powershell
./deploy/local.ps1 -Action Prepare
./deploy/local.ps1 -Action ServicesStart
./deploy/local.ps1 -Action Initialize
./deploy/local.ps1 -Action Check
```

Prepare仅创建仓库外受限状态、四份独立随机秘密和资源身份，不执行DDL。已有同名资源、部分秘密、链接路径或端口冲突会拒绝，不重新生成凭据。默认状态路径由本机用户目录与源码路径身份派生，可统一使用`-StateRoot '<仓库外目录>'`覆盖；每次调用必须使用同一路径。

Initialize只对本入口拥有的、尚未开始初始化的新空数据库执行原schema和day37—40，保留原SQL字节及checksum。失败状态留下核查依据，不自动再次初始化，不用删库解决。它不执行历史固定seed。

需要观察后台时，先准备仓库外权限受限的密码文件，再执行：

```powershell
./deploy/local.ps1 -Action CreateAdmin -Username demo_operator -PasswordFile '<仓库外受限密码文件>'
./deploy/local.ps1 -Action Server
```

管理员创建拒绝覆盖。Go服务为前台进程，Ctrl+C停止；随后`ServicesStop`停止自有存储，默认保留卷。`/health`仅证明进程，`http://127.0.0.1:18080/ready`检查实际依赖。

Web客户端仍分别在5173／5174运行；将`PVE_BACKEND_URL=http://127.0.0.1:18080`设在启动Vite的当前进程中，再执行各前端`npm ci --ignore-scripts`及`npm run dev`。不修改本机全局环境。

## 既有环境 Existing

该模式只允许Check和Server，不创建／停止存储、不初始化、不创建管理员、不生成或轮换秘密。ConfigPath为仓库外JSON，`schema_version=1`，`environment`中明确给出APP_HOST、APP_PORT、GAMEPLAY_MODE、DB_HOST、DB_PORT、DB_USER、DB_NAME、DB_PASSWORD_FILE、REDIS_ADDR、REDIS_DB、REDIS_PASSWORD_FILE、JWT_SECRET_FILE。密码项指向受限外部单行文件；明确无密码的旧Redis可将REDIS_PASSWORD_FILE置空。

```powershell
./deploy/local.ps1 -Profile Existing -Action Check -ConfigPath '<外部JSON>'
./deploy/local.ps1 -Profile Existing -Action Server -ConfigPath '<外部JSON>'
```

Check仅执行只读SQL、结构／账本checksum查询和RedisPING，不调用会写账本的v2_migrate。Server会正常执行Recover、Worker和业务写入，因此不是只读检查，必须先停止同库其他玩法服务。两模式都不能借用Dungeon Contract的专用数据库。

## 维护与故障排查

- 原密码不匹配：核对安装身份和原私密目录，不删除卷或重新生成密码。
- 端口冲突：核对占用者，不自动杀进程或更换端口。
- 迁移dirty／checksum不一致：保留故障，不修改已应用SQL或账本。
- 环境恢复：入口在结束后恢复调用方变量；不修改分页、WSL、VPN或防火墙。
- 内存不足：Windows启动依赖前至少2688MiB可用提交／1536MiB物理内存，Go前至少1792MiB／1024MiB。自有Go进程每秒检查1GiB提交／512MiB物理保留线，不足停止该进程树；Compose只做启动准入，不能据此宣称容器持续受保护。重任务串行，记录阻塞，不反复重跑同一未定位负载。

旧`deploy/docker-compose.yml`是显式既有环境模板，端口限制loopback、要求外部凭据；不会因为改了源模板就应用到原容器。R5及集成测试继续使用它们自己的独立项目、端口与合成数据，见[R5入口](../experiments/r5-m6/README.md)。

发布时保留固定R3实际回退、两套CI和原迁移；实际结果只归属目标提交。维护范围与限制见[当前收口](maintenance/README.md)。
