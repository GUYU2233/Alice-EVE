# EVE Assistant Desktop v0.0.1-alpha

Go + Wails v2 + React/TypeScript desktop application.

## 发布构建

当前 Wails 配置位于 `wails.json`：前端目录为 `frontend`，发布构建会自动执行 `npm run build` 并将资源嵌入 Go 二进制。

### 环境要求

- Go 1.22+
- Wails CLI v2（本次验证：v2.15.0）
- Node.js 18+ 与 npm
- Windows 构建需要 WebView2 Runtime

### 构建命令

```powershell
cd desktop-app/frontend
npm install
npm run build

cd ..
wails build -clean
```

### 下载产物

Windows amd64 发布文件：

- `build/bin/eve-assistant.exe`（约 11.2 MiB）

本次已验证前端 Vite production build 与 `wails build -clean` 均成功。将该 EXE 复制到目标 Windows 电脑后直接运行即可；首次运行需系统已安装 WebView2 Runtime。

## 项目结构

- `internal/app`: 应用服务与生命周期
- `internal/protocol`: 版本化桌面/移动端事件信封
- `internal/storage`: SQLite 抽象与事务边界
- `frontend`: React/TypeScript UI

## 后续工作

1. 添加 SQLite 驱动与迁移。
2. 实现 ESI Gateway、SDE repository、Intel pipeline 与服务端 outbox。
3. 增加调用应用服务的 Wails bindings，避免直接暴露数据库或令牌。
