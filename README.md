# Alice-EVE

EVE Online 中文信息助手的本地优先原型，当前版本为 **v0.0.1-alpha**。

## 项目组成

- `desktop-app/`：Go + Wails + React 桌面端
- `mobile-app/`：Flutter 移动端
- `relay-server/`：Go Relay 服务与协议 API
- `shared/protocol/`：JSON Schema、OpenAPI 与协议文档
- `docs/`：设计与通信文档

## Alpha 能力

- Relay 健康检查、设备配对、Token 认证与撤销
- 桌面端测试告警发布
- 移动端告警获取与确认
- 桌面 Outbox、ACK 与重试基础
- PostgreSQL Store 与部署模板
- ESI Gateway、PKCE 和 Intel 风险评分基础

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

当前 Alpha 仍不包含完整 EVE SSO 角色绑定、FCM/APNs、完整 WSS 实时推送、市场工作台和正式代码签名。

## 发布文件

构建发布产物请在本地生成；发布目录约定为 `Releases/`，但默认通过 `.gitignore` 排除二进制文件，避免将大型构建产物和潜在敏感运行文件提交到源码仓库。
