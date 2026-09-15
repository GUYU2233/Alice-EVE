# Releases

本目录整理当前仓库中已存在的发布产物。扫描范围排除了构建缓存、依赖目录和版本控制目录。

## Desktop App

当前工作区 `releases/` 目录存在以下（被 `.gitignore` 排除、未纳入 Git 对象）产物：

- `releases/eve-assistant-v0.0.1-alpha.exe`
- `releases/eve-assistant-v0.0.1-alpha.zip`

## Mobile

当前工作区 `releases/` 目录存在以下（被 `.gitignore` 排除、未纳入 Git 对象）产物：

- `releases/eve-assistant-mobile-android-v0.0.1-alpha.apk`
- `releases/eve-assistant-mobile-windows.zip`

## Other Existing Binary

历史提交曾包含 `relay-server/relay-linux`、`relay-server/relay-prod`、`relay-server/relay-server/bin/relay-server` 和 `relay-server/relay-server/relay-server.exe`；这些旧服务端二进制不属于客户端发布产物，且当前命名 refs 已不再包含它们。

## Checksums

`releases/SHA256SUMS.txt` 是当前跟踪的清单，列出上述四个工作区产物的 SHA-256；清单中的文件名相对于 `releases/` 目录解析。产物本身被忽略，不应据此声称它们已经提交到 Git 或上传到远端。
