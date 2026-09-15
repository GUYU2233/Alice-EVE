# Alice-EVE 服务端路线图（Server 产品化）

> 目录和旧入口 `relay-server` / `cmd/relay-server` 暂保留，以确保已有部署、脚本和客户端兼容；新文案统一使用“服务端”。

> 本文是基于当前工作区代码的审阅和可执行拆分建议，不是一次性重写计划。服务端仍可暂时保留 `relay-server` 仓库目录和旧路径，但产品定位应改为 **Alice-EVE Server**：负责账号、设备、应用版本、设置同步，以及手机端与桌面端 Agent 的安全会话。

## 1. 当前审阅结论

### 1.1 现有边界

当前服务端只有一个 `relay-server/internal/api/server.go`，同时承担：路由注册、OAuth 状态流程、配对码生成/消费、设备认证、消息校验、幂等、消息持久化、同步查询、SSE 推送和撤销设备。它已接近 400 行，且每增加一个领域都会继续修改同一个文件。

持久化接口 `relay-server/internal/store/store.go` 也是一个横跨消息、配对、设备和认证的总接口；`memory.go` 与 `postgres.go` 必须同步实现所有领域。建议保留兼容适配器，但新代码不要再向 `Store` 添加方法。

### 1.2 必须先记录的行为/契约问题

1. **消息投递不是按连接隔离的。** `Server.events` 是一个全局 channel；多个 SSE 客户端会竞争消费事件，而不是每个设备都收到自己的事件。事件写入 channel 前也没有按已认证设备过滤。
2. **配对码没有账号上下文。** `POST /api/v1/pair` 未认证，配对码只包含过期时间和设备字段；任何拿到短码的客户端都可能抢先确认，无法表达“同一账号下的桌面与手机”。
3. **设备没有租户关系。** `devices` 缺少 `account_id`；发布时仅检查目标设备存在，未检查发送方和目标属于同一账号。
4. **认证凭证不可轮换/过期。** 设备 token 仅保存哈希，缺少 `expires_at`、刷新 token、上次使用时间和 token 版本；撤销接口也没有账号级“撤销全部设备”。
5. **幂等存在竞态。** `HasMessage` 后再 `PutMessage` 不是原子操作；内存 `seen` 与数据库唯一键的语义也不一致。应由 `(sender_device_id, message_id)` 唯一约束和持久化 ACK 统一实现。
6. **ACK/保留策略不一致。** PostgreSQL 将 `acknowledged_at` 标记后保留记录，内存实现直接删除；同步查询也只返回未 ACK 记录，客户端断线后无法稳定重放已持久化但未处理的消息。
7. **协议字段有漂移。** `docs/protocol.md` 使用 `version/messageId/createdAt`，Go 和 JSON Schema 使用 `v/id/ts`；`envelope.schema.json` 的 `additionalProperties: false` 又未声明 `recipient`，而 OpenAPI 声明了该字段。必须规定一个规范字段集，并为旧字段提供限期兼容层。
8. **方向权限不闭合。** 服务端 `publish` 只允许 `desktop` token，但 Flutter `sendReadOnlyRequest` 以 `sender: mobile` 调用同一端点；当前实现会拒绝该请求。读请求应有独立 API/消息类型和白名单，不应通过“伪造桌面发布”解决。
9. **桌面默认目标不合理。** 桌面发布没有 `recipient` 时，当前代码把消息归属到发布桌面本身，不能自然支持手机告警；默认行为应改为显式目标或账号内受权限控制的广播。
10. **OAuth 不是账号会话。** 当前 `internal/auth` 只验证 state/PKCE，回调返回 `validated`，没有建立账号、会话或设备会话；这应作为认证领域的底层组件，而不是由 API handler 直接拥有业务状态。
11. **启动未执行迁移。** `cmd/relay-server/main.go` 只建立连接，没有迁移版本检查/执行器。生产启动必须在独立发布步骤运行迁移，或启动时以锁保护执行；不能假定手工执行 `migrations/*.sql`。
12. **SSE 只能作为过渡。** `/api/v1/events` 没有 cursor/Last-Event-ID 语义，也未表达设备目标。实时 Agent 对话应使用按账号/设备授权的 WSS，HTTPS 负责补偿同步。

## 2. 推荐目标架构

### 2.1 依赖方向

```text
cmd/server
  -> internal/app (bootstrap/config/lifecycle)
      -> internal/httpapi (router/middleware/DTO)
          -> domain services
              -> repository interfaces
                  -> postgres repositories / memory repositories
shared/protocol (wire DTO + schemas; no SQL or secrets)
```

