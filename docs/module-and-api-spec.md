> 状态：随实现更新
> 适用范围：项目实战 / 求职展示 / 前端开发
> 最后更新：2026-07-12

# 功能模块与接口说明

## 文档关系

HTTP 接口事实以 `docs/api-overview.md` 为准。

WebSocket 协议事实以 `docs/ws-protocol.md` 为准。

本文只按模块说明接口用途，不复制完整请求响应。

## 认证模块

- 玩家注册。
- 玩家登录。
- 管理员登录。
- JWT 签发。
- 玩家 token 和管理员 token 隔离。

## 玩家模块

- 查询当前玩家。
- 修改昵称。
- 玩家列表。
- 玩家详情。
- 被封禁玩家禁止登录。

## GM 模块

- 管理员当前信息。
- Dashboard 汇总。
- GM 玩家列表。
- GM 玩家详情。
- 封禁玩家。
- 解封玩家。
- 操作日志筛选。
- 操作日志详情。

## 在线状态模块

- HTTP 心跳。
- Redis 在线状态。
- WebSocket 在线状态续期。

## WebSocket 模块

- 玩家 token 鉴权。
- `server.welcome`。
- ping/pong 心跳。
- 单条消息大小限制。
- 统一 JSON 消息协议。
- 小队消息。
- 后续任务和位置同步消息。

## GM 后台接入原则

GM 后台优先接入已有 HTTP 管理接口。没有后端审计的危险操作，前端先不做。

## 游戏 Demo 接入原则

Demo 每一步都应对应一个后端能力：

```text
登录 -> JWT
建连 -> WebSocket 鉴权
创建小队 -> squad.create
准备 -> squad.ready
任务 -> mission instance
结算 -> settlement
排行榜 -> Redis ZSet
```

## 当前不做

- 不在本文维护完整 API 示例。
- 不把规划接口写成已实现接口。
- 不新增脱离 PVE 主线的接口。
