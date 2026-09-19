# Settings service

`settings.Service` is a handler-ready, repository-independent implementation of the settings contract. It validates account/device/profile scope, limits JSON size/depth/key count/array length, rejects secret-like keys and URL values, computes deterministic SHA-256 ETags, and performs optimistic concurrency checks using both `baseVersion` and `If-Match`. A stale write returns `*ConflictError`, whose `Current` document and `MergeHint` can be mapped to the `409 SETTINGS_CONFLICT` response.

`MemoryRepository` is a concurrency-safe test repository. A production adapter should implement `Repository.CompareAndSwap` as an atomic conditional version increment; if it also records `settings_changes`, both operations must share one database transaction.
