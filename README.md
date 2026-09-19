# Alice-EVE

Alice-EVE 是面向 EVE Online 的中文桌面与移动助手。系统采用服务端集中同步和计算、客户端安全展示与操作的架构。

> 公开仓库不得包含生产域名、IP、主机别名、绝对运维路径、数据库连接、密钥、Token、证书或用户数据。安全边界见 [`SECURITY.md`](SECURITY.md)。

## 架构

```text
AliceEVE Desktop (Go + Wails + React) ─┐
                                      ├─ HTTPS/WSS ─ Alice-EVE Server (Go) ─ PostgreSQL
AliceEVE Mobile (Flutter) ────────────┘                         ├─ EVE ESI / SSO
                                                              └─ SDE / 市场与路线计算
```

- `desktop-app/`：Windows 桌面端、本地日志与 SDE、市场工作台。
- `mobile-app/`：Flutter 移动端、告警、会话和设备管理。
- `relay-server/`：账户、EVE 同步、市场采集、规划任务、消息与实时通道。目录名为兼容历史路径保留。
- `shared/protocol/`：协议模型、OpenAPI 和 JSON Schema。
- `docs/`：部署、安全和协议说明。

## 当前能力

- Alice 账户会话与 EVE SSO/PKCE；EVE refresh grant 在服务端加密保存。
- 账户隔离的只读 ESI 同步，版本化 SDE 导入与名称解析。
- Chatlogs/Gamelogs 增量采集，WSS、Outbox、ACK 与游标恢复。
- 全区域市场采集、原子快照、订单深度和安全路线计算。
- 持久化市场规划任务：选定区域作为采购起点，默认在全部已采集区域出售。
- SINGLE/BASKET/CHAIN 独立会话、增量 revision、页面恢复和新快照自动重算。
- 自适应装载集中度，CHAIN 提供航段、装卸和买卖操作。

## 构建与固定产物名

统一执行：

```powershell
./scripts/build-release.ps1
```

产物固定覆盖到：

```text
releases/Alice-EVE-Desktop.exe
releases/Alice-EVE-Mobile.apk
```

安装后的应用显示名统一为 `AliceEVE`。Android `applicationId` 保持 `com.aliceeve.mobile`，因此同一签名和更高版本号会覆盖更新，不创建新的应用实例。

单独验证：

```powershell
cd relay-server; go test ./... -count=1; go vet ./...
cd ../desktop-app; go test ./... -count=1; go vet ./...
cd frontend; npm test; npm run build
cd ../../mobile-app; flutter analyze; flutter test
```

## 文档

- [`docs/architecture.md`](docs/architecture.md)：当前系统与市场规划架构。
- [`docs/protocol.md`](docs/protocol.md)：HTTP/WSS 协议约定。
- [`docs/deployment.md`](docs/deployment.md)：通用部署边界。
- [`SECURITY.md`](SECURITY.md)：敏感信息和报告策略。

历史分析、工作记录和一次性实施计划不再作为当前架构说明；以代码、迁移和上述文档为准。
