CREATE TABLE IF NOT EXISTS albums (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS album_media (
    album_id TEXT NOT NULL,
    media_id TEXT NOT NULL,
    added_at DATETIME NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (album_id, media_id),
    FOREIGN KEY (album_id) REFERENCES albums(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_albums_user_id
    ON albums(user_id);

CREATE INDEX IF NOT EXISTS idx_album_media_media_id
    ON album_media(media_id);