HTTP handler 只做解析、认证上下文、调用 service、映射错误；事务、授权规则、幂等和状态迁移必须在 service/repository 层完成。

### 2.2 第一阶段目录拆分（不改变外部 API）

建议按以下顺序新增文件，逐步从 `server.go` 搬移，不做大规模一次性改写：

```text
relay-server/
├── cmd/server/main.go                 # 新入口；旧 cmd/relay-server 可保留兼容
├── internal/config/config.go          # 环境变量、限制、TTL、地址
├── internal/httpapi/router.go         # 路由和 middleware 组合
├── internal/httpapi/response.go       # 统一错误/分页/request-id 响应
├── internal/httpapi/handlers/
│   ├── health.go
│   ├── auth.go
│   ├── pairing.go
│   ├── devices.go
│   ├── messages.go
│   ├── sync.go
│   └── sse_compat.go                  # 旧 SSE，标记 deprecated
├── internal/authn/                    # access/refresh token、OAuth state、认证上下文
├── internal/accounts/                 # Account、Identity、Session service/repository
├── internal/devices/                  # 设备注册、能力、presence、撤销
├── internal/pairing/                  # 一次性 challenge、尝试次数、绑定事务
├── internal/messaging/                # Envelope 校验、幂等、outbox、delivery/ACK
├── internal/sync/                     # cursor、补偿同步、保留/清理策略
├── internal/settings/                 # 设置文档、版本、ETag、冲突策略
├── internal/updates/                  # release manifest、渠道、平台、签名元数据
├── internal/conversations/            # 会话、消息、Agent run、权限/配额
├── internal/realtime/                  # WSS hub、连接生命周期、presence
└── internal/store/
    ├── repositories.go                # 按领域的小接口（逐步替代 Store）
    ├── postgres/                      # 每个领域一个 repository 文件
    └── memory/                        # 测试替身；语义与 PostgreSQL 一致
```

推荐的最小接口不是一个大 `Store`，而是：`AccountRepository`、`SessionRepository`、`DeviceRepository`、`PairingRepository`、`MessageRepository`、`SettingsRepository`、`ReleaseRepository`、`ConversationRepository`。事务边界可由 `UnitOfWork`/`Transactor` 暴露，但业务接口不应知道 pgx 类型。

### 2.3 Server 组装方式

`internal/app/server.go`（或 `internal/bootstrap/bootstrap.go`）负责读取配置、创建 pool、创建 repositories/services、运行 migration 检查、创建 router。`api.Server` 只保留兼容 facade：

```go
func NewServer(deps Dependencies) *Server
func (s *Server) Handler() http.Handler
```

不要让 handler 直接访问 `pgxpool.Pool`、环境变量、全局 channel 或 map。实时层使用 `DeliveryHub` 接口：`Publish(ctx, accountID, deviceID, event)`、`Subscribe(ctx, deviceID, cursor)`；实现层负责每连接队列、背压和断线清理。

## 3. 对外 API 演进

所有新接口使用 `/api/v1`，旧 `/pair`、`/messages`、`/sync` 保持 1 个迁移周期的兼容别名并记录 deprecation。除健康检查和 OAuth 跳转外，统一返回 `application/json` 的错误对象（见 `shared/protocol/server-contract.md`）。

### 3.1 账号与会话

| 方法 | 路径 | 作用 |
|---|---|---|
| `POST` | `/api/v1/auth/sso/start` | 创建一次性 PKCE state，返回授权 URL；verifier 优先由客户端生成 |
| `POST` | `/api/v1/auth/sso/callback` | 消费 state，建立/查找 Account，签发设备会话；服务端不保存 EVE access/refresh token |
| `POST` | `/api/v1/auth/token/refresh` | 轮换 refresh token，旧 token 立即失效 |
| `POST` | `/api/v1/auth/logout` | 撤销当前会话 |
| `GET` | `/api/v1/me` | 返回 account、功能开关和协议能力 |

身份提供商通过 `IdentityProvider` 接口隔离；EVE SSO 只提供 `subject`/角色摘要等最小身份结果。账号登录与设备配对是两个状态机，不要把 OAuth callback 直接当成配对成功。

### 3.2 设备与配对

