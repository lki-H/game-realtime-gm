# PVE 网页控制面

本地验证器，玩家登录与HTTP/WS控制面使用真实Go V2后端；没有任务事件token、finish、伤害自报或奖励写入口。

1. 对明确数据库执行 `go run ./cmd/tools/v2_migrate -stage r4`，同一账本顺序检查day37—40；已有数据先备份恢复演练。
2. 从backend启动V2 Go服务，默认 `APP_PORT=8080`、`GAMEPLAY_MODE=v2`，其他DB/Redis/JWT使用本地独立环境。
3. 在本目录执行 `npm ci --ignore-scripts`、`npm run dev`，打开 `http://127.0.0.1:5174/`。

开发代理默认指向本机8080，WS只接受本网页127.0.0.1/localhost的5174端口Origin后转到后端Origin；不开放任意来源。登录token只在内存，不写localStorage；刷新页面需要重新登录，然后通过活动恢复查询回到原房间/Run/招募/续组。测试账号在隔离库注册，不把真实密码写入README。

后端使用其他本机端口时，启动网页前设置 `PVE_BACKEND_URL`，例如 PowerShell 的 `$env:PVE_BACKEND_URL='http://127.0.0.1:18082'`。GM与PVE网页支持同一配置；配置只在开发服务端使用，不暴露凭据或改变WS来源限制。

四人流程：A建房邀请B、应用v2方案；C通过招募申请/被接受/独立任务和准备，D无个人任务单排；全员准备与候选确认；受限CLI提供局内事件；查询结果后逐人自愿续组。失败操作、候选拒绝、不兼容任务/停用、断线和过期申请均有明确业务反馈。最后停止Vite与测试Go进程，清理专用测试环境。

`npm run build` 包含TypeScript检查；Unity Editor与本机四客户端已验证，跨机器和商业容量仍延期。端口代理只是本机开发配置，不代表线上发布。
