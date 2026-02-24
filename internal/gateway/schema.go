package gateway

import (
	"database/sql"
	"fmt"
)

// スキーマ定義。db/gateway/schema.sql と同期すること。
const schema = `
CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    provider TEXT NOT NULL,
    provider_user_id TEXT NOT NULL,
    email TEXT NOT NULL,
    display_name TEXT NOT NULL,
    avatar_url TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    last_login_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_users_provider
    ON users(provider, provider_user_id);

CREATE INDEX IF NOT EXISTS idx_users_email
    ON users(email);
`

// initSchema はSQLiteデータベースにスキーマを適用する。
func initSchema(db *sql.DB) error {
	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("スキーマの適用に失敗: %w", err)
	}
	return nil
}
