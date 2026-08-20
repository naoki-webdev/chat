ALTER TABLE messages ADD COLUMN IF NOT EXISTS created_sequence BIGINT NOT NULL DEFAULT 0;
CREATE INDEX IF NOT EXISTS messages_channel_created_idx ON messages(channel_id, created_sequence, id);

CREATE TABLE IF NOT EXISTS chat_events (
  sequence BIGSERIAL PRIMARY KEY,
  event_type TEXT NOT NULL,
  channel_id TEXT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
  message_id TEXT NOT NULL,
  payload JSONB,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS chat_events_sequence_idx ON chat_events(sequence);
CREATE INDEX IF NOT EXISTS chat_events_channel_sequence_idx ON chat_events(channel_id, sequence);
