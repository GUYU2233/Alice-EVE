# Alice-EVE Server

Go 服务端，负责账户与设备安全、EVE SSO/ESI、公共数据、市场采集、持久化贸易规划、路线、消息和实时同步。`relay-server/` 与 `cmd/relay-server` 名称为兼容历史路径保留。

## 核心接口

- `/health`：健康检查。
- `/api/v1/auth/*`：设备授权、会话轮换和 EVE SSO/PKCE。
- `/api/v1/eve/*`：账户隔离的角色快照与详情。
- `/api/v1/trade/candidates/search`、`/trade/routes`：候选和安全路线。
- `/api/v1/trade/plan-jobs`：创建持久规划任务。
- `/api/v1/trade/plan-jobs/{id}`、`/{id}/results?afterRevision=N`：状态和增量结果。
- `/api/v1/realtime`、`/api/v1/sync`：实时通知与可靠补偿；旧 SSE 仅兼容保留。

## 市场规划

`internal/marketplan` 使用 PostgreSQL jobs/results/frontiers、租约 fencing 和并发 Worker。起点区域只约束采购，默认可向全部已采集区域出售。SINGLE、BASKET、CHAIN 输出 canonical payload；市场快照更新后 watching 任务重新排队。

完整说明见 [`../docs/architecture.md`](../docs/architecture.md)。

## 开发

```bash
go test ./... -count=1
go vet ./...
go run ./cmd/relay-server
```

配置 `DATABASE_URL` 时启动器按 checksum 和 advisory lock 应用迁移。生产应使用 TLS 反向代理、私有 PostgreSQL、最小权限账户、限流和 Secret Manager。环境变量示例位于 `config/production.env.example`；真实域名、地址、DSN 和密钥不得进入仓库。
