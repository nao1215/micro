-- name: CreateNotification :exec
INSERT INTO notifications (id, user_id, title, message, created_at)
VALUES (?, ?, ?, ?, datetime('now'));

-- name: GetNotificationByID :one
SELECT id, user_id, title, message, is_read, created_at
FROM notifications
WHERE id = ?;

-- name: ListNotificationsByUserID :many
SELECT id, user_id, title, message, is_read, created_at
FROM notifications
WHERE user_id = ?
ORDER BY created_at DESC;

-- name: ListUnreadNotifications :many
SELECT id, user_id, title, message, is_read, created_at
FROM notifications
WHERE user_id = ? AND is_read = 0
ORDER BY created_at DESC;

-- name: MarkAsRead :exec
UPDATE notifications
SET is_read = 1
WHERE id = ?;

-- name: MarkAllAsRead :exec
UPDATE notifications
SET is_read = 1
WHERE user_id = ? AND is_read = 0;
