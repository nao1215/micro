CREATE TABLE IF NOT EXISTS media_read_models (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    filename TEXT NOT NULL,
    content_type TEXT NOT NULL,
    size INTEGER NOT NULL,
    storage_path TEXT NOT NULL,
    thumbnail_path TEXT,
    width INTEGER,
    height INTEGER,
    duration_seconds REAL,
    status TEXT NOT NULL DEFAULT 'uploaded',
    last_event_version INTEGER NOT NULL DEFAULT 0,
    uploaded_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_media_user_id
    ON media_read_models(user_id);

CREATE INDEX IF NOT EXISTS idx_media_status
    ON media_read_models(status);

CREATE INDEX IF NOT EXISTS idx_media_content_type
    ON media_read_models(content_type);
