ALTER TABLE chat_events ALTER COLUMN message_id DROP NOT NULL;
ALTER TABLE chat_events ADD COLUMN IF NOT EXISTS member_id TEXT;
CREATE INDEX IF NOT EXISTS chat_events_member_idx ON chat_events(member_id, sequence);
