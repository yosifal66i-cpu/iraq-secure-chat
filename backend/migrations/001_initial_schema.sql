-- IraqSecureChat Database Schema
-- PostgreSQL 16 Migration: 001_initial_schema.sql

-- Enable UUID generation
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- ============================================
-- USERS
-- ============================================
CREATE TABLE users (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    phone       TEXT UNIQUE,
    email       TEXT UNIQUE,
    username    TEXT UNIQUE,
    display_name TEXT NOT NULL DEFAULT '',
    bio         TEXT DEFAULT '',
    avatar_url  TEXT DEFAULT '',
    premium     BOOLEAN DEFAULT FALSE,
    last_seen   TIMESTAMPTZ DEFAULT NOW(),
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    settings    JSONB DEFAULT '{
        "last_seen_privacy": "everyone",
        "profile_photo_privacy": "everyone",
        "phone_privacy": "contacts",
        "group_add_privacy": "everyone",
        "show_forwarded_sender": true,
        "notifications_enabled": true,
        "dnd_enabled": false,
        "language": "ar"
    }'::jsonb
);

CREATE INDEX idx_users_phone ON users(phone);
CREATE INDEX idx_users_username ON users(username);
CREATE INDEX idx_users_display_name ON users USING gin(display_name gin_trgm_ops);
CREATE INDEX idx_users_created_at ON users(created_at DESC);

-- ============================================
-- CONTACTS
-- ============================================
CREATE TABLE contacts (
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    contact_id  UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    added_at    TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (user_id, contact_id)
);

CREATE INDEX idx_contacts_user ON contacts(user_id);

-- ============================================
-- BLOCKED USERS
-- ============================================
CREATE TABLE blocked_users (
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    blocked_id  UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (user_id, blocked_id)
);

-- ============================================
-- CHATS (Groups, Channels, Private)
-- ============================================
CREATE TABLE chats (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    type        TEXT NOT NULL CHECK (type IN ('private','group','supergroup','channel','bot')),
    title       TEXT NOT NULL DEFAULT '',
    username    TEXT UNIQUE,
    description TEXT DEFAULT '',
    avatar_url  TEXT DEFAULT '',
    created_by  UUID REFERENCES users(id),
    settings    JSONB DEFAULT '{
        "slow_mode_seconds": 0,
        "sign_messages": false,
        "join_by_link": true,
        "hidden_members": false,
        "no_forwards": false,
        "topics_enabled": false,
        "auto_delete_message_ttl": 0
    }'::jsonb,
    created_at  TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_chats_type ON chats(type);
CREATE INDEX idx_chats_username ON chats(username);
CREATE INDEX idx_chats_created_at ON chats(created_at DESC);

-- ============================================
-- CHAT MEMBERS
-- ============================================
CREATE TABLE chat_members (
    chat_id     UUID NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role        TEXT DEFAULT 'member' CHECK (role IN ('owner','admin','member','restricted','banned')),
    permissions JSONB DEFAULT '{
        "can_send_messages": true,
        "can_send_media": true,
        "can_send_stickers": true,
        "can_send_polls": true,
        "can_add_members": false,
        "can_pin_messages": false,
        "can_change_info": false,
        "can_delete_messages": false
    }'::jsonb,
    joined_at   TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (chat_id, user_id)
);

CREATE INDEX idx_chat_members_user ON chat_members(user_id);
CREATE INDEX idx_chat_members_role ON chat_members(chat_id, role);

-- ============================================
-- MESSAGES (Primary store — for production use Cassandra/ScyllaDB)
-- ============================================
CREATE TABLE messages (
    id              UUID NOT NULL,
    chat_id         UUID NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    sender_id       UUID NOT NULL REFERENCES users(id),
    type            TEXT NOT NULL DEFAULT 'text',
    content         TEXT DEFAULT '',
    reply_to        UUID,
    forward_from    JSONB,
    media           JSONB,
    poll            JSONB,
    entities        JSONB DEFAULT '[]'::jsonb,
    edited_at       TIMESTAMPTZ,
    deleted_for     UUID[] DEFAULT '{}',
    deleted_for_all BOOLEAN DEFAULT FALSE,
    reactions       JSONB DEFAULT '{}'::jsonb,
    sent_at         TIMESTAMPTZ DEFAULT NOW(),
    schedule_at     TIMESTAMPTZ,
    auto_delete_at  TIMESTAMPTZ,
    idempotency_key TEXT,
    PRIMARY KEY (chat_id, id, sent_at)
) PARTITION BY RANGE (sent_at);

