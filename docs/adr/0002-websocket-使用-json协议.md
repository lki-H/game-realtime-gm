> 状态：已确定
> 适用范围：项目实战 / 前端开发
> 最后更新：2026-07-12

# 0002：WebSocket 使用 JSON 协议

## 背景

项目已经使用 WebSocket 承载玩家长连接、小队消息和后续任务实时消息。

## 决策

当前阶段继续使用 JSON 消息协议：

```json
{
  "type": "squad.create",
  "request_id": "req-001",
  "data": {}
}
```

## 原因

- JSON 便于 Apifox、浏览器和 Demo 调试。
- 初学阶段更容易解释和排错。
- 前端和后端都能快速迭代。
- 小队、匹配、任务、结算闭环完成前，不引入额外编码复杂度。

## 暂不采用

- Protobuf 全量替换。
- Socket.IO。
- Colyseus SDK。
- KCP、QUIC、自研 TCP。

## 影响

后续 WebSocket 消息必须同步维护 `docs/ws-protocol.md`。二期如研究 Protobuf，应单独作为实验，不直接推翻主项目协议。
