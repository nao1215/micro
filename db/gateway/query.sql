-- name: CreateUser :exec
INSERT INTO users (id, provider, provider_user_id, email, display_name, avatar_url, created_at, last_login_at)
VALUES (?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'));

-- name: GetUserByID :one
SELECT id, provider, provider_user_id, email, display_name, avatar_url, created_at, last_login_at
FROM users
WHERE id = ?;

-- name: GetUserByProvider :one
SELECT id, provider, provider_user_id, email, display_name, avatar_url, created_at, last_login_at
FROM users
WHERE provider = ? AND provider_user_id = ?;

-- name: UpdateLastLogin :exec
UPDATE users
SET last_login_at = datetime('now')
WHERE id = ?;

-- name: UpdateUserProfile :exec
UPDATE users
SET display_name = ?, avatar_url = ?, last_login_at = datetime('now')
WHERE id = ?;
