-- name: CreateAlbum :exec
INSERT INTO albums (id, user_id, name, description, created_at, updated_at)
VALUES (?, ?, ?, ?, datetime('now'), datetime('now'));

-- name: GetAlbumByID :one
SELECT id, user_id, name, description, created_at, updated_at
FROM albums
WHERE id = ?;

-- name: ListAlbumsByUserID :many
SELECT id, user_id, name, description, created_at, updated_at
FROM albums
WHERE user_id = ?
ORDER BY created_at DESC;

-- name: UpdateAlbum :exec
UPDATE albums
SET name = ?, description = ?, updated_at = datetime('now')
WHERE id = ?;

-- name: DeleteAlbum :exec
DELETE FROM albums
WHERE id = ?;

-- name: AddMediaToAlbum :exec
INSERT INTO album_media (album_id, media_id, added_at)
VALUES (?, ?, datetime('now'));

-- name: RemoveMediaFromAlbum :exec
DELETE FROM album_media
WHERE album_id = ? AND media_id = ?;

-- name: ListMediaInAlbum :many
SELECT album_id, media_id, added_at
FROM album_media
WHERE album_id = ?
ORDER BY added_at DESC;

-- name: ListAlbumsByMediaID :many
SELECT a.id, a.user_id, a.name, a.description, a.created_at, a.updated_at
FROM albums a
JOIN album_media am ON a.id = am.album_id
WHERE am.media_id = ?
ORDER BY a.created_at DESC;

-- name: GetDefaultAlbumByUserID :one
SELECT id, user_id, name, description, created_at, updated_at
FROM albums
WHERE user_id = ? AND name = 'All Media';
