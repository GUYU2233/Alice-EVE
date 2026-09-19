# 服务端账号与会话基础

本轮新增 `relay-server/internal/accounts` 领域层和 `relay-server/internal/authn` 凭证工具，保留现有 `internal/api/server.go` 及旧配对/设备 API 不变。

## 数据模型

迁移 `relay-server/migrations/004_accounts_sessions.sql` 创建：

- `accounts`：账号状态为 `active` 或 `revoked`，撤销时记录 `revoked_at`。
- `identities`：外部身份 `(provider, subject)` 唯一映射到账号。
- `sessions`：账号/设备会话，以及 access/refresh token 的 SHA-256 哈希。

会话的 `token_family_id` 标识一组轮换凭证；`replaced_by_hash` 和 `rotated_at` 记录轮换链路，`family_revoked_at` 标记因重放而整族撤销。数据库中不保存任何裸 token。

## 领域 API

`accounts.Service` 提供：

- `EnsureAccount`：按身份查找或创建账号，身份唯一冲突时安全重读。
- `IssueSession`：创建新 token family，返回一次性的裸 access/refresh credential；只把哈希写入 repository。
- `AuthenticateAccess`：校验 access token、access TTL、会话撤销状态和账号状态。
- `Refresh` / `RefreshAt`：refresh token 一次性轮换。旧 token 立即撤销，替换 token保留原 family 的绝对过期时间。
- `RevokeSession`、`RevokeAccount`：撤销当前会话或账号的全部会话。

`SessionRepository.RotateRefresh` 要求实现为原子状态迁移。PostgreSQL 实现用 `SELECT ... FOR UPDATE`；内存实现使用同一把锁。已经轮换过的 refresh token 再次出现时，整条 token family 都会被撤销，并返回 `ErrRefreshReplay`。

## 凭证边界

`authn.GenerateToken` 使用操作系统 CSPRNG 生成 256-bit opaque token，`authn.HashToken` 生成 SHA-256 查找摘要。SHA-256 仅用于高熵随机 token，不用于密码。裸 token 只能出现在签发响应的内存值中，调用方不得写日志或持久化。

当前 API handler 保持旧 `/api/v1/pair`、`/api/v1/pair/confirm` 和设备 token 行为，同时提供无配对码设备授权：`POST /api/v1/auth/device/start` 创建仅保存哈希的 5 分钟挑战，`POST /api/v1/auth/device/complete` 一次性消费挑战并签发短期 access、轮换 refresh 和兼容 device token。请求可用已有 `accountId`，或用 `provider` + `subject` 自动查找/创建账号；生产配置数据库时默认拒绝未验证的匿名身份输入，开发环境可用 `ALLOW_UNVERIFIED_DEVICE_AUTH=1` 显式开启 bootstrap。EVE SSO 可从部署 Secret 注入 `EVE_SSO_CLIENT_SECRET`，仅用于 token endpoint HTTP Basic 且不写数据库；EVE refresh grant 使用 `EVE_GRANT_KEYRING` 加密保存以支持后台同步。`POST /api/v1/auth/device/revoke` 使用 access token 撤销当前会话及设备。挑战即使完成失败也会消费，防止重放。
