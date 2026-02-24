-- Album スキーマ
-- アルバム管理サービスのデータベース。
-- アルバムとメディアの多対多の関連を管理する。

CREATE TABLE IF NOT EXISTS albums (
    -- アルバムの一意識別子
    id TEXT PRIMARY KEY,
    -- アルバムを作成したユーザーのID
    user_id TEXT NOT NULL,
    -- アルバム名
    name TEXT NOT NULL,
    -- アルバムの説明
    description TEXT NOT NULL DEFAULT '',
    -- 作成日時
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    -- 更新日時
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS album_media (
    -- アルバムID
    album_id TEXT NOT NULL,
    -- メディアID
    media_id TEXT NOT NULL,
    -- 追加日時
    added_at DATETIME NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (album_id, media_id),
    FOREIGN KEY (album_id) REFERENCES albums(id) ON DELETE CASCADE
);

-- ユーザーIDでの検索を高速化するインデックス。
CREATE INDEX IF NOT EXISTS idx_albums_user_id
    ON albums(user_id);

-- メディアIDでの逆引き検索を高速化するインデックス。
CREATE INDEX IF NOT EXISTS idx_album_media_media_id
    ON album_media(media_id);
