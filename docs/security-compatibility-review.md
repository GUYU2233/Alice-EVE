# OAuth / WSS / Durable / Push / ESI-SDE 审查

审查范围：`relay-server`、`desktop-app`、`mobile-app` 及共享协议。本文只记录公开的架构风险，不包含部署凭证或真实拓扑。

## 结论（发布阻断项）

| 优先级 | 缺口 | 影响 | 当前状态 |
| --- | --- | --- | --- |
| P0 | 设备缺少强制 `account_id` 归属；旧消息发布按 `DeviceExists` 检查目标 | 已知设备 ID 可造成跨账号投递/混淆 | 需要迁移与发布前集成测试 |
| P0 | OAuth 回调必须完成浏览器会话绑定；仅 state/PKCE 不能替代 callback cookie/nonce | login-CSRF 或账号链接错误，可能把 EVE 身份绑定到攻击者会话 | 服务端 OAuth 基础已支持 PKCE；浏览器端仍需会话绑定审查 |
| P1 | WSS/事件层不能依赖单实例内存；旧 SSE 使用全局 channel | 多连接会互相消费事件，重启/多实例丢失；必须先 sync 后 subscribe | `realtime.Hub` 已提供设备队列/游标抽象，但尚未接入持久 outbox |
| P1 | 设备 challenge、通知 token/偏好、conversation 内存实现 | 重启和多实例不一致；FCM/APNs 无真实 dispatcher | 需要 PostgreSQL repository、outbox、TTL/去重 |
| P1 | 移动端命令入口与服务端鉴权不匹配 | `sender=mobile` 发往桌面专用旧消息接口会失败；不能绕过 conversation 权限 | 应使用账号/设备授权的 conversation ingress 与固定 operation 白名单 |
| P1 | 现代 access/refresh 生命周期未贯通桌面；移动端无 refresh-on-401 | access 过期后客户端表现为掉线，稳定 legacy token 又扩大撤销/轮换边界 | 需要安全存储 access/refresh、轮换、失败清理和重试幂等 |
| P1 | FCM/APNs 依赖/适配器尚未配置，且 provider wire enum 漂移 | 后台通知在生产不可交付或无法按平台路由 | 统一 provider/platform 枚举并加入真实适配器验收 |
| P1 | ESI/SDE 仍是通用/进程内实现 | 无 ETag/429/420/Retry-After 完整处理；SDE 非版本化 SQLite；数据更新可能静默漂移 | 需要固定上游 allowlist、缓存元数据/版本/签名与资源上限 |
| P2 | 桌面端 relay URL 若接受任意来源会向钓鱼主机发送 Bearer | 本地配置/localStorage 被篡改时可泄露凭证 | 已修复：仅 HTTPS；HTTP 仅允许 loopback 开发地址 |
| P2 | Windows DPAPI 对空/损坏文件取 `&b[0]` | 空值/损坏凭据文件可能 panic | 已修复空值输入与空文件错误返回 |
| P2 | 桌面 Clear pairing 未必远程撤销 | 被盗的服务端 token 在本地清理后仍有效 | 需要 revoke-before-delete（并明确网络失败策略） |
| P2 | 桌面非 Windows 使用内存 SecretStore；local-agent 根目录依赖 CWD | 重启丢凭据；启动目录变化或 symlink 影响工作区边界 | 需要 Keychain/Secret Service 与显式 user-data workspace 根 |
| P3 | SQLite `synchronous=NORMAL` 的断电语义、工具审计取消关联 | 电源故障可能丢最近提交；取消后审计行可能残留 | 明确保留策略，或将 turn/request marker 贯穿所有审计行 |

## 已完成的低风险改动

- `relay-server/internal/api/conversation_realtime.go`：WebSocket writer 串行化数据帧/控制帧，避免 gorilla websocket 并发写；Origin 不再无条件放行，浏览器 Origin 仅匹配 `CORS_ALLOWED_ORIGINS`，无 Origin 的原生客户端仍可连接。
- `desktop-app/internal/app/relay.go`：解析并限制 relay URL，拒绝用户信息、query/fragment 和外部 HTTP；仅 loopback 允许 HTTP 测试/开发。
- `desktop-app/internal/storage/secret_store_windows.go`：DPAPI Save/Load 对空输入和空文件安全处理，避免索引越界 panic。
- 新增 URL 安全负向测试及 SDE cache round-trip 测试，覆盖缓存文件名一致性和加载路径。
- `relay-server/internal/api/sso.go`：删除已不存在的 state metadata API 调用，使当前 OAuth Exchange/PKCE 代码可编译；该流程仍需设备元数据/浏览器会话绑定的集成验收。

## 可行检查

- `relay-server`: `go test ./...` 通过。
- `desktop-app`: `go test ./...` 通过。
- Flutter 工具链若在 CI 可用，应执行 `flutter analyze`、`flutter test`；本次审查未将 Android/iOS SDK 当作已配置事实。

## 发布前验收清单

1. 两个账号、两个设备同时在线：跨账号 recipient、ACK、sync、WSS 订阅均拒绝。
2. OAuth：state、PKCE verifier、callback 浏览器 cookie/nonce 必须同一会话；重复 callback、过期 state、并发实例、重启均失败关闭。
3. 重启/多实例：challenge、session、conversation、delivery cursor、push token 均从 PostgreSQL 恢复；先 sync 后 subscribe，不丢不重放。
4. FCM/APNs：只发送非敏感摘要与 event ID；token 加密/哈希边界、TTL、去重、撤销和 provider 错误重试均有测试。
5. ESI：仅允许固定 HTTPS host；响应体上限、ETag、Cache-Control、429/420/Retry-After、取消和坏 JSON 有测试。
6. SDE：采用明确版本/来源/校验和，目录/文件/条目资源上限，symlink escape 和损坏缓存测试；不要把通用 JSON walker 当作 CCP canonical SDE。
7. 发布包：锁定依赖版本和 registry，使用 `npm ci`/等价可重复构建；代码签名、WebView2/系统钥匙串和 macOS network entitlement 纳入验收。
