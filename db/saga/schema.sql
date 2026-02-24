-- Saga 状態管理スキーマ
-- Orchestration Sagaの実行状態を管理する。
-- 各Sagaインスタンスの現在のステップ、ステータス、補償履歴を記録する。

CREATE TABLE IF NOT EXISTS sagas (
    -- Sagaインスタンスの一意識別子（UUID）
    id TEXT PRIMARY KEY,
    -- Sagaの種類（media_upload 等）
    saga_type TEXT NOT NULL,
    -- 現在のステップ名
    current_step TEXT NOT NULL,
    -- Sagaの状態（started, in_progress, completed, failed, compensating, compensated）
    status TEXT NOT NULL DEFAULT 'started',
    -- Sagaに関連するデータ（JSON形式）。各ステップの結果を蓄積する。
    payload TEXT NOT NULL DEFAULT '{}',
    -- Saga開始日時
    started_at DATETIME NOT NULL DEFAULT (datetime('now')),
    -- 最終更新日時
    updated_at DATETIME NOT NULL DEFAULT (datetime('now')),
    -- 完了日時（未完了の場合はNULL）
    completed_at DATETIME
);

-- Saga内の各ステップの実行履歴を記録するテーブル。
-- 成功したステップと失敗したステップ、補償アクションの実行結果を記録する。
CREATE TABLE IF NOT EXISTS saga_steps (
    -- ステップ履歴の一意識別子
    id TEXT PRIMARY KEY,
    -- 所属するSagaのID
    saga_id TEXT NOT NULL,
    -- ステップ名（upload_file, generate_thumbnail, add_to_album, send_notification）
    step_name TEXT NOT NULL,
    -- ステップの状態（pending, executing, completed, failed, compensating, compensated）
    status TEXT NOT NULL DEFAULT 'pending',
    -- ステップの実行結果やエラー情報（JSON形式）
    result TEXT NOT NULL DEFAULT '{}',
    -- 実行開始日時
    started_at DATETIME,
    -- 完了日時
    completed_at DATETIME,
    -- リトライ回数
    retry_count INTEGER NOT NULL DEFAULT 0,
    -- 最後のエラーメッセージ
    last_error TEXT NOT NULL DEFAULT '',
    FOREIGN KEY (saga_id) REFERENCES sagas(id) ON DELETE CASCADE
);

-- Sagaの状態でのフィルタリングを高速化するインデックス。
CREATE INDEX IF NOT EXISTS idx_sagas_status
    ON sagas(status);

-- Sagaの種類でのフィルタリングを高速化するインデックス。
CREATE INDEX IF NOT EXISTS idx_sagas_type
    ON sagas(saga_type);

-- Sagaステップの所属Saga検索を高速化するインデックス。
CREATE INDEX IF NOT EXISTS idx_saga_steps_saga_id
    ON saga_steps(saga_id);

-- Orchestratorのオフセット（最後にポーリングしたイベントのタイムスタンプ）を永続化するテーブル。
CREATE TABLE IF NOT EXISTS projector_offsets (
    id TEXT PRIMARY KEY DEFAULT 'default',
    last_timestamp DATETIME NOT NULL,
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);
