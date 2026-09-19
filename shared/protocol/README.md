# Shared Protocol

本目录是桌面端、Go 服务端（Server）、手机端之间通信协议的共享源目录。Relay 仅作为历史兼容称呼保留，不改变现有字段或 API 行为。

## 文件规划

```text
shared/protocol/
├── envelope.schema.json
├── messages.schema.json
├── errors.schema.json
├── pairing.schema.json
├── openapi.yaml
├── examples/
└── generated/
    ├── go/
    ├── typescript/
    └── dart/
```

## 约定

1. 当前 JSON Schema/OpenAPI 是兼容草案；实际实现包含短字段 Envelope 和端点 DTO。完成路由/字段对齐、固定生成工具和 round-trip CI 后，才能升级为唯一真源。
2. `protocol.md` 记录语义、状态机、安全和兼容策略。
3. 新增字段必须可选且保持旧客户端可解析；破坏性变更提升主版本。
4. 生成 Go、TypeScript、Dart 模型后，CI 执行示例解析和跨语言 round-trip 测试。
5. 生成物提交到仓库，便于桌面端、服务端和手机端独立构建；生成脚本应固定工具版本。
6. 禁止在模型中出现 EVE Token、Authorization、任意 SQL、任意命令或未脱敏私密原文。

## 当前状态

已补充 `envelope.schema.json`、`messages.schema.json`、`pairing.schema.json`、`errors.schema.json` 和 `openapi.yaml`。协议正文见 [`docs/protocol.md`](../../docs/protocol.md)。Schema 仍主要描述兼容消息，Go/TypeScript/Dart 业务 DTO 仍有手写实现；下一步先对齐实际 API，再补生成脚本和跨语言 round-trip 测试。
