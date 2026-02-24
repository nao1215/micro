CREATE TABLE IF NOT EXISTS projector_offsets (
    id TEXT PRIMARY KEY DEFAULT 'default',
    last_timestamp DATETIME NOT NULL,
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);
