# Alice-EVE 当前架构

## 运行边界

Alice-EVE 由桌面端、移动端、服务端和 PostgreSQL 构成。客户端不持有数据库凭证，不直接调用需要长期凭证的 EVE 接口。服务端负责认证、同步、持久化计算和跨设备消息；桌面端额外负责本地日志与本地 SDE 能力。

## 服务端

Go 服务端启动时校验并应用 `relay-server/migrations/`。主要领域：

- `accounts` / `auth` / `evegrant`：账户、会话、PKCE 和加密 EVE grant。
- `esidata` / `esisync` / `esipublic`：角色快照、后台同步和公共 ESI 缓存。
- `marketdata`：区域采集调度、原子市场快照和候选查询。
- `routeplanner`：active SDE 星门图、安全约束路线和有界缓存。
- `marketplan`：持久化规划任务、Worker 租约与 revision 化结果；frontier 表保留用于后续切片续算，目前不作为已实现能力。
- `realtime` / `notifications` / `conversations`：WSS、补偿同步、通知和对话事件。

## 持久化市场规划

### 语义

- `sourceRegionIds` 是采购起点，不限制出售范围。
- 默认 `destinationScope=all_collected_regions`；也可显式指定目的地区域。
- 候选按物品保留多个采购/出售位置，而非只取最低卖价和最高买价各一个站点。
- 无可用路线与合法零跳路线严格区分。

### 数据与执行

迁移 `028_market_plan_jobs.sql` 提供：

- `market_plan_jobs`：约束、状态、租约、iteration 和 result revision。
- `market_plan_results`：按 revision/rank 保存 canonical payload。
- `market_plan_frontiers`：为后续切片续算预留；当前引擎每次按最新快照执行一次完整有界计算。

Worker 使用 `FOR UPDATE SKIP LOCKED` 和 token/lease fencing 并发领取。状态为：

```text
queued → discovering → routing → optimizing → watching
```

市场快照更新后，相关 watching 任务重新排队。客户端通过 `afterRevision` 获取增量结果；HTTP 是可靠恢复来源，实时消息只用于唤醒。

### 三种模式

- **SINGLE**：单项交易及完整路线。
- **BASKET**：同一采购/销售路线上的多物品组合；集中度按 `0.25 → 0.4 → 0.6 → 1.0` 自适应放宽，默认目标装载率 90%。
- **CHAIN**：多站取送；每站含 `arriveVia`、`loads`、`unloads`，展示经由星系和 BUY/SELL 操作。

桌面 Workspace 为 SINGLE/BASKET/CHAIN 分别保存参数、job ID、状态、revision、结果和选中项；异步结果必须同时匹配模式和 job ID，不能跨模式覆盖。

## 客户端

### Desktop

Go/Wails 暴露强类型 Bridge，React 只接收业务 DTO。令牌由 Go 层和操作系统安全存储管理。市场状态持久化在应用级 Workspace；页面切换或重启不清空结果。

### Mobile

Flutter 使用 HTTPS/WSS、系统安全存储和平台推送抽象。发布 APK 必须使用 CI/Secret Manager 注入的 release keystore，禁止回退到 debug 签名。

## 一致性与安全

- API 查询必须带账户边界；角色资源需验证所属账户。
- 数据库迁移带 checksum 和 advisory lock。
- 服务端 access/refresh token 仅以哈希持久化；EVE grant 加密保存。
- 构建和文档不得嵌入生产拓扑或凭证。
- 发布前执行 `scripts/security-audit.ps1 -IncludeIgnored` 和 Git 历史扫描。
