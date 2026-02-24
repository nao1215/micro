-- name: CreateAlbum :exec
-- アルバムを新規作成する。
INSERT INTO albums (id, user_id, name, description, created_at, updated_at)
VALUES (?, ?, ?, ?, datetime('now'), datetime('now'));

-- name: GetAlbumByID :one
-- アルバムIDで1件取得する。
SELECT id, user_id, name, description, created_at, updated_at
FROM albums
WHERE id = ?;

-- name: ListAlbumsByUserID :many
-- ユーザーIDでアルバム一覧を取得する。
SELECT id, user_id, name, description, created_at, updated_at
FROM albums
WHERE user_id = ?
ORDER BY created_at DESC;

-- name: UpdateAlbum :exec
-- アルバムの名前と説明を更新する。
UPDATE albums
SET name = ?, description = ?, updated_at = datetime('now')
WHERE id = ?;

-- name: DeleteAlbum :exec
-- アルバムを削除する。album_mediaも CASCADE で削除される。
DELETE FROM albums
WHERE id = ?;

-- name: AddMediaToAlbum :exec
-- メディアをアルバムに追加する。
INSERT INTO album_media (album_id, media_id, added_at)
VALUES (?, ?, datetime('now'));

-- name: RemoveMediaFromAlbum :exec
-- メディアをアルバムから削除する。
DELETE FROM album_media
WHERE album_id = ? AND media_id = ?;

-- name: ListMediaInAlbum :many
-- アルバム内のメディアID一覧を取得する。
SELECT album_id, media_id, added_at
FROM album_media
WHERE album_id = ?
ORDER BY added_at DESC;

-- name: ListAlbumsByMediaID :many
-- メディアが属するアルバム一覧を取得する。
SELECT a.id, a.user_id, a.name, a.description, a.created_at, a.updated_at
FROM albums a
JOIN album_media am ON a.id = am.album_id
WHERE am.media_id = ?
ORDER BY a.created_at DESC;

-- name: GetDefaultAlbumByUserID :one
-- ユーザーのデフォルトアルバム（"All Media"）を取得する。
-- アップロードされたメディアは自動的にこのアルバムに追加される。
SELECT id, user_id, name, description, created_at, updated_at
FROM albums
WHERE user_id = ? AND name = 'All Media';