| 方法 | 路径 | 作用 |
|---|---|---|
| `GET` | `/api/v1/devices` | 列出当前账号设备（名称、类型、能力、lastSeen、状态） |
| `PATCH` | `/api/v1/devices/{id}` | 重命名、更新能力声明 |
| `POST` | `/api/v1/devices/{id}/revoke` | 撤销单设备 |
| `POST` | `/api/v1/devices/revoke-all` | 撤销账号其他设备/全部设备 |
| `POST` | `/api/v1/pairing/challenges` | 已认证设备为当前账号创建 5 分钟一次性 challenge/二维码 |
| `POST` | `/api/v1/pairing/confirm` | 另一设备提交 challenge、公钥、签名，事务内绑定 account + 两台设备并签发会话 |
| `POST` | `/api/v1/devices/{id}/heartbeat` | 更新 presence 与客户端版本 |

服务端必须验证 challenge 的创建账号、目标设备、尝试次数（最多 5 次）、过期时间和签名覆盖范围；响应不再返回长期裸 token，采用短期 access + 轮换 refresh。

### 3.3 消息、同步和实时连接

| 方法 | 路径 | 作用 |
|---|---|---|
| `POST` | `/api/v1/events` | 已认证桌面发布允许的摘要事件（如 `intel.alert`） |
| `GET` | `/api/v1/devices/{id}/sync?after=&limit=` | 设备范围的补偿同步，返回 `nextCursor` 和是否还有更多 |
| `POST` | `/api/v1/messages/{id}/ack` | 带 `stage` 的幂等 ACK；只允许消息目标设备确认 |
| `GET` | `/api/v1/stream` | 过渡 SSE，必须带 `Last-Event-ID` 且按设备过滤 |
| `GET` | `/api/v1/ws` | WSS 实时事件、presence、Agent 会话帧；连接建立后由 token 推导 account/device，不信任 query deviceId |

事件投递按 `(account_id, recipient_device_id, sender_device_id, message_id)` 幂等。服务端先写 outbox，再投递实时帧；重连先 sync，再订阅 WSS。`intel.alert`、`desktop.status`、`conversation.*` 与查询响应使用不同权限检查。

### 3.4 设置同步

| 方法 | 路径 | 作用 |
|---|---|---|
| `GET` | `/api/v1/settings/{scope}` | 读取 `account`、`device` 或 `profile` 设置文档 |
| `PUT` | `/api/v1/settings/{scope}` | 使用 `If-Match`/`baseVersion` 乐观并发更新 |
| `POST` | `/api/v1/settings/{scope}/reset` | 恢复默认值（审计） |
| `GET` | `/api/v1/settings/changes?cursor=` | 设备断线后的设置变更同步 |

设置只保存白名单 JSON（大小、深度、键数有上限），禁止把 EVE token、私钥、原始聊天/邮件等秘密放入服务端。并发冲突返回 `409 SETTINGS_CONFLICT`，携带服务器版本和可选择的 merge hint；不要静默覆盖另一设备的修改。

### 3.5 应用更新

| 方法 | 路径 | 作用 |
|---|---|---|
| `GET` | `/api/v1/updates/manifest?platform=&arch=&channel=&version=` | 返回适用版本、强制级别、sha256、签名和下载 URL |
| `POST` | `/api/v1/updates/{release}/ack` | 记录下载/安装结果（不上传文件内容） |
| `GET` | `/api/v1/updates/policies` | 返回账号/组织的更新渠道策略 |

版本元数据由发布流水线写入 `app_releases`，二进制放对象存储/CDN；服务端不代理大文件。客户端必须校验签名和摘要，服务端只做平台、渠道、最低版本、灰度和撤回判断。

### 3.6 手机端与桌面端 Agent 对话

| 方法 | 路径 | 作用 |
|---|---|---|
| `POST` | `/api/v1/conversations` | 创建账号范围会话，选择绑定桌面设备/Agent |
| `GET` | `/api/v1/conversations` | 列出当前账号可见会话 |
| `GET` | `/api/v1/conversations/{id}/messages?cursor=` | 分页读取消息 |
| `POST` | `/api/v1/conversations/{id}/messages` | 手机发送用户消息，服务端持久化并路由到在线桌面 |
| `POST` | `/api/v1/conversations/{id}/cancel` | 取消当前 Agent run（需权限和幂等 key） |
| `GET` | `/api/v1/conversations/{id}/events` | 过渡 SSE；优先 WSS 订阅增量 token/状态 |

消息方向通过服务端路由，客户端不能把 `sender` 或 `recipient` 当作授权依据。桌面 Agent 只接受固定能力/操作白名单；禁止任意 SQL、URL、命令和 ESI endpoint。流式 token 可短暂存储为 run event，但默认保留摘要和最终消息，敏感原文按账号策略保留/删除。

