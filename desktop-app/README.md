# Alice-EVE Desktop

Go + Wails v2 + React/TypeScript 桌面客户端。

## 当前能力

- Alice 账户与 EVE SSO；令牌由 Go 层和操作系统安全存储管理，前端不接触令牌。
- Chatlogs/Gamelogs 本地增量采集、SQLite 游标和结构化事件。
- 版本化 SDE SQLite：物品、星系、星门、市场组、蓝图和中文名称。
- 角色、经济、市场、Intel、告警与 Agent 工作区。
- 全区域市场候选、订单深度、单品/同路线组合/取送链式贸易规划。
- 最低安全等级和最大跳数硬约束；批量安全路线调用。
- 最近一次市场规划本地恢复。

## 路线与缓存

市场页面先按起终点星系去重，再通过 `FetchEVESecureTradeRoutes` 一次批量请求路线。Go `RelayClient` 为整个应用提供：

- 路线缓存：10 分钟、最多 2,048 条；键包含起点、终点、安全等级和最大跳数。
- 公共实体缓存：24 小时、最多 4,096 条；当前覆盖空间站、星系和物品类型。
- 仅成功实体响应进入缓存；派生路线不写磁盘。

服务端返回 `unavailable` 时不会伪装成零跳。同一星系内不同空间站可合法显示为零星门跳。

## 构建

环境要求：

- Go 1.25+
- Wails CLI v2（当前验证使用 v2.15.0）
- Node.js 18+ 与 npm
- Windows WebView2 Runtime

```powershell
cd desktop-app/frontend
npm install
npm run build
cd ..
wails build -clean
```

Windows amd64 产物生成于：

```text
build/bin/eve-assistant.exe
```

构建产物由 `.gitignore` 排除，不提交到源码仓库。正式发布时由发布流水线生成并公布 SHA-256。

## 测试

```powershell
go test ./... -count=1
go vet ./...
cd frontend
npm run build
```

## 目录

- `internal/app`：应用服务、Relay Client、应用级缓存和 Wails DTO。
- `internal/eve`：ESI/SDE 桥接。
- `internal/evesde`：本地 SDE SQLite 查询和路线。
- `internal/ingest`：本地日志采集。
- `internal/storage`：SQLite 持久化和安全存储抽象。
- `frontend`：React/TypeScript UI。

## 安全边界

不得将 `EveClient.json`、访问令牌、refresh token、数据库、日志、生产地址或本机配置提交到 Git。公开示例只使用占位符；参见仓库根目录 `SECURITY.md`。
