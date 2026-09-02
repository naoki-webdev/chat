CREATE TABLE IF NOT EXISTS channel_read_states (
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  channel_id TEXT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
  last_read_sequence BIGINT NOT NULL DEFAULT 0,
  last_read_message_id TEXT REFERENCES messages(id) ON DELETE SET NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, channel_id)
);
ALTER TABLE channel_read_states ADD COLUMN IF NOT EXISTS last_read_message_id TEXT REFERENCES messages(id) ON DELETE SET NULL;
