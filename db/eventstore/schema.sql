-- Event Store スキーマ
-- すべてのサービスの状態変更をイベントとして永続化する。
-- イベントは不変（immutable）であり、追記のみ（append-only）で運用する。

CREATE TABLE IF NOT EXISTS events (
    -- イベントの一意識別子（UUID）
    id TEXT PRIMARY KEY,
    -- 対象エンティティの識別子（例: media-001, album-001）
    aggregate_id TEXT NOT NULL,
    -- 対象エンティティの種類（Media, Album, User）
    aggregate_type TEXT NOT NULL,
    -- イベントの種類（MediaUploaded, AlbumCreated 等）
    event_type TEXT NOT NULL,
    -- イベント固有のデータ（JSON形式）
    data TEXT NOT NULL,
    -- Aggregate内でのイベント順序番号。楽観的排他制御に使用する。
    version INTEGER NOT NULL,
    -- イベント作成日時（UTC）
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

-- AggregateIDとVersionの組み合わせで一意制約を設ける。
-- 同じAggregateに対して同じVersionのイベントが二重に書き込まれることを防ぐ。
CREATE UNIQUE INDEX IF NOT EXISTS idx_events_aggregate_version
    ON events(aggregate_id, version);

-- イベントタイプでの検索を高速化するインデックス。
-- Sagaオーケストレータが特定のイベントタイプをポーリングする際に使用する。
CREATE INDEX IF NOT EXISTS idx_events_event_type
    ON events(event_type);

-- 作成日時でのソートを高速化するインデックス。
-- イベントの時系列順での取得に使用する。
CREATE INDEX IF NOT EXISTS idx_events_created_at
    ON events(created_at);