## 4. 数据库迁移计划

每个迁移只做一件事，使用 `schema_migrations` 记录版本；迁移执行器在发布步骤运行并使用 PostgreSQL advisory lock。所有新表带 `created_at`、必要的 `updated_at`，以 `account_id` 做租户边界，敏感删除采用显式 retention job。

### `004_accounts_sessions.sql`

```sql
CREATE TABLE accounts (
  id UUID PRIMARY KEY,
  status TEXT NOT NULL DEFAULT 'active',
  display_name TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE identities (
  id UUID PRIMARY KEY,
  account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  provider TEXT NOT NULL,
  subject TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(provider, subject)
);
CREATE TABLE sessions (
  id UUID PRIMARY KEY,
  account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  device_id TEXT,
  refresh_token_hash TEXT NOT NULL UNIQUE,
  expires_at TIMESTAMPTZ NOT NULL,
  revoked_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_seen_at TIMESTAMPTZ
);
```

先创建 nullable `devices.account_id`，回填策略明确后再设为 `NOT NULL`；现有无法归属的开发数据必须隔离或清理，不能默认为同一个账号。

### `005_device_credentials_pairing.sql`

- `devices.account_id`、`last_seen_at`、`capabilities JSONB`、`token_version`、`revoked_at`、`updated_at`；将 `revoked BOOLEAN` 兼容迁移为 `revoked_at IS NULL` 语义。
- `pairing_codes.account_id`、`created_by_device_id`、`target_type`、`attempts`、`max_attempts`、`consumed_at`、`created_at`；索引 `(account_id, expires_at)`。
- token hash 加 `issued_at/expires_at`；新 token 采用 refresh rotation，旧记录只读兼容一周期。

### `006_message_outbox.sql`

将现有 `messages` 逐步扩展而非直接删除：

- `account_id`, `sender_device_id`, `recipient_device_id`, `kind`, `status`, `expires_at`, `persisted_at`, `delivered_at`, `processed_at`, `attempt_count`, `last_error`, `created_at`。
- 将唯一键从仅 `id` 迁移为 `(sender_device_id, id)`；先检查冲突再创建唯一索引。
- `cursor` 应是按投递目标可稳定查询的序列；至少索引 `(recipient_device_id, cursor)`、`(account_id, created_at)`。
- 保留 `body` 过渡期，新增 `payload JSONB` 或受控压缩字段；服务端不要把任意大 JSON 当成无界文本。
- 将 ACK 改为状态事件或 `message_receipts` 表，支持 `received/persisted/delivered/processed` 幂等写入。

### `007_settings.sql`

