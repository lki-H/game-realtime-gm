# 展示客户端类型映射约定

> 文档角色：TypeScript 与 Unity C# 对当前契约的映射规则
> 权威级别：L2（跨语言约定，字段以契约为准）
> 状态：React与Unity已实现；本文维护跨语言约定，客户端仍以实际契约为准
> 适用范围：React GM/PVE 与 Unity Demo
> 事实来源：`openapi.yaml` 与 `ws-protocol.md`
> 最后更新：2026-10-10

## 1. 唯一字段来源

- HTTP schema：[OpenAPI](openapi.yaml)。
- WebSocket envelope/message：[WS 协议](ws-protocol.md)。
- 本文只说明映射，不复制所有 DTO；出现冲突时以前两者和当前代码为准。

## 2. 通用映射

| JSON/OpenAPI | TypeScript | C# |
| --- | --- | --- |
| `string` | `string` | `string` |
| `integer int64` | `number`（当前值可安全表达时） | `long` |
| `boolean` | `boolean` | `bool` |
| RFC3339 date-time | `string`，边界处解析 | `DateTimeOffset` |
| 可省略字段 | `field?: T` | nullable/reference + 序列化配置 |
| JSON `null` | `T | null` | `T?` |
| string enum | union/enum | enum + 明确 JSON 字符串映射 |

JavaScript `number` 对大于 `2^53-1` 的 int64 不安全。当前自增 ID 规模较小；若协议进入大规模或外部系统，应将 int64 ID 统一改为 JSON string 或使用显式转换策略。

## 3. HTTP envelope

TypeScript：

```ts
export interface ApiResponse<T> {
  code: number;
  message: string;
  data?: T;
}

export interface Page<T> {
  items: T[];
  page: number;
  page_size: number;
  total: number;
}
```

C#：

```csharp
public sealed class ApiResponse<T>
{
    public int Code { get; init; }
    public string Message { get; init; } = string.Empty;
    public T? Data { get; init; }
}
```

客户端必须同时处理 HTTP 状态和业务 `code`。不能只以 HTTP 200 判断成功。

## 4. 身份类型

- 玩家登录 data：`token` + `player`。
- 管理员登录 data：`token` + `admin`。
- 两种 token 使用独立客户端状态和请求封装，避免误加到错误接口。
- token 不写入日志、异常详情、截图或可提交配置。

## 5. WebSocket envelope

TypeScript：

```ts
export interface ClientMessage<T> {
  type: string;
  request_id?: string;
  data?: T;
}

export interface ServerMessage<T> {
  type: string;
  request_id?: string;
  code: number;
  message: string;
  data?: T;
  server_time: string;
}
```

C# DTO 应使用序列化属性映射 snake_case；不要依赖字段名自动转换的隐式默认。广播 `request_id` 可能缺失，客户端按 `type` 分发并允许未知消息进入安全日志/忽略策略。

## 6. 时间、枚举和可选字段

- 所有 date-time 以服务端 RFC3339 为准，显示时再转本地时区。
- 玩家状态：`normal | banned`。
- 任务状态：`waiting | ready | running | finished | canceled`。
- 匹配状态：`queued | canceled | timeout`，当前没有 `matched`。
- `banned_at`、`banned_by_admin_id`、`squad`、`mission`、`matchmaking`、`latest_settlement` 等可能省略或为 null，UI 必须有空状态。
- 当前 WS 没有 `version` 字段，客户端不能假设存在。

## 7. 契约兼容验证

未来实现客户端时至少维护：

1. HTTP/WS JSON golden fixture。
2. TypeScript 和 C# 反序列化测试。
3. enum 未知值策略。
4. 缺失可选字段和 null 测试。
5. int64、时间和 snake_case 映射测试。
6. 路由/消息改动时同步契约并在两端回归。

当前不锁定 OpenAPI 代码生成器、Unity JSON/WebSocket 第三方库或共享 DTO 仓库；选型前先做最小 fixture 实验。
