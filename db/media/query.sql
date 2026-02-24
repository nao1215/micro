-- name: UpsertMediaReadModel :exec
INSERT INTO media_read_models (id, user_id, filename, content_type, size, storage_path, status, last_event_version, uploaded_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'))
ON CONFLICT(id) DO UPDATE SET
    status = excluded.status,
    last_event_version = excluded.last_event_version,
    updated_at = datetime('now');

-- name: UpdateMediaProcessed :exec
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
UPDATE media_read_models
SET status = ?,
    last_event_version = ?,
    updated_at = datetime('now')
WHERE id = ?;

-- name: GetMediaByID :one
SELECT id, user_id, filename, content_type, size, storage_path,
       thumbnail_path, width, height, duration_seconds,
       status, last_event_version, uploaded_at, updated_at
FROM media_read_models
WHERE id = ?;

-- name: ListMediaByUserID :many
SELECT id, user_id, filename, content_type, size, storage_path,
       thumbnail_path, width, height, duration_seconds,
       status, last_event_version, uploaded_at, updated_at
FROM media_read_models
WHERE user_id = ? AND status != 'deleted'
ORDER BY uploaded_at DESC;

-- name: ListAllMedia :many
SELECT id, user_id, filename, content_type, size, storage_path,
       thumbnail_path, width, height, duration_seconds,
       status, last_event_version, uploaded_at, updated_at
FROM media_read_models
WHERE status != 'deleted'
ORDER BY uploaded_at DESC;

-- name: SearchMedia :many
SELECT id, user_id, filename, content_type, size, storage_path,
       thumbnail_path, width, height, duration_seconds,
       status, last_event_version, uploaded_at, updated_at
FROM media_read_models
WHERE filename LIKE ? AND status != 'deleted'
ORDER BY uploaded_at DESC;

-- name: DeleteAllMediaReadModels :exec
DELETE FROM media_read_models;

-- name: GetProjectorOffset :one
SELECT last_timestamp FROM projector_offsets WHERE id = 'default';

-- name: UpsertProjectorOffset :exec
INSERT INTO projector_offsets (id, last_timestamp, updated_at)
VALUES ('default', ?, datetime('now'))
ON CONFLICT(id) DO UPDATE SET last_timestamp = excluded.last_timestamp, updated_at = datetime('now');