-- Create monthly partitions for messages
CREATE TABLE messages_2024_01 PARTITION OF messages
    FOR VALUES FROM ('2024-01-01') TO ('2024-02-01');
CREATE TABLE messages_2024_02 PARTITION OF messages
    FOR VALUES FROM ('2024-02-01') TO ('2024-03-01');
CREATE TABLE messages_2024_03 PARTITION OF messages
    FOR VALUES FROM ('2024-03-01') TO ('2024-04-01');
CREATE TABLE messages_2024_04 PARTITION OF messages
    FOR VALUES FROM ('2024-04-01') TO ('2024-05-01');
CREATE TABLE messages_2024_05 PARTITION OF messages
    FOR VALUES FROM ('2024-05-01') TO ('2024-06-01');
CREATE TABLE messages_2024_06 PARTITION OF messages
    FOR VALUES FROM ('2024-06-01') TO ('2024-07-01');
CREATE TABLE messages_2024_07 PARTITION OF messages
    FOR VALUES FROM ('2024-07-01') TO ('2024-08-01');
CREATE TABLE messages_2024_08 PARTITION OF messages
    FOR VALUES FROM ('2024-08-01') TO ('2024-09-01');
CREATE TABLE messages_2024_09 PARTITION OF messages
    FOR VALUES FROM ('2024-09-01') TO ('2024-10-01');
CREATE TABLE messages_2024_10 PARTITION OF messages
    FOR VALUES FROM ('2024-10-01') TO ('2024-11-01');
CREATE TABLE messages_2024_11 PARTITION OF messages
    FOR VALUES FROM ('2024-11-01') TO ('2024-12-01');
CREATE TABLE messages_2024_12 PARTITION OF messages
    FOR VALUES FROM ('2024-12-01') TO ('2025-01-01');
CREATE TABLE messages_2025_01 PARTITION OF messages
    FOR VALUES FROM ('2025-01-01') TO ('2025-02-01');
CREATE TABLE messages_2025_02 PARTITION OF messages
    FOR VALUES FROM ('2025-02-01') TO ('2025-03-01');
CREATE TABLE messages_2025_03 PARTITION OF messages
    FOR VALUES FROM ('2025-03-01') TO ('2025-04-01');
CREATE TABLE messages_2025_04 PARTITION OF messages
    FOR VALUES FROM ('2025-04-01') TO ('2025-05-01');
CREATE TABLE messages_2025_05 PARTITION OF messages
    FOR VALUES FROM ('2025-05-01') TO ('2025-06-01');
CREATE TABLE messages_2025_06 PARTITION OF messages
    FOR VALUES FROM ('2025-06-01') TO ('2025-07-01');
CREATE TABLE messages_2025_07 PARTITION OF messages
    FOR VALUES FROM ('2025-07-01') TO ('2025-08-01');
CREATE TABLE messages_2025_08 PARTITION OF messages
    FOR VALUES FROM ('2025-08-01') TO ('2025-09-01');
CREATE TABLE messages_2025_09 PARTITION OF messages
    FOR VALUES FROM ('2025-09-01') TO ('2025-10-01');
CREATE TABLE messages_2025_10 PARTITION OF messages
    FOR VALUES FROM ('2025-10-01') TO ('2025-11-01');
CREATE TABLE messages_2025_11 PARTITION OF messages
    FOR VALUES FROM ('2025-11-01') TO ('2025-12-01');
