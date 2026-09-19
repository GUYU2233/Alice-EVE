# Alice-EVE

EVE Online 中文信息助手的本地优先原型，当前版本为 **v0.0.1-alpha**。

> 公开仓库安全边界：生产域名、IP、Cloudflare/DNS 标识、服务器路径、数据库连接信息和所有密钥不属于源码仓库。参见 [`SECURITY.md`](SECURITY.md) 与 [`docs/deployment.md`](docs/deployment.md)。

## 项目组成

- `desktop-app/`：Go + Wails + React 桌面端
- `mobile-app/`：Flutter 移动端
- `relay-server/`：Go 服务端（Server）与协议 API；`relay-server/` 仅作为代码目录兼容旧路径，暂不更改
- `shared/protocol/`：JSON Schema、OpenAPI 与协议文档
- `docs/`：设计与通信文档

## Alpha 能力

- 服务端健康检查、账户/设备会话、Token 轮换与撤销
- EVE SSO/PKCE、加密 refresh grant 和账户隔离的只读 ESI 同步
- Chatlogs/Gamelogs 本地增量采集、SQLite 游标和结构化事件
- 版本化 SDE SQLite/PostgreSQL 导入、校验与原子激活
- 全区域市场调度、原子快照、32 档订单深度与候选查询
- 单品、同路线组合和取送链式贸易规划
- 可复用安全路线模块、批量路线 API 和分层缓存
- WSS、Outbox、ACK、游标恢复与移动端告警基础

## 术语与兼容性

产品和文档统一称为“服务端”（Server）。仅为保持代码目录与既有接口兼容，保留 `relay-server/` 目录、`cmd/relay-server` 启动路径、现有 API 路径、协议线上的 `relay` 字段以及客户端兼容标识；不改变 API 行为。

## 构建

### Desktop

```powershell
cd desktop-app/frontend
npm install
npm run build
cd ..
wails build -clean
```

### Mobile

```powershell
cd mobile-app
flutter pub get
flutter analyze
flutter test
flutter build apk --release
```

## 安全说明

本仓库不应提交任何 EVE Token、数据库密码、设备 Token、私钥、证书、服务器环境文件或生产数据库。生产配置请使用本地未跟踪的环境文件。

公开文档同样不得暴露真实公网 IP、域名、主机别名、绝对运维路径、Cloudflare Zone ID、DNS/代理细节或其他可定位生产环境的信息；示例必须使用占位符或文档保留值。公开仓库与私有运维仓库的边界见 [`docs/security-public-boundary.md`](docs/security-public-boundary.md)。

当前 Alpha 仍不包含生产 FCM/APNs、完整三端移动实时联调、玩家建筑完整名称覆盖、正式代码签名和自动更新发布链。

## 发布文件

构建发布产物请在本地生成；发布目录约定为 `releases/`，但默认通过 `.gitignore` 排除二进制文件，避免将大型构建产物和潜在敏感运行文件提交到源码仓库。
