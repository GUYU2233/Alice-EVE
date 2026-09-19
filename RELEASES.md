# Alice-EVE 发布约定

构建产物使用固定名称并由新构建直接覆盖：

```text
releases/Alice-EVE-Desktop.exe
releases/Alice-EVE-Mobile.apk
```

安装后的产品显示名统一为 `AliceEVE`。Android 更新必须保持 `com.aliceeve.mobile`、相同 release 签名并递增 build number。

统一构建：

```powershell
./scripts/build-release.ps1
```

也可使用 `-DesktopOnly` 或 `-MobileOnly`；二者互斥。发布二进制由 `.gitignore` 排除，不提交源码仓库。每次发布记录 SHA-256；移动 release 必须由 Secret Manager/CI 注入正式签名，不允许 debug key。
