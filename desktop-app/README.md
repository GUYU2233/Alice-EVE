# AliceEVE Desktop

Go 1.25 + Wails v2 + React/TypeScript 桌面客户端。

## 主要能力

- Alice 会话、EVE SSO 和操作系统安全存储；前端不接触 Token。
- Chatlogs/Gamelogs 增量采集与 SQLite 游标。
- 本地版本化 SDE：物品、星系、星门、市场组、蓝图和名称。
- 角色、经济、市场、Intel、告警和 Agent 工作区。
- 服务端持久化 SINGLE/BASKET/CHAIN 市场任务、增量结果和独立模式缓存。
- 安全路线、站点名称、装载清单，以及 CHAIN 航段和买卖操作。

## 构建

要求 Go 1.25+、Wails CLI v2.15+、Node.js 18+、npm 和 WebView2 Runtime。

```powershell
cd frontend
npm install
npm test
npm run build
cd ..
go test ./... -count=1
go vet ./...
wails build -clean
```

`wails.json` 固定生成：

```text
build/bin/Alice-EVE-Desktop.exe
```

安装/运行显示名为 `AliceEVE`。发布脚本会覆盖复制到 `releases/Alice-EVE-Desktop.exe`，不增加版本或临时后缀。

## 目录

- `internal/app`：服务端 Client、Bridge DTO、应用服务与缓存。
- `internal/eve`、`internal/evesde`：ESI/SDE 桥接和本地查询。
- `internal/ingest`、`internal/storage`：日志采集、SQLite 和安全存储。
- `frontend`：React UI、市场 Workspace 和测试。

不要提交 Token、数据库、日志、生产地址、用户配置或构建产物。完整架构见 [`../docs/architecture.md`](../docs/architecture.md)。
