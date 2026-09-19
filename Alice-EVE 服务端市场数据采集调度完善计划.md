# Alice-EVE 服务端市场数据采集调度完善计划

## 0. 实施状态（2026-09-19）

本计划中的服务端持久化调度、ESI 限流、有界分页采集、批量写入、current/previous 原子快照、服务端版本化 SDE、best-level/32 档订单深度、全品类候选、固定路线组合、取送链式规划和桌面工作台均已实现并完成测试/构建验证。

路线能力已进一步抽取为 `relay-server/internal/routeplanner` 共享模块：

- active SDE build 对应不可变星门图；
- 最低安全等级与最大跳数硬约束；
- 单条和最多 500 条批量 API；
- 服务端版本化路线缓存和 Desktop 应用级缓存；
- SDE build 切换后自动失效；
- 不可达路线明确返回 `unavailable`；
- 同星系不同空间站允许合法零星门跳。

仍需迭代的内容包括多版本规划记录、玩家建筑名称、装配后精确货舱、路线/缓存指标、Linux race/压力验证和移动端联动。

## 1. 目标

将当前“客户端点击一次、服务端采集一次”的区域市场采集，升级为服务端自主、持久化、可恢复、可观测的全服滚动市场数据平台，并为全品类单品、组合和链式贸易规划提供稳定数据基础。

核心完成标准：

- 不依赖客户端首次点击即可自动维护市场快照；
- 每个区域独立原子发布 current/previous 快照；
- Relay 重启后队列、任务和调度状态可恢复；
- 同一区域不会重复并发采集；
- 大区域采用有界并发分页与批量写入；
- 根据 ESI Error Limit 自动降速或暂停；
- 服务端具备完整市场物品名称、体积、市场组、星系、安全等级和星门数据；
- 快照发布后生成候选查询摘要，避免每次扫描数十万原始订单；
- 客户端可查看快照年龄、覆盖率、任务状态、下一次刷新及 last-good 状态。

## 2. 当前基线

已具备：

- ESI 区域订单完整分页采集；
- PostgreSQL 规范化订单存储；
- staging batch；
- current/previous 原子快照；
- 失败批次不污染 current；
- 跨页重复订单幂等处理；
- 异步采集和客户端状态轮询；
- The Forge 409 页、约 40.8 万唯一订单生产快照；
- 桌面端完整 CCP SDE build 3503375；
- 全品类候选、组合和链式规划基础模块。

当前瓶颈：

- 采集依赖客户端触发；
- 分页严格串行；
- 每页订单逐条 INSERT；
- 任务状态仅保存在进程内 `sync.Map`；
- 服务端只有极少量 `public_data_cache` 类型体积；
- 候选查询仍依赖原始订单聚合；
- 无自动刷新、失败退避、ESI Error Limit 控制与生产指标。

## 3. 目标数据流

```text
区域调度配置
  → 持久化任务队列
  → 数据库 Claim / 同区域互斥
  → 第一页探测 X-Pages
  → 有界分页 Worker Pool
  → 批量 COPY/UNNEST staging
  → 页面覆盖与数据完整性校验
  → current/previous 原子发布
  → best-level 摘要与候选 generation
  → 单品 / 组合 / 链式规划
```

客户端仅负责查看状态、请求优先刷新和消费规划结果。

## 4. P0：持久化调度与可靠性

### 4.1 Migration 022

新增 `region_market_schedule`：

- `region_id` 主键；
- `enabled`；
- `tier`；
- `priority`；
- `refresh_interval_seconds`；
- `min_refresh_interval_seconds`；
- `max_staleness_seconds`；
- `last_requested_at`；
- `next_run_at`；
- `last_success_at`；
- `consecutive_failures`；
- `backoff_until`；
- `access_score`；
- `change_score`；
- 时间戳。

新增 `region_market_collection_jobs`：

- `id`；
- `region_id`；
- `state`：queued/claimed/collecting/publishing/complete/failed/canceled；
- `priority`；
- `trigger`：scheduler/user_priority_refresh/startup_recovery/manual_admin/stale_snapshot；
- `requested_by_account_id`；
- `claim_token`、`claim_expires_at`；
- `requested_at`、`claimed_at`、`started_at`、`completed_at`；
- `attempt`；
- `failure_code`、`failure_detail`。

使用部分唯一索引保证同一区域最多一个活动任务。

