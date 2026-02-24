-- Gateway（ユーザー管理）スキーマ
-- OAuth2認証で取得したユーザー情報を永続化する。

CREATE TABLE IF NOT EXISTS users (
    -- ユーザーの一意識別子（UUID）
    id TEXT PRIMARY KEY,
    -- OAuth2プロバイダー名（github, google）
    provider TEXT NOT NULL,
    -- プロバイダーが発行したユーザーID
    provider_user_id TEXT NOT NULL,
    -- メールアドレス
    email TEXT NOT NULL,
    -- 表示名
    display_name TEXT NOT NULL,
    -- アバター画像のURL
    avatar_url TEXT NOT NULL DEFAULT '',
    -- 初回ログイン日時
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    -- 最終ログイン日時
    last_login_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

-- プロバイダーとプロバイダーユーザーIDの組み合わせで一意制約を設ける。
-- 同じOAuthアカウントで二重登録されることを防ぐ。
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_provider
    ON users(provider, provider_user_id);

-- メールアドレスでの検索を高速化するインデックス。
CREATE INDEX IF NOT EXISTS idx_users_email
    ON users(email);
