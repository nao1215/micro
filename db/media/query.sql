-- name: UpsertMediaReadModel :exec
-- メディアRead Modelを挿入または更新する。
-- Event Storeのイベントから投影する際に使用する。
INSERT INTO media_read_models (id, user_id, filename, content_type, size, storage_path, status, last_event_version, uploaded_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'))
ON CONFLICT(id) DO UPDATE SET
    status = excluded.status,
    last_event_version = excluded.last_event_version,
    updated_at = datetime('now');

-- name: UpdateMediaProcessed :exec
-- メディア処理完了時にRead Modelを更新する。
UPDATE media_read_models
SET thumbnail_path = ?,
    width = ?,
    height = ?,
    duration_seconds = ?,
    status = 'processed',
    last_event_version = ?,
    updated_at = datetime('now')
WHERE id = ?;

-- name: UpdateMediaStatus :exec
-- メディアのステータスを更新する。
UPDATE media_read_models
SET status = ?,
    last_event_version = ?,
    updated_at = datetime('now')
WHERE id = ?;

-- name: GetMediaByID :one
-- メディアIDで1件取得する。
SELECT id, user_id, filename, content_type, size, storage_path,
       thumbnail_path, width, height, duration_seconds,
       status, last_event_version, uploaded_at, updated_at
FROM media_read_models
WHERE id = ?;

-- name: ListMediaByUserID :many
-- ユーザーIDでメディア一覧を取得する（削除済みを除く）。
SELECT id, user_id, filename, content_type, size, storage_path,
       thumbnail_path, width, height, duration_seconds,
       status, last_event_version, uploaded_at, updated_at
FROM media_read_models
WHERE user_id = ? AND status != 'deleted'
ORDER BY uploaded_at DESC;

-- name: ListAllMedia :many
-- 全メディア一覧を取得する（削除済みを除く）。
SELECT id, user_id, filename, content_type, size, storage_path,
       thumbnail_path, width, height, duration_seconds,
       status, last_event_version, uploaded_at, updated_at
FROM media_read_models
WHERE status != 'deleted'
ORDER BY uploaded_at DESC;

-- name: SearchMedia :many
-- ファイル名で部分一致検索する。
SELECT id, user_id, filename, content_type, size, storage_path,
       thumbnail_path, width, height, duration_seconds,
       status, last_event_version, uploaded_at, updated_at
FROM media_read_models
WHERE filename LIKE ? AND status != 'deleted'
ORDER BY uploaded_at DESC;

-- name: DeleteAllMediaReadModels :exec
-- すべてのRead Modelを削除する。Read Modelの完全再構築時に使用する。
DELETE FROM media_read_models;
