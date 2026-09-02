ALTER TABLE channels
  ADD COLUMN IF NOT EXISTS dm_peer_user_id TEXT REFERENCES users(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS channels_dm_peer_user_idx
  ON channels(dm_peer_user_id)
  WHERE kind = 'dm';