### 4.2 Claim 与恢复

- `FOR UPDATE SKIP LOCKED` 领取任务；
- Claim Token 防止旧 Worker 完成新任务；
- 租约定时续期；
- 启动时回收过期 Claim；
- 中断批次标记 failed，不发布半快照；
- last-good current 始终可用。

## 5. P0：采集性能

### 5.1 有界分页并发

- 第一页串行获取 `X-Pages`、ETag、Expires、Error Limit；
- 第 2..N 页使用 Worker Pool；
- 初始单区域并发 4；
- 全局 ESI 并发 8；
- 同时活动区域 2；
- 支持环境变量配置。

### 5.2 批量写入

将每页约 1,000 次单条 INSERT 改为：

- `pgx.CopyFrom` 到临时/暂存表；
- 再 `INSERT ... SELECT ... ON CONFLICT DO NOTHING`；
- 或使用一次有界 UNNEST 批量插入；
- `order_count` 仅统计实际插入唯一订单。

### 5.3 完整性

- 页面号唯一；
- 发布前验证 1..X-Pages 全覆盖；
- X-Pages 变化时整批失败并重新排队；
- 跨页 Order ID 冲突幂等忽略并计数；
- ESI 缓存 Expires 不作为长批次发布硬约束。

## 6. ESI Error Limit 控制

解析：

- `X-ESI-Error-Limit-Remain`；
- `X-ESI-Error-Limit-Reset`；
- `Retry-After`。

策略：

- Remain > 50：正常并发；
- 20–50：并发减半；
- 5–20：停止领取新页；
- < 5：全局暂停到 Reset；
- 420/429 遵循 Retry-After；
- 5xx、超时使用带 jitter 指数退避。

## 7. P1：服务端完整 SDE

新增服务端表：

- `eve_sde_builds`；
- `eve_types`：中英文名、group、market group、published、volume、packaged volume、capacity；
- `eve_market_groups`；
- `eve_regions`；
- `eve_constellations`；
- `eve_systems`：security；
- `eve_stargates`。

提供导入命令读取 CCP 官方 JSONL ZIP，经 staging 校验后原子切换 build。

运输体积规则：

1. `packaged_volume > 0` 时优先；
2. 否则使用 `volume > 0`；
3. 两者都无效则排除并记录原因。

候选查询切换到 `eve_types`，不再依赖零散 `public_data_cache`。

## 8. P2：自动分层刷新

初始 Tier：

- A：主要贸易区域，10–15 分钟；
- B：活跃区域，30–60 分钟；
- C：普通区域，1–3 小时；
- D：冷区域，6–24 小时或按需。

优先级由以下因素组成：

- 无 current snapshot；
- 快照年龄；
- 用户优先刷新请求；
- 查询与规划访问热度；
- 市场活跃度和变化率；
- 失败恢复。

每次计划加入 ±10%～20% jitter。

失败退避：1 分钟、3 分钟、10 分钟、30 分钟，最大 2 小时。

## 9. P3：候选预计算

新增 `region_market_best_levels`：

- batch/region/location/system/type；
- best ask、ask depth；
- best bid、bid depth。

快照发布后生成摘要，候选查询不再重复扫描原始订单。

新增 candidate generation，记录所用 snapshot IDs、生成时间和过期时间。预算、货舱和税率在请求时动态应用。

## 10. API

- `GET /api/v1/trade/market/regions`：区域目录与快照健康度；
- `GET /api/v1/trade/market/regions/{id}`：current/previous、年龄、覆盖、任务、下次刷新；
- `POST /api/v1/trade/market/regions/{id}/refresh`：请求优先刷新，不强制重复任务；
- 管理 API：启停区域、修改 Tier、重排失败任务、暂停调度。

规划规则：

- current 可用时，即使后台刷新也使用 last-good；
- 超过硬过期阈值才阻止可执行计划；
- 所有计划记录 snapshot generation；
- 执行前重新校验但不静默改变冻结参数。

## 11. 客户端

- “采集当前区域”改为“请求优先刷新”；
- 展示 current 快照时间、年龄、页数、订单数；
- 展示任务状态、队列位置、下次自动刷新；
- 展示 last-good 与后台刷新；
- 快照可用时不因刷新任务阻塞规划。

## 12. 可观测性

指标：

