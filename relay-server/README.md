# EVE Assistant Server

Go 服务端，负责账户/设备安全通信、EVE SSO 与只读 ESI 同步、公共数据缓存、全区域市场采集、贸易规划和共享路线计算。

> 兼容性说明：产品和文档统一称为“服务端”（Server）。仅为保持代码目录与既有接口兼容，保留项目目录 `relay-server/`、启动路径 `cmd/relay-server` 和现有 API 路径；不改变 API 行为。

## Server API

- `GET /health`：健康检查
- `POST /api/v1/pair`：生成 5 分钟有效的一次性六位配对码（旧兼容入口）
- `POST /api/v1/pair/confirm`：消费配对码并返回设备凭证（旧兼容入口）
- `POST /api/v1/auth/device/start`：创建 5 分钟一次性设备授权挑战（不需要配对码）；默认拒绝客户端提供的任意 `provider/subject`。生产环境必须使用已认证 access token 提交匹配的 `accountId`；仅开发/测试可显式设置 `ALLOW_UNVERIFIED_DEVICE_AUTH=1`（或 `true`）开启匿名 identity bootstrap，严禁生产使用。
- `POST /api/v1/auth/device/complete`：消费挑战，按账号身份签发短期 access 与轮换 refresh token。错误响应统一为 `{ "error": { "code": "...", "message": "..." } }`，客户端应按 `code` 处理 `identity_required`、`invalid_challenge` 等安全错误。
- `POST /api/v1/auth/device/revoke`：撤销当前 access token 对应设备/会话
- `POST /api/v1/auth/token/refresh`：`/auth/refresh` 的规范别名
- `POST /api/v1/messages`：发布版本化消息，按 `id` 幂等
- `GET /api/v1/events`：**Deprecated** SSE 事件流（按设备 cursor 私有订阅；响应含 `Deprecation: true`，新客户端请使用 `/api/v1/realtime` 或 `/api/v1/sync`）
- `GET /api/v1/alerts`、`GET /api/v1/sync`：设备消息补偿同步
- `POST /api/v1/messages/ack`、`POST /api/v1/devices/revoke`：确认消息、撤销设备

以上路径保持现状，属于旧客户端兼容面；“Server/服务端”仅是产品概念和文案名称，不改变 API 行为。

### 设备授权请求示例

```http
POST /api/v1/auth/device/start
Authorization: Bearer <existing-access-token>
Content-Type: application/json

{"accountId":"acct_...","deviceName":"Alice-EVE mobile","deviceType":"mobile"}
```

未持有已验证会话时，服务端返回 `401 identity_required`；不要用用户密码、任意 subject 或 provider 绕过此检查。挑战只能完成一次且 5 分钟后失效。

## EVE SSO OAuth 基础能力

`internal/auth` 提供服务端侧可复用的 OAuth 2.0 Authorization Code + PKCE（S256）基础组件：

- `OAuthConfig` 配置授权、token、userinfo 端点、client ID、固定 HTTPS redirect URI 和最小 scope；机密客户端可使用部署侧注入的 ClientCredential，仅作为 token 端点 HTTP Basic 密码。
- `POST /api/v1/auth/sso/start` 接收客户端生成的 PKCE challenge，生成随机 state 并绑定固定 redirect/device 元数据；verifier 只由桌面端安全保存。
- `GET /api/v1/auth/sso/start` 是浏览器 flow：服务端生成 PKCE verifier/nonce，state 绑定服务端保存的 verifier、nonce 和 `__Host-eve-oauth` cookie；cookie 为 `Secure; HttpOnly; SameSite=Lax; Path=/`，回调成功或失败都会清除。
- `GET /api/v1/auth/sso/callback` 只接受匹配 state/cookie，失败统一返回 `401 oauth_invalid`；成功只重定向到配置的精确 `EVE_SSO_DEEP_LINK_URI`；未配置时显示不含凭证的完成页，不回跳 callback URI，且不把 code/state/token 放入 redirect。
- `POST /api/v1/auth/sso/callback` 一次性消费 state，使用 verifier 向 EVE token endpoint 换取短期上游 token，再向 userinfo/verify endpoint 获取最小 `subject`/display name，创建或查找本地 Account 并签发 Server access/refresh 与设备凭证。
- EVE access token 只短时驻留服务端内存；经授权的 refresh grant 使用服务端密钥加密持久化，用于后台只读同步。任何 EVE token 都不返回桌面前端、手机端或模型。state、PKCE、上游交换或 identity 失败使用统一错误边界，避免泄露验证细节。
- 旧 `/auth/oauth/*` 接口保留兼容，仅执行 state/PKCE 校验；生产新接入应使用 `/auth/sso/*`。

## 市场与共享路线 API

- `POST /api/v1/trade/candidates/search`：按预算、货舱、安全和收益约束查询全品类候选。
- `POST /api/v1/trade/plans/compose`：在固定起终点路线内组合多物品装载。
- `POST /api/v1/trade/plans/chain`：生成沿途取货/交付的链式计划。
- `GET /api/v1/trade/routes`：单条最短安全路线；无合规路线返回 404 `route_not_found`。
- `POST /api/v1/trade/routes`：最多 500 条批量路线，保持输入顺序；不可达项返回 `unavailable`。

路线模块位于 `internal/routeplanner`。它按 active SDE build 缓存不可变星门图，并使用包含 SDE version 和完整路线约束的有界结果缓存。旧的 PostgreSQL 递归路线查询已经移除。

市场数据由持久化调度器维护，分页采集完成后才原子发布 current/previous 快照；失败批次不可见。订单深度最多保留 32 档，用于加权成交价和货舱利用计算。

## 开发

安装 Go 后执行：

```bash
go test ./...
go run ./cmd/relay-server
```

When `DATABASE_URL` is configured, startup applies all checked-in migrations
before serving traffic. The runner records SHA-256 checksums in
`schema_migrations`, serializes runners with a transaction-scoped PostgreSQL
advisory lock, and rolls back/stops at the first failure. Set
`MIGRATIONS_MODE=dry-run` for a read-only plan or `MIGRATIONS_MODE=off` only
when migrations are run separately. The legacy `/api/v1/events` SSE endpoint
is deprecated and uses a per-device cursor subscription rather than a global
channel; reconnect with `?cursor=<last-id>` (or `Last-Event-ID`) and prefer
`/api/v1/realtime` for new clients.

账号、会话和 refresh token 基础已实现于 `internal/accounts`（详见 `../docs/server-accounts-sessions.md`），迁移为 `migrations/004_accounts_sessions.sql`。服务端签发的 access/refresh token 只以 SHA-256 哈希持久化；refresh 重放会撤销整个 token family，账号撤销会撤销账号下全部会话。现有旧 API 仍保持兼容，新的认证 handler 可按路线图逐步接入。

生产环境必须使用反向代理提供 TLS、PostgreSQL 持久化、最小权限账户、请求限流和受保护的密钥配置。公开仓库只保留脱敏示例；生产域名、地址、路径、数据库连接和密钥不得进入 Git。
