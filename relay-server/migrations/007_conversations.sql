-- Account-scoped Agent conversations and durable conversation messages.
-- account_id remains TEXT in this compatibility migration because the original
-- schema predates the accounts table; a later account migration may add the FK.
CREATE TABLE IF NOT EXISTS conversations (
  id TEXT PRIMARY KEY,
  account_id TEXT NOT NULL,
  target_device_id TEXT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
  agent_kind TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'active'
    CHECK (status IN ('active', 'closed', 'cancelled')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS conversations_account_updated_idx
  ON conversations(account_id, updated_at DESC);
CREATE INDEX IF NOT EXISTS conversations_target_idx
  ON conversations(target_device_id, status);

CREATE TABLE IF NOT EXISTS conversation_messages (
  id TEXT PRIMARY KEY,
  conversation_id TEXT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
  sender_kind TEXT NOT NULL CHECK (sender_kind IN ('mobile', 'desktop', 'server')),
  sender_device_id TEXT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
  client_message_id TEXT NOT NULL,
  operation TEXT NOT NULL,
  body TEXT NOT NULL CHECK (octet_length(body) <= 262144),
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  status TEXT NOT NULL DEFAULT 'persisted'
    CHECK (status IN ('accepted', 'persisted', 'delivered', 'processed', 'failed')),
  cursor BIGSERIAL NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  expires_at TIMESTAMPTZ,
  UNIQUE(conversation_id, sender_device_id, client_message_id)
);
CREATE UNIQUE INDEX IF NOT EXISTS conversation_messages_cursor_idx
  ON conversation_messages(conversation_id, cursor);
CREATE INDEX IF NOT EXISTS conversation_messages_page_idx
  ON conversation_messages(conversation_id, cursor);
CREATE INDEX IF NOT EXISTS conversation_messages_status_idx
  ON conversation_messages(conversation_id, status);
