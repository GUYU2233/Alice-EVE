# Shared Protocol

本目录是桌面端、Go Relay Server、手机端之间通信协议的共享源目录。

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

1. JSON Schema/OpenAPI 是唯一真源，禁止手工维护跨语言模型的不同字段定义。
2. `protocol.md` 记录语义、状态机、安全和兼容策略。
3. 新增字段必须可选且保持旧客户端可解析；破坏性变更提升主版本。
4. 生成 Go、TypeScript、Dart 模型后，CI 执行示例解析和跨语言 round-trip 测试。
5. 生成物提交到仓库，便于桌面端、Relay 和手机端独立构建；生成脚本应固定工具版本。
6. 禁止在模型中出现 EVE Token、Authorization、任意 SQL、任意命令或未脱敏私密原文。

## 当前状态

已补充 `envelope.schema.json`、`messages.schema.json`、`pairing.schema.json`、`errors.schema.json` 和 `openapi.yaml`。协议正文见 [`docs/protocol.md`](../../docs/protocol.md)。下一步补充生成脚本，然后生成 Go/TypeScript/Dart DTO。
