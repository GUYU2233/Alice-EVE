-- Expand push-token provider allowlist. Existing rows remain compatible.
-- Provider/platform validation is enforced by the service boundary; this check
-- prevents unsupported vendor names from entering durable storage.
ALTER TABLE push_tokens DROP CONSTRAINT IF EXISTS push_tokens_provider_check;
ALTER TABLE push_tokens ADD CONSTRAINT push_tokens_provider_check
  CHECK (provider IN ('fcm', 'xiaomi', 'huawei', 'oppo', 'vivo', 'apns', 'webpush'));
ALTER TABLE push_tokens ADD CONSTRAINT push_tokens_provider_platform_check
  CHECK ((provider = 'fcm' AND platform IN ('android', 'ios', 'web'))
    OR (provider IN ('xiaomi', 'huawei', 'oppo', 'vivo') AND platform = 'android')
    OR (provider = 'apns' AND platform = 'ios')
    OR (provider = 'webpush' AND platform = 'web'));
