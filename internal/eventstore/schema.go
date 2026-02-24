package eventstore

import (
	"database/sql"
	"fmt"
)

// スキーマ定義。db/eventstore/schema.sql と同期すること。
const schema = `
CREATE TABLE IF NOT EXISTS events (
    id TEXT PRIMARY KEY,
    aggregate_id TEXT NOT NULL,
    aggregate_type TEXT NOT NULL,
    event_type TEXT NOT NULL,
    data TEXT NOT NULL,
    version INTEGER NOT NULL,
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_events_aggregate_version
    ON events(aggregate_id, version);

CREATE INDEX IF NOT EXISTS idx_events_event_type
    ON events(event_type);

CREATE INDEX IF NOT EXISTS idx_events_created_at
    ON events(created_at);
`

// initSchema はSQLiteデータベースにスキーマを適用する。
func initSchema(db *sql.DB) error {
	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("スキーマの適用に失敗: %w", err)
	}
	return nil
}
