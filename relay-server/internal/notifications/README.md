# Notification settings and push token abstraction

`notifications.Service` provides a provider-neutral domain boundary:

- Device-scoped `ReminderPreferences` with optimistic version/ETag updates.
- FCM, Xiaomi, Huawei, OPPO, Vivo, APNs, and Web Push token registration with provider/platform validation. The raw token is hashed with SHA-256 immediately; only `token_hash` is persisted by a durable adapter.
- Provider metadata is available from `ProviderMetadata()` and the API provider catalog. Android-only vendors (Xiaomi, Huawei, OPPO, Vivo) reject iOS/web platforms; APNs accepts iOS, Web Push accepts web, and FCM accepts Android/iOS/web.
- Provider-neutral `Dispatcher`/`Delivery` interfaces are intentionally adapter-only. Dispatch returns `ErrProviderNotConfigured` until a provider adapter is explicitly registered; no vendor send is faked.
- Idempotent token registration (same device/provider/token updates the existing record) and owner-scoped revocation.

HTTP routes are authenticated by the paired mobile device token:

- `GET /api/v1/notifications/preferences`
- `PUT /api/v1/notifications/preferences` body `{preferences, baseVersion, ifMatch}`
- `GET /api/v1/notifications/providers` (provider metadata and configured state)
- `GET/POST/DELETE /api/v1/notifications/tokens` (`DELETE` takes query `id`)

The current service is in-memory so tests and the MVP server do not require Firebase credentials. The SQL migration (`009_notifications.sql`) defines the durable shape for a PostgreSQL repository.

## FCM production boundary

`FCMDispatcher` is an explicit HTTP v1 adapter. It is constructed with:

- `FCMConfigFromEnv`, which reads only non-secret controls (`FCM_ENABLED`, `FCM_PROJECT_ID`, endpoint, timeout and retry limits).
- `FCMAccessTokenSource`, normally created with `NewSecretManagerAccessTokenSource` or a workload-identity implementation.
- `FCMTokenResolver`, a vault-backed hash-to-token lookup. Raw FCM tokens are never stored in the SQL notification row.

Credential-shaped environment variables (`FCM_ACCESS_TOKEN`, `GOOGLE_APPLICATION_CREDENTIALS`, service-account JSON, etc.) are rejected. Production endpoints must use HTTPS; loopback endpoints are accepted only for tests. Dispatch applies one overall context timeout, retries only transport/429/5xx failures with bounded exponential backoff and `Retry-After`, and emits provider/result-only metrics without token or payload labels. No constructor performs a network call, and tests use `httptest` only—there are no real credentials or vendor sends in this repository.
