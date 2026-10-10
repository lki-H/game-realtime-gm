# 测试与演示验收摘要

> 文档角色：面向展示的冒烟清单与证据边界
> 权威级别：L2（测试计划的展示视图）
> 状态：V2 后端、React GM/PVE 与 Unity Windows 可本机验收；跨机器延期
> 适用范围：本地演示、录屏和答辩式讲解
> 事实来源：`test-plan.md`、一期成果与前端规划
> 最后更新：2026-10-10

详细测试层级、用例 ID、缺陷和进出场标准以 [测试计划](test-plan.md) 为准。本文不重复全部测试，只定义演示前最小验收。

## 后端演示前检查

```powershell
cd .\backend
$goFiles = Get-ChildItem .\cmd,.\internal -Recurse -Filter *.go | Select-Object -ExpandProperty FullName
gofmt -d $goFiles
go test ./...
go vet ./...
```

检查隔离 MySQL/Redis、编号迁移账本、活性/就绪和合成账号。普通测试默认跳过存储集成；真实存储必须显式运行 `PVE_INTEGRATION=1`，Linux race 结果也只覆盖实际执行的路径。

## 当前冒烟链路

- 玩家注册/重复注册、正确/错误/封禁登录。
- 玩家/管理员 token 隔离。
- V2 WebSocket、连接代次、断线/重连、慢消费者和资源预算。
- Party 准备/名单/方案版本、招募来源隔离和自愿续组。
- 不同/可选/不兼容任务、共享贡献、增援竞争和统一结束。
- 整组票据、逐人确认、取消/超时竞态和加载回退。
- 可信测试事件重复/乱序/缺口/旧来源、逐人幂等结算与对账。
- GM 六类只读观察及 operator 修复；玩家管理/审计 API。
- AccessLog 中不出现 WebSocket query token。

## 数据对照

| 结果 | 检查位置 |
| --- | --- |
| 账号、管理员、审计 | MySQL |
| 结算、奖励、余额、流水 | MySQL，同事务一致性 |
| 匹配、Party、Run、个人任务 | MySQL 事实 |
| 在线、匹配队列、奖励展示/历史排行榜 | Redis 投影 |
| 当前连接/发送队列 | 当前 Go 进程，查询恢复依赖持久化事实 |

演示中必须解释：Redis 是投影，连接是内存状态，Party/Run/任务是 MySQL 事实；演示 Run 在进程重启后按技术中止规则恢复结算，不虚构战斗继续。

## 当前 GM 后台

React 页面已支持登录、V2 Party/Proposal/Run/任务/pending/outbox、分页/刷新、失效退出和 operator 修复确认。Dashboard/玩家管理/审计后端存在，完整 React 管理 UI 是后续补充。

## 当前 Unity Demo

Windows 控制面已验证真实四客户端单排/混排、登录、任务、匹配、重连和结果查询。当前没有真实战斗同步，Editor 场景、跨机器和真实 DS 另行验收。

## 证据边界

- 自动测试和 race：E2。
- Day34 本机短时 `ws_bot`/pprof：E3。
- 本机故障、备份恢复、已发布 CI 与实际二进制回滚：受控环境 E4。
- 当前没有 E5 真实生产用户、上线、值班或事故经验。

## 失败即停止展示

- 核心测试失败或出现数据不一致。
- 使用了真实密码/token/个人信息。
- 需要手工改 MySQL 余额才能完成闭环。
- 文档、OpenAPI、WS 消息与代码不一致。
- 将规划中的 React/Unity/微服务/云部署描述为已实现。
