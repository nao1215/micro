-- Media Read Model スキーマ
-- Event Storeのイベントから投影（Projection）して構築される読み取り専用のビュー。
-- 検索・一覧表示に最適化された非正規化データを保持する。
-- このテーブルはいつでも破棄してEvent Storeから再構築可能。

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

-- Projectorのオフセット（最後にポーリングしたイベントのタイムスタンプ）を永続化するテーブル。
CREATE TABLE IF NOT EXISTS projector_offsets (
    id TEXT PRIMARY KEY DEFAULT 'default',
    last_timestamp DATETIME NOT NULL,
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);
