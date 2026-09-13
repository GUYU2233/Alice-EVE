# EVE Assistant Relay Server

Go 中继服务，负责桌面端和手机端之间的安全通信。当前是 MVP 骨架。

## API

- `GET /health`：健康检查
- `POST /api/v1/pair`：生成临时六位配对码（占位实现）
- `POST /api/v1/messages`：发布版本化消息，按 `id` 幂等
- `GET /api/v1/events`：SSE 事件流

## 开发

安装 Go 后执行：

```bash
go test ./...
go run ./cmd/relay-server
```

生产环境必须使用反向代理提供 TLS，并在真正上线前补齐设备认证、配对持久化、WSS、Outbox、ACK、离线消息、限流和 PostgreSQL 存储。
