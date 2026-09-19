# 桌面端、服务端与手机端通信协议

> 术语与兼容性：本文将产品和文档统一称为“服务端”（Server）。仅为保持协议兼容，协议线字段 `sender: "relay"` 和现有 API/实现标识继续保留；不改变任何 API 路径、字段或协议行为。

## 1. 范围与拓扑

```text
Mobile App ── HTTPS/WSS ──> Server <── HTTPS/WSS ── Desktop App
```

桌面端只建立出站 WSS；服务端不反向连接桌面端。Alice 会话令牌保存在客户端安全存储；EVE access token 只短时驻留服务端内存，经授权的 EVE refresh grant 由服务端加密持久化以支持后台只读同步。EVE token 不返回前端、手机端或模型。

通信分为：

- HTTPS REST：登录、设备管理、配对、历史事件、设置
- WSS：实时事件、请求、ACK、presence
- FCM/APNs：手机后台推送，仅传递摘要和事件 ID

## 2. JSON Envelope

版本化 Envelope 用于旧 `/api/v1/messages`、兼容同步/事件；账号、设备、EVE、交易、通知和会话 REST API 使用端点 DTO，WSS 使用独立版本化帧：

```json
{
  "version": 1,
  "messageId": "01J...ULID",
  "correlationId": "01J...ULID",
  "conversationId": null,
  "sender": "desktop",
  "recipient": "mobile:device-123",
  "type": "intel.alert",
  "createdAt": "2026-08-23T10:20:00Z",
  "expiresAt": null,
  "payload": {},
  "meta": {"requestId": "req-..."}
}
```

字段规则：

| 字段 | 类型 | 规则 |
|---|---|---|
| `version` | integer | 当前为 `1`；未知版本拒绝 |
| `messageId` | string | ULID/UUID；同一发送方范围内唯一 |
| `correlationId` | string/null | 请求与响应、重试关联；事件可为空 |
| `sender` | enum | `desktop`、`mobile`、`relay`（协议兼容值） |
| `recipient` | string/null | 设备地址；广播为 null，由权限决定 |
| `type` | string | 版本化消息类型 |
| `createdAt` | RFC3339 | 允许时钟偏差 ±5 分钟 |
| `expiresAt` | RFC3339/null | 过期消息不得投递 |
| `payload` | object | 必须通过对应 JSON Schema |
| `meta` | object | 不得放 Token、Authorization 或私密原文 |

当前全局 HTTP 请求体上限为 1 MiB、WSS 入站帧为 512 KiB；各端点可设置更小上限（认证接口为 4–8 KiB）。256 KiB 仍可作为 Envelope 目标上限，但需先统一实现和 Schema。所有字符串、数组深度和字段数量均需限制。

## 3. 消息类型

### 设备与配对

- `device.register`：注册设备公钥和能力
- `device.revoke`：撤销设备
- `pairing.create`：创建短期配对码
- `pairing.confirm`：确认并绑定另一设备
- `presence.update`：在线、版本、最后心跳

### 事件与同步

- `intel.alert`：情报告警
- `intel.summary`：情报摘要
- `market.alert`：市场告警
- `desktop.status`：桌面核心状态
- `sync.request`：请求增量事件/状态
- `sync.result`：增量结果

### 请求与协议控制

- `query.request`：手机向桌面发起有限只读请求
- `query.result`：只读请求结果
- `ack`：确认收到/持久化/处理
- `error`：失败响应
- `ping` / `pong`：心跳（可使用 WSS 控制帧）

### 事件 payload 示例

```json
{
  "eventId": "evt-123",
  "severity": "high",
  "title": "2 跳发现敌对情报",
  "summary": "过去 5 分钟内有 3 名敌对飞行员经过",
  "systemId": 30000142,
  "systemName": "Example",
  "source": "chat-log",
  "observedAt": "2026-08-23T10:18:00Z",
  "expiresAt": "2026-08-23T10:30:00Z",
  "evidenceCount": 2
}
```

默认只传摘要、实体 ID/名称、来源和时间；原始聊天、邮件、钱包、资产明细必须显式授权才可传输。

## 4. 设备授权

当前规范设备接入使用 EVE SSO，或由已认证账号调用 `/api/v1/auth/device/start` 并一次性消费 5 分钟 challenge；完成后签发短期 access token、轮换 refresh token 和兼容设备凭据。

旧 `/api/v1/pair` 与 `/api/v1/pair/confirm` 是 deprecated 兼容路径，不具备账号绑定、设备签名或尝试次数上限，不得作为账号安全边界。目标状态仍是账号绑定、单次使用、短时有效、限尝试和设备密钥签名；在旧接口下线前必须保留明确的兼容风险说明。

## 5. ACK、重试与 Outbox

`ack` payload：

```json
{
  "ackMessageId": "01J...ULID",
  "status": "accepted",
  "stage": "persisted",
  "receivedAt": "2026-08-23T10:20:01Z"
}
```

`stage`：`received`、`persisted`、`delivered`、`processed`。桌面端必须先写本地 Outbox，再发送；收到服务端 `persisted` ACK 后才可标记服务端已持久化。手机收到事件后返回 `received`，用户确认后可返回 `processed`。