```sql
CREATE TABLE settings_documents (
  account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  scope TEXT NOT NULL,                    -- account | device | profile
  scope_id TEXT NOT NULL DEFAULT '',
  version BIGINT NOT NULL DEFAULT 1,
  document JSONB NOT NULL DEFAULT '{}'::jsonb,
  updated_by_device_id TEXT,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY(account_id, scope, scope_id)
);
CREATE TABLE settings_changes (
  id BIGSERIAL PRIMARY KEY,
  account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  scope TEXT NOT NULL,
  scope_id TEXT NOT NULL DEFAULT '',
  version BIGINT NOT NULL,
  changed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

`PUT` 必须在事务中校验 `baseVersion`，成功更新文档并追加 change；清理任务按账号保留最近 N 天/条数。

### `008_updates.sql`

`app_releases(id, app, version, platform, arch, channel, min_supported_version, mandatory, manifest JSONB, artifact_url, sha256, signature, published_at, withdrawn_at)`；对 `(app, version, platform, arch, channel)` 建唯一索引。灰度规则单独放 `update_rollouts`，不要硬编码在 handler。

### `009_conversations.sql`

- `conversations(id, account_id, target_device_id, agent_kind, status, created_at, updated_at)`
- `conversation_messages(id, conversation_id, sender_kind, sender_device_id, client_message_id, body, metadata JSONB, created_at, expires_at)`，唯一 `(conversation_id, sender_device_id, client_message_id)`。
- `agent_runs(id, conversation_id, input_message_id, status, started_at, completed_at, error_code)`。
- `conversation_events(id BIGSERIAL, conversation_id, run_id, sequence, type, payload JSONB, created_at)`，唯一 `(conversation_id, sequence)`。

默认不保存 EVE access/refresh token、私钥、原始聊天日志、钱包/资产全文；敏感内容采用加密/可选保留策略并支持账号导出与删除。

## 5. 分阶段交付

### P0：边界和安全基线（先做）

- 新增 `config`、`httpapi/router`、统一错误和认证 middleware；将 `server.go` 只留路由 facade。
- 修复按设备隔离的 delivery hub；SSE 每连接独立队列，sync 使用 `after/nextCursor/hasMore`。
- 将配对码绑定账号并限制尝试；补齐 `account_id` 的数据迁移设计。
- 用仓储层原子 `InsertIfAbsent` 替换 `HasMessage` + `PutMessage`；统一内存/PostgreSQL ACK 语义。
- 固化 canonical envelope 和错误 Schema，保留 `v/id/ts` 读取兼容但只生成长字段（或反之，见协议文档）。

### P1：账号、设备、设置

- 账号会话、refresh rotation、设备列表/心跳/撤销全部设备。
- settings document + ETag/version 冲突返回；移动端/桌面端使用同一 DTO。
- migration runner、审计日志、限流、request-id、指标和结构化日志。

### P2：更新与实时通信

- signed update manifest + CDN；桌面和手机分别声明 platform/arch/channel。
- WSS 连接鉴权、presence、订阅/背压；HTTPS sync 作为唯一补偿机制。
- FCM/APNs 只推送非敏感摘要和 event ID。

### P3：Agent 会话

- conversation/message/run/event 数据模型和配额；手机消息路由到指定桌面。
- Agent 能力白名单、用户确认、取消、断线恢复、审计；逐步弃用旧 `/messages`。
- 端到端 contract tests：Go、Flutter、桌面端共享示例 round-trip。

## 6. 直接实施的文件清单

### 本轮已落地（核心层与安全组件）

- `docs/server-roadmap.md`（本文）
- `shared/protocol/server-contract.md`（规范字段、API 状态机、兼容窗口）
- `relay-server/internal/accounts`、`internal/authn`：账号、会话、Token rotation/replay 撤销；兼容 API 已接入 `/api/v1/auth/refresh`
- `relay-server/internal/settings`：账号/设备设置、白名单、ETag/version 冲突
- `relay-server/internal/updates`：签名清单、SHA-256、版本选择和显式回滚
- `relay-server/internal/conversations`：账号/设备授权的 Agent 会话与消息状态机
- `relay-server/internal/realtime`：每设备独立队列、cursor replay、背压
- `relay-server/internal/httpapi`：request-id、统一错误、Origin/CORS、限流、body cap、日志脱敏
- `relay-server/migrations/004_accounts_sessions.sql` 至 `008_delivery.sql`
- `docs/server-accounts-sessions.md`、`docs/git-history-cleanup.md`

### 下一轮建议新增/调整

> 核心领域服务和通用安全中间件已完成；下一步重点是将它们接入账号认证、PostgreSQL repositories、settings/updates/conversations handlers，并替换旧 SSE 兼容实现。

- `relay-server/internal/config/config.go`
- `relay-server/internal/httpapi/router.go`
- `relay-server/internal/httpapi/middleware.go`
- `relay-server/internal/httpapi/handlers/{auth,pairing,devices,messages,sync}.go`
- `relay-server/internal/{accounts,authn,devices,pairing,messaging,sync,settings,updates,conversations,realtime}/`
- `relay-server/internal/store/repositories.go`（新增小接口，不再扩展 `store.Store`）
- `relay-server/migrations/004_accounts_sessions.sql` 至 `009_conversations.sql`
- `shared/protocol/examples/` 与固定版本的 `scripts/generate-*`
- `desktop-app/internal/app/relay.go`：改名为 `server client` 的兼容 facade，新增 sync/settings/conversation API client
- `mobile-app/lib/api/api_client.dart`：拆分 `AuthApi`、`DeviceApi`、`SyncApi`、`SettingsApi`、`ConversationApi`，不要把所有服务端能力加入一个接口

## 7. 验收门槛

- 任意两个账号的设备不能互相读消息、ACK、配对或订阅事件；测试覆盖内存和 PostgreSQL。
- 重复请求在网络重试下只产生一个业务消息；同一 ACK 重放返回相同结果。
- 断线后 `sync` 能从 cursor 补齐，WSS/SSE 不丢消息且每个设备独立接收。
- 设置并发更新返回 409 而非静默覆盖；更新 manifest 可验证签名和 sha256。
- 日志不含 token、Authorization、私密 payload；迁移可在空库和现有 001-003 库重复执行。
- `server.go` 不直接包含 SQL、环境读取、领域状态机；新增领域只需新增 handler/service/repository 文件。
