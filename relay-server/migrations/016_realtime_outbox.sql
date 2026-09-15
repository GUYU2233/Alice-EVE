-- Transactional realtime outbox. Domain repositories insert an intent in the
-- same transaction as the conversation/run mutation; a dispatcher later turns
-- it into delivery_events with the stable event ID.
CREATE TABLE IF NOT EXISTS realtime_outbox (
  id TEXT PRIMARY KEY,
  account_id TEXT NOT NULL,
  recipient_device_id TEXT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
  event_type TEXT NOT NULL,
  payload JSONB NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','processing','published','failed')),
  attempts INTEGER NOT NULL DEFAULT 0,
  last_error TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  claim_token TEXT,
  published_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS realtime_outbox_pending_idx
  ON realtime_outbox(status, available_at, created_at);
CREATE UNIQUE INDEX IF NOT EXISTS realtime_outbox_target_event_idx
  ON realtime_outbox(account_id, recipient_device_id, id);

-- Wake the materializer after a committed transactional intent. Notifications
-- are hints only; the worker always retains a polling fallback.
CREATE OR REPLACE FUNCTION notify_realtime_outbox() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  PERFORM pg_notify('relay_realtime_wakeup', json_build_object('accountId', NEW.account_id, 'deviceId', NEW.recipient_device_id)::text);
  RETURN NEW;
END;
$$;
DROP TRIGGER IF EXISTS realtime_outbox_wakeup ON realtime_outbox;
CREATE TRIGGER realtime_outbox_wakeup
AFTER INSERT ON realtime_outbox
FOR EACH ROW EXECUTE FUNCTION notify_realtime_outbox();
