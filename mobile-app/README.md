# EVE Assistant Mobile

Flutter mobile companion for the EVE Assistant desktop app.

## Implemented server flow

- **Device authorization:** the current UI calls `POST /api/v1/auth/device/start` and `/complete`. Returned device/access/refresh credentials are persisted with `FlutterSecureStorage`; six-digit `/pair` and `/pair/confirm` remain deprecated compatibility methods and are not exposed in the UI.
- **REST backlog:** after pairing, `GET /api/v1/alerts` retrieves the mobile device's saved event backlog. Requests include both `Authorization: Bearer <device token>` and `X-Device-Id`/`deviceId`; optional `cursor` and `limit` parameters are supported.
- **Protocol models:** `MessageEnvelope` accepts the canonical protocol fields (`version`, `messageId`, `createdAt`) and the compact wire fields currently emitted by the Go server (`v`, `id`, `ts`). Alert payloads include event id, severity, source, system, observed/expiry times, and evidence count.
- **ACK:** confirming an alert posts `ackMessageId`, `status: accepted`, `stage: received`, `receivedAt`, and the compatibility `id` to `POST /api/v1/messages/ack`.
- **Read-only requests:** requests use a versioned mobile envelope and never transmit EVE SSO credentials.
- **Reminder preferences:** `GET/PUT /api/v1/notifications/preferences` uses `version` + `etag` optimistic concurrency. `ReminderPreferences` covers global enablement, Intel/market/conversation categories, quiet hours, timezone, sound and vibration.
- **Push tokens:** `POST/DELETE /api/v1/notifications/tokens` registers or revokes FCM/APNs/Web Push tokens. The database stores only a SHA-256 hash; a production dispatcher must be wired to a separate Secret Manager/Vault token resolver before enabling delivery. Raw tokens and provider secrets are never persisted in the repository.
- **Background notifications:** `notifications/notification_service.dart` provides a testable `NotificationPresenter` port and Firebase-compatible `BackgroundMessageHandler`. Android now includes the Firebase Messaging dependency, `google-services` Gradle plugin, an ignored local `google-services.json`, and `AliceFirebaseMessagingService` for FCM token/message callbacks. The service remains token-log-free; server registration still happens through the Flutter API boundary.
- **Provider-neutral Android push abstraction:** `android/app/src/main/kotlin/.../PushAbstraction.kt` defines `PushProvider`, `PushProviderAdapter`, `PushProviderCapabilities`, `PushProviderRegistry`, `NotificationRouter`, and the common incoming/routed message types for FCM, Xiaomi Mi Push, Huawei Push Kit, OPPO PUSH, and vivo Push. The registry is declarative: the default adapter returns `NotConfigured` and never pretends an SDK is initialized.
- **Flutter bridge:** `lib/notifications/push_platform.dart` reuses the API model's canonical provider enum and exposes `MethodChannelPushPlatform` on `eve_assistant_mobile/push`. It lists capabilities, obtains FCM tokens, listens for native token rotation on `eve_assistant_mobile/push_tokens`, and requests Android notification permission. Authenticated token refreshes are forwarded to the existing HTTPS registration endpoint; raw tokens are not persisted by the app.

### Provider integration and permission notes

| Provider | Delivery/adapter boundary | Android requirements |
| --- | --- | --- |
| FCM | Firebase Messaging is linked in the Android build; `MainActivity` obtains the registration token and `AliceFirebaseMessagingService` receives token/message callbacks. | `INTERNET`; `POST_NOTIFICATIONS` runtime permission on Android 13+; project/app configuration is deployment-supplied and its config file must remain ignored. |
| Xiaomi Mi Push | Add the Xiaomi SDK in an Xiaomi-enabled build flavor and map registration/message callbacks to the common adapter. | `INTERNET`; `POST_NOTIFICATIONS` on Android 13+; Xiaomi/MIUI device and app registration, auto-start/background policy review. |
| Huawei Push Kit | Add HMS Core/Push Kit only to Huawei-enabled variants and map callbacks to the common adapter. | `INTERNET`; `POST_NOTIFICATIONS` on Android 13+; HMS Core availability and deployment-supplied Push Kit app configuration. |
| OPPO PUSH | Add the OPPO/Heytap SDK in an OPPO-enabled build flavor and map callbacks to the common adapter. | `INTERNET`; `POST_NOTIFICATIONS` on Android 13+; ColorOS channel and background delivery policy review. |
| vivo Push | Add the vivo SDK in a vivo-enabled build flavor and map callbacks to the common adapter. | `INTERNET`; `POST_NOTIFICATIONS` on Android 13+; vivo service registration and vendor delivery policy review. |

All five providers share the same notification routes (`intel.alert`, `market.alert`, `conversation.update`, or `general`). Provider-specific credentials, app IDs, registration tokens and vendor metadata are deployment/build-flavor inputs. The current FCM build uses an ignored local `android/app/google-services.json`; it must remain untracked, and no raw token or provider secret may be committed. `PushProviderRegistry` documents permissions only and does not initialize optional vendor SDKs.

The server exposes deprecated SSE at `GET /api/v1/events` and WSS at `/api/v1/realtime`. `ConversationClient` implements WSS with `/api/v1/sync` gap recovery, while alert refresh uses REST backlog; the Agent page has not yet wired the conversation realtime client. No Firebase dependency is required for foreground synchronization.

## Directory

```text
lib/
├── main.dart
├── api/api_client.dart
├── models/protocol.dart
├── models/notifications.dart
├── notifications/notification_service.dart
└── intel/intel_parser.dart
test/
├── api_client_test.dart
├── notifications_test.dart
├── intel_parser_test.dart
└── widget_test.dart
```

## Release signing

Release builds intentionally refuse debug-key signing. CI must inject the keystore path and credentials through a secret manager using `ALICE_ANDROID_KEYSTORE_FILE`, `ALICE_ANDROID_KEYSTORE_PASSWORD`, `ALICE_ANDROID_KEY_ALIAS`, and `ALICE_ANDROID_KEY_PASSWORD`. Do not commit a keystore or place these values in Gradle files, shell history, or logs.

Run from `mobile-app`:

```bash
flutter pub get
flutter analyze
flutter test
```

The mobile app communicates with the public Go server and must never directly expose or receive desktop EVE SSO tokens.