- collection duration/pages/orders/duplicates/failures；
- queue depth、active regions；
- snapshot age；
- candidate query duration/count；
- ESI Error Limit；
- COPY duration；
- scheduler lag。

日志字段：job_id、batch_id、region_id、page、expected_pages、received、inserted、duplicates、duration、ESI limit。

告警：Tier A 超龄、连续失败 3 次、Error Limit < 10、采集超时、候选查询 > 2 秒、队列积压、发布失败。

## 13. 测试与性能目标

测试覆盖：

- 调度优先级、jitter、退避；
- 同区域互斥、Claim 恢复；
- 并发分页、随机页失败、页数变化；
- 重复订单；
- ESI 420/429/5xx；
- Relay 重启；
- current/previous；
- 采集与查询并发；
- 服务端 SDE 完整性。

目标：

- The Forge 完整采集 < 5 分钟，优化目标 1–2 分钟；
- 全服滚动覆盖保守 30–60 分钟；
- 单页批量写入 < 100 ms；
- 候选摘要生成 < 10 秒；
- 候选 API P95 < 1 秒；
- 状态 API P95 < 100 ms。

## 14. 实施顺序

1. P0 Migration、Repository、Claim、恢复；
2. 批量写入与分页 Worker Pool；
3. ESI Error Limit 控制；
4. P1 服务端 SDE 导入与查询迁移；
5. P2 Scheduler 与 Tier；
6. P3 Best Levels 和候选 Generation；
7. 状态/管理 API；
8. 客户端状态与优先刷新；
9. 性能、故障注入和生产部署验证。

每阶段必须保持：失败不发布半快照、last-good 可读、认证边界有效、资源有硬上限。

## 15. 贸易运输规划二期：从价差候选升级为可执行整船方案

### 15.1 问题与模式定义

当前 best-level 候选只能表达最优一档价格，无法正确利用后续仍盈利的订单深度；路线缺失时还可能显示为 `0 跳`，组合与链式也没有形成严格的取送货语义。二期改造不再把“商品行”当成完整计划：

- **单品**：一个采购地点购买一种物品，运到一个销售地点出售；
- **组合**：在同一个具体空间站/建筑购买多种物品，一次装船，在另一个具体地点集中出售；
- **链式**：沿途依次取货和送货，可以同时携带多个目的地的库存，每站更新现金、货舱、库存和剩余订单深度。

地点以具体 station/structure 为边界；同星系换站仍是一次停靠，不能合并成原地成交。

### 15.2 P0：多档订单深度与成交曲线

快照发布时除 `region_market_best_levels` 外，新增有界多档深度摘要。每个 batch/region/location/system/type/direction 保存按成交优先级排列的价格区段：

- 卖单：价格由低到高；
- 买单：价格由高到低；
- price、remaining volume、minimum volume、level sequence；
- snapshot/batch、location、system、type；
- 摘要截断状态及被省略深度。

第一版可使用每侧前 20–32 个价格区段，并同时设置单类型数量上限，保证存储和查询资源有硬边界。无法覆盖计划数量时必须标记深度不足，不能按最优价外推。

数量为 `q` 时按价格区段累加：

```text
采购成本 = Σ(采购区段数量 × 区段卖价)
销售收入 = Σ(销售区段数量 × 区段买价)
平均买价 = 采购成本 / q
平均卖价 = 销售收入 / q
净利润 = 销售收入 - 采购成本 - 销售税 - 经纪费用 - 配置的运输成本
```

继续加购由下一增量区段的边际收益决定，而不是只取最低卖价。系统尽量填充货舱，但绝不为满仓加入负边际利润货物。不满仓必须返回原因：盈利深度不足、预算不足、剩余体积碎片、路线不合规或成交规则不满足。

### 15.3 P0：真实路线与可执行性状态

路线必须区分：

```text
当前位置 → 采购地点（接货航程）
采购地点 → 销售地点（载货航程）
```

链式模式返回完整站点序列。路线 DTO 至少包含：中文地点/星系名、系统序列、接货跳数、载货跳数、总跳数、实际最低安全等级、停靠次数、状态和失败原因。

规则：

- 基于服务端完整 SDE 星门图寻路；
- 最低安全等级作为寻路硬约束，不符合的星系不可进入；
- 最大跳数作为计划硬约束；
- 路线失败显示 `unavailable`，未知显示 `pending`，不得显示为 `0 跳`；
- 用户安全阈值不得冒充路线实际最低安全等级；
- 建筑准入未知时标记未验证，不归类为可直接执行。

