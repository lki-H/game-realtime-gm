# 测试与演示验收摘要

> 文档角色：面向展示的冒烟清单与证据边界
> 权威级别：L2（测试计划的展示视图）
> 状态：后端一期可验收；前端规划中
> 适用范围：本地演示、录屏和答辩式讲解
> 事实来源：`test-plan.md`、一期成果与前端规划
> 最后更新：2026-08-31

详细测试层级、用例 ID、缺陷和进出场标准以 [测试计划](test-plan.md) 为准。本文不重复全部测试，只定义演示前最小验收。

## 后端演示前检查

```powershell
cd .\backend
$goFiles = Get-ChildItem .\cmd,.\internal -Recurse -Filter *.go | Select-Object -ExpandProperty FullName
gofmt -d $goFiles
go test ./...
go vet ./...
```

检查 Docker MySQL/Redis、7 张表、健康检查和专用测试账号。Day34 已在 Linux Go 容器完成 `go test -race ./...`；结论只覆盖自动测试执行路径，不再写成“尚未完成”。

## 当前冒烟链路

- 玩家注册/重复注册、正确/错误/封禁登录。
- 玩家/管理员 token 隔离。
- 在线心跳和 WebSocket welcome、echo、断线/重连。
- 小队创建/加入/ready/离队和广播。
- 任务正常状态机与非法迁移。
- 匹配 enqueue/me/cancel/timeout。
- 首次结算、幂等重试、nonce/key 冲突和资产对照。
- Top N、个人排名、战绩分页。
- GM Dashboard、玩家管理、审计、实时摘要、玩家观察和结算查询。
- AccessLog 中不出现 WebSocket query token。

## 数据对照

| 结果 | 检查位置 |
| --- | --- |
| 账号、管理员、审计 | MySQL |
| 结算、奖励、余额、流水 | MySQL，同事务一致性 |
| 在线、匹配、排行榜 | Redis |
| 连接、小队、任务会话 | 当前 Go 进程/GM 观察 |

演示中必须解释：Redis 排行是投影，内存状态重启清空，GM 聚合不是原子快照。

## 未来 GM 后台

规划验收：登录、Dashboard、玩家管理、危险操作二次确认、审计、实时观察、错误/空/加载状态和 token 失效。当前没有 React 页面，不能勾选为完成。

## 未来 Unity Demo

规划验收：登录、WebSocket、小队、任务、结算、排行、协议错误和重连查询。当前协议没有位置或战斗消息；不能演示商业级状态同步。

## 证据边界

- 自动测试和 race：E2。
- Day34 本机短时 `ws_bot`/pprof：E3。
- 未来本地故障、备份、发布/回滚：最多 E4。
- 当前没有 E5 真实生产用户、上线、值班或事故经验。

## 失败即停止展示

- 核心测试失败或出现数据不一致。
- 使用了真实密码/token/个人信息。
- 需要手工改 MySQL 余额才能完成闭环。
- 文档、OpenAPI、WS 消息与代码不一致。
- 将规划中的 React/Unity/微服务/云部署描述为已实现。
