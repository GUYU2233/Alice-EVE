# Server Contract（规范字段与兼容策略）

本文件是服务端产品化后的线协议补充，和 `shared/protocol/*.schema.json`、`openapi.yaml` 一起维护。服务端从“中继消息”演进为多端后端，但客户端仍可通过同一套版本化 Envelope 交换事件和 Agent 会话帧。

## 1. 规范 Envelope

新客户端生成以下长字段；服务端在兼容窗口内同时接受当前 Alpha 的短字段：

```json
{
  "version": 1,
  "messageId": "01J...ULID",
  "correlationId": "01J...ULID",
  "conversationId": "01J...ULID",
  "sender": "desktop",
  "recipient": "mobile:device-123",
  "type": "intel.alert",
  "createdAt": "2026-08-23T10:20:00Z",
  "expiresAt": "2026-08-23T10:30:00Z",
  "payload": {},
  "meta": {"requestId": "req-123"}
}
```

### 必填和限制

- `version` 当前为 `1`，未知版本返回 `INVALID_ENVELOPE`。
- `messageId` 在发送设备范围内唯一；数据库幂等键为 `(senderDeviceId, messageId)`，不能只依赖全局 `id`。
- `sender` 是服务端从 access token 推导后的身份，客户端声明值只用于校验，不用于授权。允许 `desktop`、`mobile`、`server`；Alpha 的 `relay` 读取兼容。
- `recipient` 必须是设备/会话地址，不能接受客户端提供的任意账号 ID 作为绕过授权的依据。
- `createdAt` 使用 RFC3339，服务端允许 ±5 分钟偏差；`expiresAt` 过期后不投递。
- `payload` 必须匹配 `type` 对应 Schema；单 Envelope 最大 256 KiB，JSON 深度、键数、数组长度均受限。
- `meta` 只允许 request/correlation/client 追踪信息，不得包含 `Authorization`、access/refresh token、私钥、原始 EVE SSO 响应或敏感原文。

### Alpha 字段映射

| Alpha | 规范 | 处理策略 |
|---|---|---|
| `v` | `version` | 读取两者；冲突拒绝；兼容响应按请求字段风格返回 |
| `id` | `messageId` | 同上 |
| `ts` | `createdAt` | 同上 |
| 无 | `recipient` | 新消息必须显式目标或使用服务端授权广播 |
| `relay` | `server` | 读取兼容；新响应使用 `server` |

兼容窗口只覆盖 `/api/v1/messages`、`/api/v1/sync` 和旧 `/api/v1/events` SSE；新账号、设置、更新、会话 API 只使用规范长字段。删除短字段前，先通过 capability response 发布截止版本，并在至少一个客户端版本中停止生成短字段。

## 2. HTTP API 约定

认证接口以外的请求带 `Authorization: Bearer <short-lived-access-token>`。服务端从 token 得到 `accountId`、`deviceId`、设备类型和 capability；不信任 `X-Device-Id`、query `deviceId` 或 body 中的 owner 字段。所有响应带 `X-Request-Id`，客户端重试写操作必须带 `Idempotency-Key`。

当前统一错误外层为：

```json
{
  "error": {
    "code": "settings_conflict",
    "message": "settings version is stale",
    "requestId": "req-..."
  }
}
```

更多 `details`/retryability 只能在端点契约明确后添加，客户端不得依赖尚未实现的平铺字段。

`message` 面向客户端，不得泄漏 SQL、堆栈、内部路径、token 或原始私密数据。HTTP 映射：`400 INVALID_*`、`401 AUTH_REQUIRED/DEVICE_REVOKED`、`403 FORBIDDEN`、`404 NOT_FOUND`、`409 CONFLICT/DUPLICATE`、`413 MESSAGE_TOO_LARGE`、`429 RATE_LIMITED`、`503 UPSTREAM_UNAVAILABLE`。

## 3. 消息状态和同步

服务端写入 outbox 后才返回 `persisted`。ACK 是幂等状态推进，不是无条件删除：

```json
{
  "ackMessageId": "01J...",
  "status": "accepted",
  "stage": "received",
  "receivedAt": "2026-08-23T10:20:01Z"
}
```

`stage` 顺序为 `received -> persisted -> delivered -> processed`，允许重复提交同一阶段，不允许回退。当前兼容入口是 `POST /api/v1/messages/ack`；`/messages/{id}/ack` 是目标形态。ACK 必须验证提交方是目标设备，服务端按 `expiresAt` 和账号 retention policy 清理。

目标补偿同步形态如下；当前实现入口仍是 `/api/v1/sync`：

```text
GET /api/v1/devices/{deviceId}/sync?after=<cursor>&limit=100
-> {"items": [...], "nextCursor": 123, "hasMore": false}
```

`after` 是目标设备维度的单调 cursor；WSS 重连顺序固定为“先 sync，后 subscribe”。旧 `/api/v1/sync` 保留别名，返回 `messages/cursor` 直到兼容窗口结束。实时订阅必须按 token 推导的设备隔离连接队列，每个连接独立背压，不能使用一个全局消费 channel。

## 4. 领域消息类型

- 设备：`device.registered`、`device.revoked`、`presence.update`
- 摘要事件：`intel.alert`、`intel.summary`、`market.alert`、`desktop.status`
- 同步/控制：`sync.request`、`sync.result`、`ack`、`error`、`ping`、`pong`
- Agent：`conversation.created`、`conversation.message.created`、`agent.run.started`、`agent.run.delta`、`agent.run.completed`、`agent.run.failed`、`agent.run.cancelled`
- 设置：`settings.updated`、`settings.conflict`
- 更新：`update.available`、`update.policy.changed`

消息 schema 必须按 type 分文件或 `$defs` 命名，不能使用一个允许任意对象的 `payload` 代替校验。Agent delta 默认不落长期存储；最终消息、状态和审计结果按 retention policy 保存。

## 5. 安全边界

1. 客户端只保存 Alice 会话凭据，不接收 EVE token。服务端 OAuth 组件保存短期 state/challenge 和最小身份 subject；经授权的 EVE refresh grant 使用部署 keyring 加密持久化，access token 仅短时驻留内存。
2. 手机端不得提交 EVE 凭据；手机请求桌面 Agent 只能使用固定 operation 白名单和服务端签发的 conversation 权限。
3. 配对 challenge 绑定 `accountId + creatorDeviceId + targetType`，五分钟一次性、最多五次尝试；成功、过期和撤销都立即失效。
4. 设置文档只允许白名单键和受限 JSON；不接受 token、私钥、任意命令、SQL、URL 或原始私密内容。
5. 更新下载使用 CDN；客户端验证 manifest 签名与 sha256，服务端不接受客户端上传的二进制作为更新来源。
6. 审计记录只记 account/device/request/type/status/size/duration；日志禁止记录 Authorization 和 payload 原文。

## 6. 兼容性测试清单

- Go、Flutter、桌面端分别解析长字段 Envelope，并能读取 `v/id/ts` Alpha fixture。
- 长短字段同时出现且值冲突时，三端都拒绝，不出现“最后一个字段覆盖”差异。
- 同一 `(senderDeviceId,messageId)` 重试只创建一条消息并返回相同 ACK。
- 两个 SSE/WSS 设备同时在线时，各自只能收到自己的目标消息；断线后 sync 能补齐。
- 设置 stale version 得到 409；两个设备并发更新不互相静默覆盖。
- 同一 release manifest 在 Go、Flutter、桌面端校验摘要/签名结果一致。
- 账号撤销会话或设备后，所有 API、WSS、pending outbox 均拒绝或停止投递。
