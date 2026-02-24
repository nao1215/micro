-- name: CreateUser :exec
-- ユーザーを新規作成する。OAuth2認証の初回ログイン時に使用する。
INSERT INTO users (id, provider, provider_user_id, email, display_name, avatar_url, created_at, last_login_at)
VALUES (?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'));

-- name: GetUserByID :one
-- ユーザーIDで1件取得する。
SELECT id, provider, provider_user_id, email, display_name, avatar_url, created_at, last_login_at
FROM users
WHERE id = ?;

-- name: GetUserByProvider :one
-- プロバイダーとプロバイダーユーザーIDでユーザーを取得する。
-- OAuth2ログイン時に既存ユーザーを検索するために使用する。
SELECT id, provider, provider_user_id, email, display_name, avatar_url, created_at, last_login_at
FROM users
WHERE provider = ? AND provider_user_id = ?;

-- name: UpdateLastLogin :exec
-- 最終ログイン日時を更新する。
UPDATE users
SET last_login_at = datetime('now')
WHERE id = ?;

-- name: UpdateUserProfile :exec
-- ユーザーの表示名とアバターURLを更新する。
UPDATE users
SET display_name = ?, avatar_url = ?, last_login_at = datetime('now')
WHERE id = ?;
