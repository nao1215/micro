# micro - ローカル専用マイクロサービス学習プラットフォーム

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

CQRS、Event Sourcing、Saga Pattern を学習するためのローカル専用マイクロサービスプロジェクトです。
メディア（画像・動画）管理プラットフォーム「**MediaHub**」を題材に、分散システムの設計パターンを実践します。

> **注意**: このプロジェクトはクラウドサービスを一切使用せず、Docker Compose でローカル環境のみで動作します。

## スポンサー

開発の継続には皆さまのご支援が必要です。
[![GitHub Sponsors](https://img.shields.io/badge/Sponsor-GitHub-ea4aaa)](https://github.com/sponsors/nao1215)

## 目次

- [アーキテクチャ概要](#アーキテクチャ概要)
- [サービス一覧](#サービス一覧)
- [CQRS - Command/Query の分離](#cqrs---commandquery-の分離)
- [Event Sourcing - イベントストアとRead Modelの違い](#event-sourcing---イベントストアとread-modelの違い)
- [Saga Pattern - 分散トランザクション](#saga-pattern---分散トランザクション)
- [Event 設計](#event-設計)
- [セキュリティ設計](#セキュリティ設計)
- [ディレクトリ構成](#ディレクトリ構成)
- [技術スタック](#技術スタック)
- [起動方法](#起動方法)
- [開発方法](#開発方法)

## アーキテクチャ概要

```
┌─────────┐     ┌──────────────┐     ┌───────────────┐
│ Frontend │────▶│   Gateway    │────▶│ media-command  │──┐
│ (3000)   │     │  (8080)      │     │ (8081)         │  │
└─────────┘     │ OAuth2 + JWT │     │ ファイル保存    │  │  Event
                └──────┬───────┘     │ サムネイル生成  │  │
                       │             └───────────────┘  │
                       │                                 ▼
                       │  ┌───────────────┐     ┌──────────────┐
                       ├─▶│ media-query   │◀────│  Event Store │
                       │  │ (8082)        │     │  (8084)      │
                       │  │ Read Model    │     │ イベント永続化 │
                       │  └───────────────┘     └──────┬───────┘
                       │                               │
                       │  ┌───────────────┐     ┌──────┴───────┐
                       ├─▶│ album         │◀────│    Saga      │
                       │  │ (8083)        │     │  (8085)      │
                       │  │ アルバム管理   │     │ 分散TX調整    │
                       │  └───────────────┘     └──────┬───────┘
                       │                               │
                       │                        ┌──────┴───────┐
                       └───────────────────────▶│ notification │
                                                │ (8086)       │
                                                │ 通知配信      │
                                                └──────────────┘
```

## サービス一覧

| サービス | ポート | 責務 | DB |
|---------|--------|------|-----|
| **gateway** | 8080 | API Gateway、OAuth2認証（GitHub/Google）、JWT発行、リクエストルーティング | ユーザー情報 (SQLite) |
| **media-command** | 8081 | メディアのアップロード・更新・削除。Command側。ファイル保存とサムネイル生成を担当 | なし（Event Store経由） |
| **media-query** | 8082 | メディアの一覧・詳細・検索。Query側。Event Storeのイベントからビューを構築 | Read Model (SQLite) |
| **album** | 8083 | アルバムのCRUD、メディアとの関連付け | アルバム情報 (SQLite) |
| **eventstore** | 8084 | イベントの永続化と配信。全サービスの状態変更を記録する中央ストア | Event Store (SQLite) |
| **saga** | 8085 | Orchestration Saga。分散トランザクションの調整と失敗時の補償アクション管理 | Saga状態 (SQLite) |
| **notification** | 8086 | イベント駆動の通知サービス。メディア処理完了等の通知を配信 | 通知履歴 (SQLite) |
| **frontend** | 3000 | 簡素なWeb UI。デバッグ・動作確認用 | なし |

## CQRS - Command/Query の分離

CQRS（Command Query Responsibility Segregation）は、データの書き込みと読み取りを別のモデルで処理するパターンです。

### なぜ分離するのか？

従来のCRUDでは、同じデータモデルで書き込みと読み取りを行います。しかし以下の問題があります:

1. **書き込みと読み取りで最適なデータ構造が異なる** - 書き込み時は整合性重視、読み取り時は検索性能重視
2. **スケーリングの粒度が異なる** - 読み取りは書き込みより圧倒的に多い場合がある
3. **ドメインロジックの複雑化** - 読み取り用のクエリが書き込みロジックを汚染する

### 本プロジェクトでの実装

```
[Command側: media-command]
 ユーザーリクエスト → バリデーション → ビジネスロジック → イベント生成 → Event Storeに保存

[Query側: media-query]
 Event Storeからイベント購読 → Read Model（SQLite）を更新 → クエリに対して非正規化ビューを返却
```

- **Command側 (media-command)**: 書き込み専用。バリデーション・ビジネスルール適用後、イベントを生成してEvent Storeに送る。自身ではデータを永続化しない
- **Query側 (media-query)**: 読み取り専用。Event Storeのイベントを購読し、検索に最適化されたRead Modelを構築する

## Event Sourcing - イベントストアとRead Modelの違い

### Event Store（イベントストア）とは？

Event Storeは **「何が起きたか」の完全な履歴** を保持するデータベースです。

```
┌─────────────────────────────────────────────────────────────┐
│ Event Store                                                  │
├──────┬──────────────┬─────────────────┬──────┬──────────────┤
│ ID   │ aggregate_id │ event_type      │ ver  │ data (JSON)  │
├──────┼──────────────┼─────────────────┼──────┼──────────────┤
│ ev-1 │ media-001    │ MediaUploaded   │  1   │ {filename..} │
│ ev-2 │ media-001    │ MediaProcessed  │  2   │ {thumb..}    │
│ ev-3 │ media-002    │ MediaUploaded   │  1   │ {filename..} │
│ ev-4 │ media-001    │ MediaDeleted    │  3   │ {}           │
└──────┴──────────────┴─────────────────┴──────┴──────────────┘
```

**特徴:**
- イベントは **不変（immutable）** - 一度書き込んだら変更・削除しない
- すべての状態変更がイベントとして記録される - **完全な監査ログ** になる
- 現在の状態は **イベントを最初から再生** することで復元できる
- **時間旅行** が可能 - 任意の時点の状態を再現できる

### Read Model（読み取りモデル）とは？

Read Modelは **「現在の状態」を問い合わせに最適化した形で保持** するデータベースです。

```
┌─────────────────────────────────────────────────────────────┐
│ Read Model (media-query の SQLite)                           │
├──────────────┬──────────┬────────┬───────────┬──────────────┤
│ media_id     │ filename │ status │ thumb_url │ uploaded_at  │
├──────────────┼──────────┼────────┼───────────┼──────────────┤
│ media-002    │ cat.jpg  │ active │ /thumb/.. │ 2026-01-02   │
└──────────────┴──────────┴────────┴───────────┴──────────────┘
```

**特徴:**
- Event Storeのイベントから **投影（Projection）** して構築される
- 検索・表示に最適化された **非正規化データ** を持つ
- **いつでも再構築可能** - Event Storeからイベントを再生すれば元に戻せる
- Read Modelは **使い捨て** - スキーマ変更時は破棄して再構築するだけ

### 両者の違い

| 観点 | Event Store | Read Model |
|------|-------------|------------|
| 保持するもの | 過去のすべての出来事（イベント） | 現在の状態のスナップショット |
| データの変更 | 追記のみ（append-only） | 更新・削除あり |
| 真実の源泉 | **はい** - これが唯一の正解 | いいえ - Event Storeから導出される |
| 最適化対象 | 書き込み性能 | 読み取り性能 |
| 再構築 | 不可能（これが原本） | いつでも再構築可能 |

## Saga Pattern - 分散トランザクション

### なぜSagaが必要か？

マイクロサービスでは、複数サービスにまたがるトランザクションを単一のDBトランザクションで実現できません。Sagaパターンは、各サービスのローカルトランザクションを連鎖させ、失敗時には **補償アクション（Compensation）** で整合性を保ちます。

### メディアアップロードSaga

#### 成功フロー
```
[1] ユーザー → gateway → media-command: ファイルアップロード
         ↓
[2] media-command → eventstore: MediaUploadedイベント発行
         ↓
[3] saga: MediaUploaded受信 → media-command: サムネイル生成依頼
         ↓
[4] media-command → eventstore: MediaProcessedイベント発行
         ↓
[5] saga: MediaProcessed受信 → album: デフォルトアルバムにメディア追加依頼
         ↓
[6] album → eventstore: MediaAddedToAlbumイベント発行
         ↓
[7] saga: 完了 → notification: アップロード完了通知依頼
         ↓
[8] notification → eventstore: NotificationSentイベント発行 → Saga完了
```

#### 失敗フロー（補償アクション）
```
[Step 5で失敗: アルバム追加に失敗した場合]
saga → media-command: メディアを無効化（補償アクション）
saga → eventstore: MediaUploadCompensatedイベント発行
saga: Saga失敗として記録

[Step 3で失敗: サムネイル生成に失敗した場合]
saga → media-command: アップロード済みファイルを削除（補償アクション）
saga → eventstore: MediaUploadCompensatedイベント発行
saga: Saga失敗として記録
```

**ポイント**: 補償アクションは「元に戻す」のではなく「打ち消す新しいアクション」を実行します。Event Sourcingではイベントは不変なので、過去のイベントを削除するのではなく、新しい補償イベントを追加します。

## Event 設計

### イベント一覧

| イベント | 発行元 | 意味 |
|---------|--------|------|
| `MediaUploaded` | media-command | メディアファイルがアップロードされた |
| `MediaProcessed` | media-command | サムネイル生成等の処理が完了した |
| `MediaProcessingFailed` | media-command | メディア処理が失敗した |
| `MediaDeleted` | media-command | メディアが削除された |
| `MediaUploadCompensated` | media-command | アップロードの補償アクションが実行された |
| `AlbumCreated` | album | アルバムが作成された |
| `AlbumDeleted` | album | アルバムが削除された |
| `MediaAddedToAlbum` | album | メディアがアルバムに追加された |
| `MediaRemovedFromAlbum` | album | メディアがアルバムから削除された |
| `NotificationSent` | notification | 通知が送信された |

### イベント構造

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "aggregate_id": "media-001",
  "aggregate_type": "Media",
  "event_type": "MediaUploaded",
  "data": {
    "user_id": "user-001",
    "filename": "photo.jpg",
    "content_type": "image/jpeg",
    "size": 1048576
  },
  "version": 1,
  "created_at": "2026-01-01T00:00:00Z"
}
```

## セキュリティ設計

### 認証フロー（OAuth2 + JWT）

1. ユーザーが Gateway にアクセス
2. Gateway が GitHub/Google の OAuth2 認可エンドポイントにリダイレクト
3. 認可コードを受け取り、アクセストークンと交換
4. ユーザー情報を取得し、JWT を発行
5. 以降のリクエストは JWT を Bearer トークンとして送信

### セキュリティ上の注意点

- **JWT署名検証**: Gateway が発行した JWT を各サービスで検証。HMAC-SHA256 で署名
- **サービス間通信**: Docker 内部ネットワークで閉じる。外部からは Gateway のみアクセス可能
- **ユーザーID伝播**: JWT の claims に `user_id` を含め、サービス間は `X-User-ID` ヘッダーで伝播
- **OAuth2 state パラメータ**: CSRF 対策として必須
- **redirect_uri 検証**: 登録済み URI のみ許可
- **ファイルアップロード**: Content-Type 検証、サイズ制限（50MB）、パストラバーサル防止
- **CORS**: Gateway で Origin を制限

## ディレクトリ構成

```
micro/
├── CLAUDE.md                   # AI向け開発ガイド（コンテキスト保持用）
├── README.md                   # 本ドキュメント
├── go.mod                      # Go module定義
├── go.sum
├── Makefile                    # 開発コマンド
├── docker-compose.yml          # 全サービスの起動定義
├── .env.example                # 環境変数テンプレート
│
├── cmd/                        # 各サービスのエントリポイント
│   ├── gateway/
│   │   └── main.go
│   ├── media-command/
│   │   └── main.go
│   ├── media-query/
│   │   └── main.go
│   ├── album/
│   │   └── main.go
│   ├── eventstore/
│   │   └── main.go
│   ├── saga/
│   │   └── main.go
│   └── notification/
│       └── main.go
│
├── internal/                   # 各サービスの内部実装（非公開）
│   ├── gateway/                # Gateway固有のハンドラ・ルーティング
│   ├── media/
│   │   ├── command/            # CQRS Command側のハンドラ・ドメインロジック
│   │   └── query/              # CQRS Query側のハンドラ・Read Model
│   ├── album/                  # アルバムサービスのハンドラ・ドメインロジック
│   ├── eventstore/             # Event Storeのハンドラ・永続化ロジック
│   ├── saga/                   # Sagaオーケストレータのステートマシン
│   └── notification/           # 通知サービスのハンドラ
│
├── pkg/                        # サービス間で共有するパッケージ（公開）
│   ├── event/                  # イベント型定義・シリアライズ
│   ├── middleware/              # JWT検証・ログ・リカバリ等の共通ミドルウェア
│   └── httpclient/             # サービス間HTTP通信のクライアント
│
├── db/                         # SQLスキーマとsqlc設定
│   ├── eventstore/             # Event Store用スキーマ
│   │   ├── sqlc.yaml
│   │   ├── schema.sql
│   │   └── query.sql
│   ├── media/                  # Media Read Model用スキーマ
│   │   ├── sqlc.yaml
│   │   ├── schema.sql
│   │   └── query.sql
│   ├── album/                  # Album用スキーマ
│   │   ├── sqlc.yaml
│   │   ├── schema.sql
│   │   └── query.sql
│   ├── gateway/                # Gateway (ユーザー情報) 用スキーマ
│   │   ├── sqlc.yaml
│   │   ├── schema.sql
│   │   └── query.sql
│   ├── saga/                   # Saga状態管理用スキーマ
│   │   ├── sqlc.yaml
│   │   ├── schema.sql
│   │   └── query.sql
│   └── notification/           # 通知履歴用スキーマ
│       ├── sqlc.yaml
│       ├── schema.sql
│       └── query.sql
│
├── web/                        # フロントエンド
│   └── frontend/
│       ├── index.html
│       └── static/
│
└── docker/                     # Dockerfile群
    ├── gateway.Dockerfile
    ├── media-command.Dockerfile
    ├── media-query.Dockerfile
    ├── album.Dockerfile
    ├── eventstore.Dockerfile
    ├── saga.Dockerfile
    ├── notification.Dockerfile
    └── frontend.Dockerfile
```

## 技術スタック

| カテゴリ | 技術 | 用途 |
|---------|------|------|
| 言語 | Go | 全バックエンドサービス |
| Web Framework | Gin | HTTP API |
| DB | SQLite | 永続化（各サービスごとに独立） |
| DB Access | sqlc | SQLからGoコード生成 |
| 認証 | OAuth2 (GitHub/Google) | ユーザー認証 |
| トークン | JWT (HMAC-SHA256) | サービス間認証 |
| コンテナ | Docker Compose | ローカル環境での全サービス起動 |
| Lint | golangci-lint | コード品質検査 |
| テスト | Go標準 + octocov | テスト・カバレッジ |
| フロントエンド | HTML/CSS/JavaScript | 簡素なデバッグUI |

## 起動方法

### 前提条件
- Docker / Docker Compose
- Go 1.24+
- make

### 環境変数の設定

```bash
cp .env.example .env
# .env を編集して GitHub/Google の OAuth2 クライアントIDとシークレットを設定
```

### 起動

```bash
# 全サービスをビルドして起動
make docker-up

# フロントエンドにアクセス
open http://localhost:3000

# 停止
make docker-down
```

### ローカル開発（Docker不使用）

```bash
# 依存ツールのインストール
make tools

# sqlcコード生成
make generate

# テスト
make test

# Lint
make lint

# 個別サービスの起動（例: event-store）
go run ./cmd/eventstore/
```

## 開発方法

### 新しいイベントを追加する

1. `pkg/event/types.go` にイベント型を定義
2. `db/eventstore/query.sql` にクエリを追加（必要に応じて）
3. `make generate` で sqlc コード再生成
4. 関連サービスにハンドラを実装

### 新しいSagaを追加する

1. Sagaのステップと補償アクションを設計
2. `internal/saga/` にSaga定義を追加
3. `db/saga/` にSaga状態管理用のクエリを追加
4. 関連するイベントハンドラを各サービスに実装

## ライセンス

MIT License - 詳細は [LICENSE](LICENSE) をご覧ください。
