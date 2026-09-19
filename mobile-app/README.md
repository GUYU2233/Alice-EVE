# AliceEVE Mobile

Flutter 移动客户端，显示名统一为 `AliceEVE`，Android application ID 为 `com.aliceeve.mobile`。

## 能力

- Alice 设备授权、access/refresh 轮换与 `FlutterSecureStorage`。
- REST backlog、WSS 与 `/api/v1/sync` 补偿恢复。
- 告警 ACK、通知偏好、设备撤销和会话消息。
- FCM 已接入 Android 边界；其他厂商推送通过 provider-neutral adapter 扩展。
- 手机端不接收或保存桌面 EVE SSO 凭证。

## 测试

```bash
flutter pub get
flutter analyze
flutter test
```

## 发布 APK

Release 构建拒绝 debug key。CI 或本机 Secret Manager 必须提供：

```text
ALICE_ANDROID_KEYSTORE_FILE
ALICE_ANDROID_KEYSTORE_PASSWORD
ALICE_ANDROID_KEY_ALIAS
ALICE_ANDROID_KEY_PASSWORD
```

执行仓库根目录：

```powershell
./scripts/build-release.ps1 -MobileOnly
```

固定发布文件：

```text
releases/Alice-EVE-Mobile.apk
```

每次发布保持相同 application ID 和签名，并递增 `pubspec.yaml` 的 build number，即可覆盖安装更新。不得提交 keystore、`google-services.json`、推送凭证或原始设备 Token。
