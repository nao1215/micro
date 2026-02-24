-- name: CreateNotification :exec
-- 通知を新規作成する。
INSERT INTO notifications (id, user_id, title, message, created_at)
VALUES (?, ?, ?, ?, datetime('now'));

-- name: GetNotificationByID :one
-- 通知IDで1件取得する。
SELECT id, user_id, title, message, is_read, created_at
FROM notifications
WHERE id = ?;

-- name: ListNotificationsByUserID :many
-- ユーザーIDで通知一覧を取得する（新しい順）。
SELECT id, user_id, title, message, is_read, created_at
FROM notifications
WHERE user_id = ?
ORDER BY created_at DESC;

-- name: ListUnreadNotifications :many
-- ユーザーの未読通知一覧を取得する。
SELECT id, user_id, title, message, is_read, created_at
FROM notifications
WHERE user_id = ? AND is_read = 0
ORDER BY created_at DESC;

-- name: MarkAsRead :exec
-- 通知を既読にする。
UPDATE notifications
SET is_read = 1
WHERE id = ?;

-- name: MarkAllAsRead :exec
-- ユーザーの全通知を既読にする。
UPDATE notifications
SET is_read = 1
WHERE user_id = ? AND is_read = 0;
