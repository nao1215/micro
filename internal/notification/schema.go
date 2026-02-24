package notification

import (
	"database/sql"
	"fmt"
)

// スキーマ定義。db/notification/schema.sql と同期すること。
const schema = `
CREATE TABLE IF NOT EXISTS notifications (
    -- 通知の一意識別子（UUID）
    id TEXT PRIMARY KEY,
    -- 通知先のユーザーID
    user_id TEXT NOT NULL,
    -- 通知のタイトル
    title TEXT NOT NULL,
    -- 通知メッセージ
    message TEXT NOT NULL,
    -- 通知の既読状態
    is_read INTEGER NOT NULL DEFAULT 0,
    -- 通知の作成日時
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

-- ユーザーIDでの検索を高速化するインデックス。
CREATE INDEX IF NOT EXISTS idx_notifications_user_id
    ON notifications(user_id);

-- 未読通知の検索を高速化するインデックス。
CREATE INDEX IF NOT EXISTS idx_notifications_unread
    ON notifications(user_id, is_read) WHERE is_read = 0;
`

// initSchema はSQLiteデータベースにスキーマを適用する。
func initSchema(db *sql.DB) error {
	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("スキーマの適用に失敗: %w", err)
	}
	return nil
}
