> 状态：规划中
> 适用范围：项目实战 / 前端开发
> 最后更新：2026-07-12

# 前端共享 API 类型约定

## 目标

GM 后台和游戏 Demo 可以共享 API 类型、HTTP client、WebSocket 消息类型、错误码映射和时间格式化工具，避免重复定义同一套数据结构。

本文是前端类型约定，不替代后端的 `api-overview.md` 和 `ws-protocol.md`。

## 通用 HTTP 响应

```ts
export interface ApiResponse<T = unknown> {
  code: number
  message: string
  data?: T
}

export interface PageResult<T> {
  items: T[]
  page: number
  page_size: number
  total: number
}
```

## 核心类型

```ts
export interface Player {
  id: number
  username: string
  nickname: string
  status: 'normal' | 'banned' | string
  banned_reason?: string
  banned_at?: string
  banned_by_admin_id?: number
  created_at: string
  updated_at: string
}

export interface Admin {
  id: number
  username: string
  display_name?: string
  role: 'super_admin' | 'gm' | string
}

export interface OperationLog {
  id: number
  admin_id: number
  admin_username: string
  admin_role: string
  action: string
  target_type: string
  target_id?: number
  detail: string
  ip: string
  user_agent: string
  created_at: string
}
```

## 实时业务类型

```ts
export interface SquadMember {
  player_id: number
  username: string
  ready: boolean
  joined_at: string
}

export interface Squad {
  id: string
  leader_id: number
  max_members: number
  members: SquadMember[]
  created_at: string
  updated_at: string
  version?: number
}

export interface MissionInstance {
  id: string
  mission_id: string
  status: 'waiting' | 'ready' | 'running' | 'finished' | 'canceled' | string
  squad_id?: string
  started_at?: string
  finished_at?: string
}

export interface MatchmakingTicket {
  id: string
  mission_id: string
  player_id?: number
  squad_id?: string
  status: 'queued' | 'matched' | 'canceled' | 'timeout' | string
  created_at: string
  timeout_at?: string
}

export interface LeaderboardItem {
  player_id: number
  username: string
  nickname?: string
  score: number
  rank: number
  updated_at?: string
}
```

## WebSocket 类型

```ts
export interface ClientMessage<T = unknown> {
  type: string
  request_id?: string
  data?: T
}

export interface ServerMessage<T = unknown> {
  type: string
  request_id?: string
  code: number
  message: string
  data?: T
  server_time: string
}
```

## 错误码映射

前端不应只展示 `request failed`。常见错误应转换成人话：

| 类型 | 前端提示 |
| --- | --- |
| 401 | 登录过期，请重新登录 |
| 403 | 权限不足 |
| 409 | 当前状态不允许该操作 |
| 500 | 服务异常，请查看后端日志 |

## 当前不做

- 不使用前端类型替代后端校验。
- 不在前端保存 password、password_hash、JWT secret。
- 不把规划类型当成已实现接口事实。
