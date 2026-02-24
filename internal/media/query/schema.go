package query

import (
	"database/sql"
	"fmt"
)

// スキーマ定義。db/media/schema.sql と同期すること。
// Read Modelは非正規化データで構成され、検索性能に最適化されている。
// このテーブルはいつでも破棄してEvent Storeから再構築可能。
const schema = `
CREATE TABLE IF NOT EXISTS media_read_models (
    -- メディアの一意識別子（AggregateID）
    id TEXT PRIMARY KEY,
    -- アップロードしたユーザーのID
    user_id TEXT NOT NULL,
    -- 元のファイル名
    filename TEXT NOT NULL,
    -- ファイルのMIMEタイプ（image/jpeg, video/mp4 等）
    content_type TEXT NOT NULL,
    -- ファイルサイズ（バイト）
    size INTEGER NOT NULL,
    -- ファイルの保存パス
    storage_path TEXT NOT NULL,
    -- サムネイル画像の保存パス（処理完了前はNULL）
    thumbnail_path TEXT,
    -- 画像/動画の幅（ピクセル、処理完了前はNULL）
    width INTEGER,
    -- 画像/動画の高さ（ピクセル、処理完了前はNULL）
    height INTEGER,
    -- 動画の長さ（秒、画像の場合はNULL）
    duration_seconds REAL,
    -- メディアの状態（uploaded, processed, failed, deleted）
    status TEXT NOT NULL DEFAULT 'uploaded',
    -- 最後に適用されたイベントのバージョン番号
    last_event_version INTEGER NOT NULL DEFAULT 0,
    -- アップロード日時
    uploaded_at DATETIME NOT NULL,
    -- Read Model更新日時
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

-- ユーザーIDでの検索を高速化するインデックス。
CREATE INDEX IF NOT EXISTS idx_media_user_id
    ON media_read_models(user_id);

-- ステータスでのフィルタリングを高速化するインデックス。
CREATE INDEX IF NOT EXISTS idx_media_status
    ON media_read_models(status);

-- Content-Typeでのフィルタリングを高速化するインデックス。
CREATE INDEX IF NOT EXISTS idx_media_content_type
    ON media_read_models(content_type);
`

// initSchema はSQLiteデータベースにRead Modelのスキーマを適用する。
// テーブルとインデックスが存在しない場合のみ作成する。
func initSchema(db *sql.DB) error {
	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("Read Modelスキーマの適用に失敗: %w", err)
	}
	return nil
}