CREATE TABLE messages_2025_12 PARTITION OF messages
    FOR VALUES FROM ('2025-12-01') TO ('2026-01-01');

-- Default partition for future dates
CREATE TABLE messages_default PARTITION OF messages DEFAULT;

CREATE INDEX idx_messages_chat_sent_at ON messages(chat_id, sent_at DESC);
CREATE INDEX idx_messages_sender ON messages(sender_id);
CREATE INDEX idx_messages_type ON messages(chat_id, type);
CREATE INDEX idx_messages_content_search ON messages USING gin(to_tsvector('arabic', content));
CREATE INDEX idx_messages_schedule ON messages(schedule_at) WHERE schedule_at IS NOT NULL;
CREATE INDEX idx_messages_auto_delete ON messages(auto_delete_at) WHERE auto_delete_at IS NOT NULL;

-- ============================================
-- MEDIA FILES
-- ============================================
CREATE TABLE media (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type          TEXT NOT NULL CHECK (type IN ('photo','video','audio','file','sticker','gif')),
    mime_type     TEXT NOT NULL,
    file_name     TEXT NOT NULL,
    file_size     BIGINT NOT NULL,
    url           TEXT NOT NULL,
    thumbnail_url TEXT DEFAULT '',
    width         INT DEFAULT 0,
    height        INT DEFAULT 0,
    duration      INT DEFAULT 0,
    status        TEXT DEFAULT 'uploading' CHECK (status IN ('uploading','processing','ready','failed')),
    created_at    TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_media_user ON media(user_id);
CREATE INDEX idx_media_type ON media(type);
CREATE INDEX idx_media_status ON media(status);

-- ============================================
-- CALLS (Voice/Video)
-- ============================================
CREATE TABLE calls (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    caller_id   UUID NOT NULL REFERENCES users(id),
    callee_id   UUID NOT NULL REFERENCES users(id),
    type        TEXT NOT NULL CHECK (type IN ('audio','video')),
    status      TEXT DEFAULT 'initiated' CHECK (status IN ('initiated','accepted','rejected','ended','missed')),
    started_at  TIMESTAMPTZ DEFAULT NOW(),
    ended_at    TIMESTAMPTZ,
    duration    INT DEFAULT 0 -- seconds
);

CREATE INDEX idx_calls_caller ON calls(caller_id);
CREATE INDEX idx_calls_callee ON calls(callee_id);
CREATE INDEX idx_calls_status ON calls(status);

-- ============================================
-- BOTS
-- ============================================
CREATE TABLE bots (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token       TEXT UNIQUE NOT NULL,
    name        TEXT NOT NULL,
    username    TEXT UNIQUE NOT NULL,
    description TEXT DEFAULT '',
    about       TEXT DEFAULT '',
    commands    JSONB DEFAULT '[]'::jsonb,
    webhook_url TEXT,
    webhook_secret TEXT,
    inline_capable BOOLEAN DEFAULT FALSE,
    created_at  TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_bots_token ON bots(token);
CREATE INDEX idx_bots_username ON bots(username);

-- ============================================
-- STORIES / STATUS
-- ============================================
CREATE TABLE stories (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type        TEXT DEFAULT 'text' CHECK (type IN ('text','photo','video')),
    content     TEXT,
    media_id    UUID REFERENCES media(id),
    expires_at  TIMESTAMPTZ DEFAULT NOW() + INTERVAL '24 hours',
    created_at  TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_stories_user ON stories(user_id);
CREATE INDEX idx_stories_expires ON stories(expires_at);

-- ============================================
-- STORY VIEWS
-- ============================================
CREATE TABLE story_views (
    story_id    UUID NOT NULL REFERENCES stories(id) ON DELETE CASCADE,
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    viewed_at   TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (story_id, user_id)
);

-- ============================================
-- CHAT FOLDERS
-- ============================================
CREATE TABLE chat_folders (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title       TEXT NOT NULL,
    chat_ids    UUID[] DEFAULT '{}',
    icon        TEXT DEFAULT '',
    position    INT DEFAULT 0,
    created_at  TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_chat_folders_user ON chat_folders(user_id);

-- ============================================
-- SCHEDULED MESSAGES
-- ============================================
CREATE TABLE scheduled_messages (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    chat_id     UUID NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    sender_id   UUID NOT NULL REFERENCES users(id),
    type        TEXT DEFAULT 'text',
    content     TEXT,
    media_id    UUID REFERENCES media(id),
    schedule_at TIMESTAMPTZ NOT NULL,
    processed   BOOLEAN DEFAULT FALSE,
    created_at  TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_scheduled_at ON scheduled_messages(schedule_at) WHERE processed = FALSE;

-- ============================================
-- DEVICE TOKENS (Push Notifications)
-- ============================================
CREATE TABLE device_tokens (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token       TEXT NOT NULL,
    platform    TEXT NOT NULL CHECK (platform IN ('ios','android','web')),
    app_version TEXT DEFAULT '',
    is_active   BOOLEAN DEFAULT TRUE,
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    updated_at  TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_device_tokens_user ON device_tokens(user_id);
CREATE UNIQUE INDEX idx_device_tokens_token ON device_tokens(token);

-- ============================================
-- AUDIT LOG (Security compliance)
-- ============================================
CREATE TABLE audit_log (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID REFERENCES users(id),
    action      TEXT NOT NULL,
    resource    TEXT,
    resource_id TEXT,
    details     JSONB DEFAULT '{}',
    ip_address  TEXT,
    user_agent  TEXT,
    created_at  TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_audit_log_user ON audit_log(user_id);
CREATE INDEX idx_audit_log_action ON audit_log(action);
CREATE INDEX idx_audit_log_created ON audit_log(created_at DESC);

-- ============================================
-- SECRET CHATS (E2EE Session Info)
-- ============================================
CREATE TABLE secret_chat_sessions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id_a       UUID NOT NULL REFERENCES users(id),
    user_id_b       UUID NOT NULL REFERENCES users(id),
    chat_id         UUID NOT NULL REFERENCES chats(id),
    identity_key_a  TEXT NOT NULL,
    identity_key_b  TEXT NOT NULL,
    session_id_a    TEXT,
    session_id_b    TEXT,
    is_active       BOOLEAN DEFAULT TRUE,
    created_at      TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_secret_chat_users ON secret_chat_sessions(user_id_a, user_id_b);

-- ============================================
-- FUNCTION: Auto-create user settings on insert
-- ============================================
CREATE OR REPLACE FUNCTION update_last_seen()
RETURNS TRIGGER AS $$
BEGIN
    NEW.last_seen = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- ============================================
-- TRIGGER for messages: cleanup deleted messages
-- ============================================
CREATE OR REPLACE FUNCTION cleanup_expired_messages()
RETURNS void AS $$
BEGIN
    DELETE FROM messages WHERE auto_delete_at IS NOT NULL AND auto_delete_at < NOW();
END;
$$ LANGUAGE plpgsql;

-- Schedule cleanup every hour
SELECT cron.schedule('cleanup-expired-messages', '0 * * * *', 'SELECT cleanup_expired_messages();');

-- ============================================
-- GRANTS (Role-based)
-- ============================================
CREATE ROLE iraqchat_app WITH LOGIN PASSWORD 'change_me_in_production';
GRANT CONNECT ON DATABASE iraqchat TO iraqchat_app;
GRANT USAGE ON SCHEMA public TO iraqchat_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO iraqchat_app;
GRANT USAGE ON ALL SEQUENCES IN SCHEMA public TO iraqchat_app;

-- ============================================
-- COMMENTS
-- ============================================
COMMENT ON TABLE users IS 'User accounts with support for Iraqi phone numbers (+964)';
COMMENT ON TABLE messages IS 'Messages stored with Arabic full-text search support';
COMMENT ON TABLE audit_log IS 'Security audit trail for government compliance';
COMMENT ON TABLE secret_chat_sessions IS 'E2EE session keys using Signal Protocol';