### 15.4 P1：同地采购、同地销售的组合整船优化

组合规划先按 `(source_location_id, destination_location_id)` 分组，只允许一个采购地点和一个销售地点：

1. 枚举该地点对之间可成交的全部物品；
2. 用多档深度生成每种物品的离散数量—成本—收入区段；
3. 在预算、现金预留、货舱、订单深度和路线约束下联合选量；
4. 使用有界状态搜索加局部替换/补货优化整船净利润；
5. 输出一份采购清单、一次载货路线和一份销售清单。

硬约束：总成本不超过可用预算；总体积不超过手动货舱容量；不重复消耗订单深度；满足 minimum volume；路线满足安全和跳数；同一物品集中度可配置但不应阻止合理满仓。

返回值需包含：装载率、总成本、费用拆分、整船净利润、平均买卖价、实际订单区段、路线及不满仓原因。小规模数据使用穷举 Oracle 对比，证明有界算法不会退化成简单评分贪心；大规模仅承诺在计算预算内返回最佳已知可行解，不虚构数学全局最优。

### 15.5 P2：链式取送与配送平台式路径效率

链式状态必须维护：

- 当前站点和已访问停靠；
- 可用现金、已实现利润；
- 在途库存（类型、数量、成本、采购站、承诺销售站）；
- 每段已用/剩余货舱；
- 已消耗订单深度；
- 累计跳数、安全、停靠和估算时间。

支持如下过程：

```text
A：采购 X、Y
B：出售 X，继续携带 Y，并采购 Z
C：出售 Y 和部分 Z
D：出售剩余 Z
```

未出售货物不能提前计入已实现利润、释放货舱或释放本金。算法采用有界取送 Beam Search：生成可信任务，尝试顺路插入取货/送货点，逐段校验取货先于送货、现金非负、容量不超限、深度不重复、安全及跳数合规，再以边际净利润、额外跳数、停靠成本和路线风险排序。允许星门拓扑要求的合理回穿，但禁止无收益循环、重复成交与无意义反复交易。

### 15.6 推荐排序与前端信息架构

推荐对象是完整方案而非孤立商品。提供三个优化目标：

- 整船净利润最大（默认）；
- 估算时间效率最高；
- 稳健成交（深度、新鲜度、路线和准入可靠性）。

未知路线、陈旧快照和未验证准入必须降级或剔除。评分只用于比较已经可执行的方案，并返回可解释的正向因素和风险因素。

前端改为：

1. 顶部规划条件：当前位置、区域/地点、预算、预留、货舱、最低安全、最大跳数/停靠、优化目标；
2. 中部方案列表：路线摘要、商品数、装载率、采购资本、整船净利润、总跳数、效率和风险；
3. 下部执行面板：采购清单、真实路线/停靠、收益风险三个页签；
4. 链式每站展示到站现金、卖出回款、本站采购、离站现金，以及卸货/装货前后的货舱变化；
5. 官方中文地点为主显示，ID 仅作次级诊断信息。

### 15.7 实施和验收顺序

1. 新增多档深度 Migration、发布事务摘要构建和查询 Repository；
2. 实现受安全约束的服务端路线服务及真实状态语义；
3. 重写单品数量计算和固定地点对组合整船优化；
4. 实现链式逐站现金/库存/货舱状态和取送 Beam Search；
5. 重构规划 API/DTO 和客户端方案视图；
6. 执行单元、Oracle、集成、性能和生产回归测试；
7. 构建客户端，并在数据库兼容确认后部署服务端。

必须通过：

- 最低卖价深度不足、后续档位仍盈利时继续采购；
- 最高买价深度不足时按后续买单档位计算收入；
- 多商品组合的整船利润优于最佳单品且不跨采购/销售地点；
- 任意时刻预算非负、货舱不超限、订单不重复消耗；
- 链式在出售回款后才能将资金投入下一次采购；
- 未出售货物在每一段持续占用货舱；
- 无安全合规路线时不返回 `0 跳` 或伪造安全值；
- 不能满仓时返回可验证原因；
- 同一输入和快照产生确定性结果；
- 计算状态、候选数量、路线腿数和响应大小均有硬上限。