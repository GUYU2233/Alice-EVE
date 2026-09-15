-- Add opaque per-claim lease identity to existing realtime outbox tables.
-- Completion/failure must match this token, preventing stale workers from
-- changing rows that were reclaimed after their lease expired.
ALTER TABLE realtime_outbox ADD COLUMN IF NOT EXISTS claim_token TEXT;
