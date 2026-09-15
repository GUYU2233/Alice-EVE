# Updates service

`updates.Service` selects a release from repository metadata for app/platform/architecture/channel queries. `Manifest` validation requires HTTPS artifact URLs, a canonical 64-character hex SHA-256 digest, and signature metadata (`algorithm`, `keyId`, `value`); binary contents and private signing keys never enter this service. Selection ignores withdrawn/invalid releases, honors mandatory minimum-version metadata, and supports explicit rollback selection only when `AllowRollback` is set. `Ack` records download/install status without accepting file content.

`MemoryRepository` is a concurrency-safe test double. A PostgreSQL adapter should map it to `app_releases` and `update_acks` from migration `006_updates.sql`.
