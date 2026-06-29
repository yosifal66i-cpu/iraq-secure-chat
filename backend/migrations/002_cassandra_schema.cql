-- IraqSecureChat Cassandra / ScyllaDB Schema
-- For high-throughput message storage (production use)

CREATE KEYSPACE IF NOT EXISTS iraqchat
WITH replication = {
  'class': 'NetworkTopologyStrategy',
  'me-south-1': 3,  -- AWS Bahrain region for Iraq
  'eu-west-1': 2    -- DR in Europe
}
AND durable_writes = true;

USE iraqchat;

-- ============================================
-- MESSAGES TABLE (Primary time-series store)
-- ============================================
CREATE TABLE IF NOT EXISTS messages (
  chat_id       UUID,
  message_id    TIMEUUID,
  sender_id     UUID,
  type          TEXT,        -- text, photo, video, audio, file, sticker, poll
  content       TEXT,        -- message text or JSON for complex types
  reply_to      TIMEUUID,
  forward_from  TEXT,        -- JSON
  media         TEXT,        -- JSON
  poll          TEXT,        -- JSON
  edited_at     TIMESTAMP,
  deleted_for   SET<UUID>,
  deleted_for_all BOOLEAN,
  reactions     MAP<TEXT, SET<UUID>>,
  entities      TEXT,        -- JSON
  schedule_at   TIMESTAMP,
  auto_delete_at TIMESTAMP,
  idempotency_key TEXT,
  PRIMARY KEY ((chat_id), message_id)
) WITH CLUSTERING ORDER BY (message_id DESC)
  AND compaction = {
    'class': 'TimeWindowCompactionStrategy',
    'compaction_window_size': 1,
    'compaction_window_unit': 'DAYS',
    'base_time_seconds': 86400
  }
  AND default_time_to_live = 0
  AND gc_grace_seconds = 864000
  AND caching = {
    'keys': 'ALL',
    'rows_per_partition': '10000'
  }
  AND speculative_retry = '95PERCENTILE';

-- ============================================
-- MESSAGES BY SENDER (for search)
-- ============================================
CREATE TABLE IF NOT EXISTS messages_by_sender (
  sender_id     UUID,
  message_id    TIMEUUID,
  chat_id       UUID,
  type          TEXT,
  content       TEXT,
  PRIMARY KEY ((sender_id), message_id)
) WITH CLUSTERING ORDER BY (message_id DESC)
  AND compaction = {
    'class': 'TimeWindowCompactionStrategy',
    'compaction_window_size': 7,
    'compaction_window_unit': 'DAYS'
  };

-- ============================================
-- USER INBOX (optimized per-user message view)
-- ============================================
CREATE TABLE IF NOT EXISTS user_inbox (
  user_id       UUID,
  chat_id       UUID,
  last_message_id TIMEUUID,
  last_message_type TEXT,
  last_message_content TEXT,
  last_message_sender UUID,
  last_message_at TIMESTAMP,
  unread_count  INT,
  PRIMARY KEY ((user_id), chat_id)
) WITH CLUSTERING ORDER BY (chat_id ASC);

-- ============================================
-- REACTIONS COUNTER TABLE
-- ============================================
CREATE TABLE IF NOT EXISTS reaction_counts (
  chat_id       UUID,
  message_id    TIMEUUID,
  emoji         TEXT,
  count         COUNTER,
  PRIMARY KEY ((chat_id, message_id), emoji)
);

-- ============================================
-- SCHEDULED MESSAGES INDEX
-- ============================================
CREATE TABLE IF NOT EXISTS scheduled_messages (
  schedule_bucket INT,      -- time-based bucket for TTL
  schedule_at   TIMESTAMP,
  message_id    TIMEUUID,
  chat_id       UUID,
  sender_id     UUID,
  type          TEXT,
  content       TEXT,
  media         TEXT,
  PRIMARY KEY ((schedule_bucket), schedule_at, message_id)
) WITH CLUSTERING ORDER BY (schedule_at ASC);

-- ============================================
-- MATERIALIZED VIEWS
-- ============================================
CREATE MATERIALIZED VIEW IF NOT EXISTS messages_by_chat_sender AS
  SELECT * FROM messages
  WHERE chat_id IS NOT NULL AND sender_id IS NOT NULL AND message_id IS NOT NULL
  PRIMARY KEY ((chat_id, sender_id), message_id)
  WITH CLUSTERING ORDER BY (message_id DESC);

-- ============================================
-- USER DEFINED TYPES
-- ============================================
CREATE TYPE IF NOT EXISTS message_reaction (
  user_id   UUID,
  emoji     TEXT,
  reacted_at TIMESTAMP
);

-- ============================================
-- Indexes for common queries
-- ============================================
CREATE INDEX IF NOT EXISTS idx_messages_type ON messages(type);
CREATE INDEX IF NOT EXISTS idx_messages_content ON messages(content) USING 'org.apache.cassandra.index.sasi.analyzer.StandardAnalyzer';
CREATE INDEX IF NOT EXISTS idx_messages_schedule ON messages(schedule_at)
  WHERE schedule_at IS NOT NULL;
