> 状态：规划中
> 适用范围：项目实战 / 求职展示 / 前端开发
> 最后更新：2026-07-12

# GM 后台与游戏 Demo 前端规划

## 总原则

GM 后台和游戏 Demo 都是展示层。它们只接入当前已实现或 Day27-Day35 明确规划的后端能力，不反向定义后端需求。

接口字段以 `docs/frontend-api-types.md` 为准，HTTP API 以 `docs/api-overview.md` 为准，WebSocket 消息以 `docs/ws-protocol.md` 为准。本文不作为接口事实来源。

```text
GM 后台：管理员视角，观察、管理、审计。
游戏 Demo：玩家视角，验证登录、小队、任务、实时同步。
```

## 推荐技术栈

GM 后台：

```text
React + Vite + TypeScript
Ant Design
React Router
Axios
TanStack Query 或轻量请求 hooks
Zustand / localStorage 管理本地登录态
ECharts 或 Ant Design Charts 用于少量统计图
```

游戏 Demo：

```text
React + Vite + TypeScript
Phaser
原生 WebSocket
少量 Zustand 或 React state
```

不建议在当前阶段引入 Unity、Cocos、完整低代码后台、复杂 RBAC 框架或额外实时服务框架。

## 目录建议

```text
frontend/
├─ gm-admin/
└─ game-demo/
```

两个前端项目可以共享类型、请求工具、错误码映射和时间格式化，但不要强行共享大量 UI 组件。GM 后台和游戏 Demo 的用户目标不同。

## 环境变量

GM 后台：

```env
VITE_API_BASE_URL=http://localhost:8080
```

游戏 Demo：

```env
VITE_API_BASE_URL=http://localhost:8080
VITE_WS_URL=ws://localhost:8080/ws
```

后续应提供：

```text
frontend/gm-admin/.env.example
frontend/game-demo/.env.example
```

## GM 后台阶段

### V1 必做

- 管理员登录。
- Dashboard 汇总。
- 玩家分页查询、搜索、详情。
- 封禁和解封。
- 危险操作二次确认和原因输入。
- 操作日志查询。
- loading、empty、error、token expired、retry 状态。

### V2 加分

- 在线玩家观察。
- WebSocket 连接观察。
- 小队房间观察。
- 匹配队列观察。
- 任务副本观察。
- 结算记录和排行榜。

### V3 暂缓

- 复杂 RBAC。
- 菜单动态配置。
- 公告系统。
- 充值订单。
- 活动配置。
- 多租户和多大区管理。

## 游戏 Demo 阶段

### V1 必做

- 玩家登录。
- WebSocket 建连。
- 显示 `connection_id`。
- 创建小队、加入小队、离开小队、切换 ready。
- 展示小队成员、队长和准备状态。
- 显示最近 WebSocket 消息。
- 错误消息可见。

### V2 加分

- 任务开始。
- 任务状态流转。
- 任务完成。
- 结算展示。
- 排行榜变化。

### V3 暂缓

- Phaser 小地图。
- WASD 移动。
- 位置同步。
- 目标区域完成任务。
- 观战视角。

## 角色边界

GM 后台不做玩家移动、技能操作、普通玩家玩法 UI。

游戏 Demo 不做管理员封禁、操作日志管理、全局玩家列表和全局任务管理。

两者唯一交汇点是同一个后端数据被两个视角看到。

## Debug 与展示模式

开发调试视图可以显示：

- `player_id`
- `admin_id`
- `connection_id`
- `squad_id`
- `mission_id`
- `request_id`
- 最近 WebSocket 消息
- 当前 API Base URL 和 WS URL

正式展示视图应隐藏 token、长 JSON 和内部字段，只展示昵称、小队、任务、结算、排行榜和操作日志等业务信息。

## Token 存储边界

本地学习演示阶段可以使用 localStorage。文档和面试表达中必须说明：

```text
当前为本地学习演示使用 localStorage。
生产环境应结合 HttpOnly Cookie、CSRF 防护、短期 access token、刷新 token 和服务端强失效策略。
```

## 当前不做

- 不为了页面效果临时改乱后端主线。
- 不做完整游戏官网。
- 不做炫技大屏。
- 不把 Demo 做成独立完整游戏客户端。
