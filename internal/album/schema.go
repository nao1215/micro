package album

import (
	"database/sql"
	"fmt"
)

// スキーマ定義。db/album/schema.sql と同期すること。
const schema = `
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
`

// initSchema はSQLiteデータベースにスキーマを適用する。
func initSchema(db *sql.DB) error {
	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("スキーマの適用に失敗: %w", err)
	}
	return nil
}