建议 Outbox 状态：`pending → sent → server_acked → delivered → processed`；失败可回到 `pending`。重试使用指数退避（1s、2s、4s、8s、最多 5 分钟）和随机抖动；过期事件不重试。服务器离线事件保留 7 天或达到用户配额即停止保存。

## 6. 幂等与顺序

幂等键为：`(senderDeviceId, messageId)`。服务端对重复消息返回同一 ACK，不重复创建事件、推送或执行查询。事件业务去重可额外使用 `eventId`。

WSS 断线后客户端发送 `sync.request`，带 `lastAckedMessageId` 或服务器 cursor；服务端返回缺失事件，客户端按 `messageId` 幂等应用。不存在全局顺序保证；同一设备到同一设备的事件按服务器 cursor 排序。告警显示时间以 `createdAt` 为主、`observedAt` 为事实时间。

## 7. 错误模型

```json
{
  "code": "AUTH_REQUIRED",
  "message": "设备凭证已过期",
  "retryable": false,
  "correlationId": "01J...ULID",
  "details": {"reauthRequired": true}
}
```

标准错误码：

- `INVALID_ENVELOPE`：版本、字段或 JSON 无效
- `INVALID_PAYLOAD`：消息类型 Schema 不通过
- `MESSAGE_TOO_LARGE`
- `AUTH_REQUIRED`、`FORBIDDEN`
- `DEVICE_REVOKED`、`PAIRING_EXPIRED`
- `DUPLICATE_MESSAGE`（可作为成功 ACK 的说明）
- `RATE_LIMITED`（带 retry-after）
- `NOT_FOUND`
- `MESSAGE_EXPIRED`
- `DESKTOP_OFFLINE`
- `UPSTREAM_UNAVAILABLE`
- `INTERNAL_ERROR`

HTTP 映射：400/401/403/404/409/413/429/500/503。错误响应不得包含堆栈、Token、内部路径或原始私密数据。

## 8. 权限模型

设备权限按能力最小化：

| 能力 | Desktop | Mobile |
|---|---:|---:|
| 发布告警摘要 | ✓ | - |
| 接收告警 | ✓ | ✓ |
| 查看最近摘要 | ✓ | ✓ |
| 修改提醒规则 | ✓ | ✓（受限） |
| 读取角色私有数据 | 本地 | 只能请求摘要 |
| 访问 EVE Token | 本地安全存储 | 禁止 |
| 远程查询桌面 | 接收 | 仅白名单只读 |
| 游戏控制/自动操作 | 禁止 | 禁止 |

`query.request` 只能使用固定操作名，例如 `get_desktop_status`、`get_recent_intel`、`get_route_summary`、`get_character_summary`；禁止传 SQL、任意 URL、任意命令或任意 ESI endpoint。每个操作再次执行用户、设备、角色和 scope 检查。

## 9. 安全要求

- 仅 HTTPS/WSS；生产环境 TLS 终止于反向代理
- access token 短期有效；refresh token 可撤销、轮换并绑定设备
- 配对码一次性、短期、限尝试
- WSS 心跳、连接数和消息速率限流
- 时间窗口、nonce/messageId 防重放
- 服务端日志只记录 requestId、设备 ID、类型、状态、大小和耗时
- 推送正文只放非敏感摘要；敏感详情在 App 鉴权后拉取
- 所有设备支持撤销；账号支持撤销全部设备
- 原始 Chatlogs/Gamelogs 不上传服务端；EVE 凭证不得进入日志或模型上下文。服务端仅可为已授权账号加密保存可撤销的 EVE refresh grant，并按 scope 最小化同步私有数据
- 数据保留、删除、导出和隐私设置可配置

## 10. 协议版本与共享 Schema

协议源文件建议维护在：

```text
shared/protocol/
├── envelope.schema.json
├── messages.schema.json
├── errors.schema.json
├── pairing.schema.json
├── openapi.yaml
├── README.md
├── generated/
│   ├── go/       # json tags + validation DTO
│   ├── typescript/
│   └── dart/
└── examples/
```

当前 Schema/OpenAPI 仍是兼容草案：旧消息使用短字段 Envelope，多个业务 API 使用手写端点 DTO。完成实际路由/字段对齐和跨语言生成前，不能将其称为唯一真源。新增字段仍应保持兼容；破坏性变更需提升协议主版本。

## 11. 传输选择

- 桌面↔服务端：WSS 长连接，HTTPS 补偿同步
- 手机↔服务端：WSS 前台实时，HTTPS 历史/设置，FCM/APNs 后台推送
- 手机↔桌面：逻辑上经过服务端，不建立直连
- 桌面离线：手机只能查看服务器已保存的有限事件，不能访问本地数据

## 12. MVP 约束

当前已包含账号会话、设备授权/撤销、兼容消息与同步、账号/设备隔离 WSS、Agent conversations、通知偏好/令牌、EVE SSO/ESI 同步和交易 API。旧配对/SSE/短字段 Envelope 仍是兼容面；FCM 生产 source/resolver 与 APNs dispatcher 尚未接线，且始终禁止自动游戏操作。
