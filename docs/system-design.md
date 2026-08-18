> 状态：规划中
> 适用范围：项目实战 / 求职展示
> 最后更新：2026-07-12

# 工作实战版系统设计

## 总体架构

当前保持单体 Go 服务：

```text
React GM 后台 / 游戏 Demo / Apifox
        ↓
Gin HTTP API / WebSocket
        ↓
Handler / Middleware / Manager
        ↓
PostgreSQL / Redis
```

单体架构利于学习、调试、演示和面试讲解。二期再研究网关、战斗服拆分、微服务和多进程广播。

## 主要模块

- 认证模块：玩家和管理员登录、JWT 签发。
- 权限模块：玩家 token 和管理员 token 隔离。
- 玩家模块：玩家资料、列表、详情、封禁状态。
- GM 模块：玩家管理、操作日志、Dashboard。
- 在线模块：Redis 在线状态。
- WebSocket 模块：连接鉴权、连接 ID、心跳、消息协议。
- 小队模块：队长、成员、ready、状态广播。
- PVE 模块：匹配、任务副本、结算、排行榜。

## 前端展示层

GM 后台只做管理员视角。

游戏 Demo 只做玩家视角。

两者通过同一个后端数据互相印证。

## 数据流示例

```text
Demo 玩家创建小队
        ↓
WebSocket squad.create
        ↓
Go 后端更新小队状态
        ↓
广播 squad.state.changed
        ↓
Demo 成员看到变化
        ↓
GM 后台观察小队状态
```

## 安全边界

- 玩家 token 不能访问 `/api/admin/*`。
- 管理员 token 不能连接玩家 WebSocket。
- 密码只保存 bcrypt hash。
- 前端不展示 password_hash、JWT secret、数据库密码。
- 危险操作必须二次确认并记录 GM 操作日志。

## 当前不做

- 不为了前端展示改乱后端结构。
- 不提前引入复杂权限策略。
- 不提前拆微服务。
- 不提前引入 Protobuf、KCP 或 QUIC。
