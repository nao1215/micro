# micro

CQRS、Event Sourcing、Saga Pattern を学習するためのローカル専用マイクロサービスプロジェクト。
メディア（画像・動画）管理プラットフォーム「MediaHub」を題材に、分散システムの設計パターンを実践する。

## アーキテクチャ概要

### サービス一覧
| サービス | ポート | 責務 |
|---------|--------|------|
| gateway | 8080 | API Gateway、OAuth2認証、JWT発行、リクエストルーティング |
| media-command | 8081 | メディアのアップロード・更新・削除（Command側） |
| media-query | 8082 | メディアの一覧・詳細・検索（Query側、Read Model） |
| album | 8083 | アルバム管理、メディアとの関連付け |
| eventstore | 8084 | イベントの永続化と配信（Event Store） |
| saga | 8085 | Saga管理、分散トランザクションの調整 |
| notification | 8086 | イベント駆動の通知サービス |
| frontend | 3000 | 簡素なWeb UI（デバッグ用） |

### 設計パターン
- **CQRS**: media-command（書き込み）とmedia-query（読み取り）でCommand/Queryを分離
- **Event Sourcing**: すべての状態変更をイベントとしてEvent Storeに永続化。状態はイベント再生で復元
- **Saga Pattern**: メディアアップロードフロー等でOrchestration Sagaを実装。失敗時は補償アクションで復旧

### 技術スタック
- Go + Gin（各サービスのHTTP API）
- SQLite + sqlc（永続化）
- Docker Compose（サービス起動）
- OAuth2（GitHub/Google認証）
- JWT（サービス間認証）

## 開発コマンド

- `make build`: 全サービスをビルド
- `make test`: テスト実行とカバレッジ計測
- `make lint`: golangci-lintによるコード検査
- `make generate`: sqlcコード生成
- `make docker-up`: Docker Composeで全サービス起動
- `make docker-down`: Docker Composeで全サービス停止
- `make clean`: 生成ファイル削除
- `make tools`: 開発ツールのインストール（golangci-lint, sqlc, octocov）

## 開発ルール

- テスト駆動開発: t-wada（和田卓人）が推進するテスト駆動開発を採用する。常にテストコードを書き、テストピラミッドを意識する。
- 動作するコード: 作業完了後、`make test` と `make lint` が成功する状態を維持する。
- スポンサー獲得: 開発には金銭的コストがかかるため、`https://github.com/sponsors/nao1215` でスポンサーを募る。READMEやドキュメントにスポンサーリンクを含める。
- コントリビュータ獲得: 誰でも開発に参加できるよう、開発者向けドキュメントを作成してコントリビュータを募る。
- ドキュメンテーションコメントはすべて日本語で記述する。

## コーディングガイドライン

- グローバル変数禁止: グローバル変数を使用しない。状態は関数の引数と戻り値で管理する。
- Goのコーディングルール: [Effective Go](https://go.dev/doc/effective_go) を基本ルールとする。
- パッケージコメント必須: 各パッケージの `doc.go` にパッケージ概要を記述する。パッケージの目的と使い方を明確にする。
- 公開関数・変数・構造体フィールドのコメント必須: 公開要素には必ずgo docルールに従ったコメントを書く。
- 重複コード排除: 作業完了後、重複コードがないか確認し不要なコードを削除する。
- エラーハンドリング: エラーインターフェースの等価チェックには `errors.Is` と `errors.As` を使用する。エラーハンドリングを省略しない。
- ドキュメンテーションコメント: ユーザーがコードの使い方を理解できるようドキュメンテーションコメントを書く。コード内コメントは「なぜそうするか/しないか」を説明する。
- CHANGELOG.md管理: 更新時にはPR番号やコミットハッシュへのGitHubリンクを含める。

## テスティング

- [読みやすいテストコード](https://logmi.jp/main/technology/327449): 過度な最適化（DRY）を避け、テストの存在が理解しやすい状態を目指す。
- 明確な入出力: `t.Run()` でテストを作成し、テストケースの入出力を明確にする。
- テスト記述: `t.Run()` の第一引数で、入力と期待出力の関係を明確に記述する。
- テスト粒度: ユニットテストで80%以上のカバレッジを目指す。
- 並列テスト実行: 可能な限り `t.Parallel()` でテストを並列実行する。
- テストデータ: サンプルファイルは testdata ディレクトリに格納する。

## プロジェクト構成

```
micro/
├── cmd/                        # 各サービスのエントリポイント
│   ├── gateway/
│   ├── media-command/
│   ├── media-query/
│   ├── album/
│   ├── eventstore/
│   ├── saga/
│   └── notification/
├── internal/                   # 各サービスの内部実装
│   ├── gateway/
│   ├── media/
│   │   ├── command/            # CQRS Command側
│   │   └── query/              # CQRS Query側
│   ├── album/
│   ├── eventstore/
│   ├── saga/
│   └── notification/
├── pkg/                        # サービス間共有パッケージ
│   ├── event/                  # イベント型定義
│   ├── middleware/              # 共通ミドルウェア（JWT検証等）
│   └── httpclient/             # サービス間HTTP通信
├── db/                         # SQLスキーマとsqlc設定
│   ├── eventstore/
│   ├── media/
│   └── album/
├── web/                        # フロントエンド
│   └── frontend/
└── docker/                     # Dockerfile群
    ├── gateway/
    ├── media-command/
    └── ...
```

## Event設計

### イベント一覧
| イベント | 発行元 | 意味 |
|---------|--------|------|
| MediaUploaded | media-command | メディアファイルがアップロードされた |
| MediaProcessed | media-command | サムネイル生成等の処理が完了した |
| MediaProcessingFailed | media-command | メディア処理が失敗した |
| MediaDeleted | media-command | メディアが削除された |
| AlbumCreated | album | アルバムが作成された |
| AlbumDeleted | album | アルバムが削除された |
| MediaAddedToAlbum | album | メディアがアルバムに追加された |
| MediaRemovedFromAlbum | album | メディアがアルバムから削除された |
| NotificationSent | notification | 通知が送信された |

### イベント構造
```json
{
  "id": "uuid",
  "aggregate_id": "対象エンティティのID",
  "aggregate_type": "Media|Album|User",
  "event_type": "MediaUploaded",
  "data": { ... },
  "version": 1,
  "created_at": "2026-01-01T00:00:00Z"
}
```

## Saga設計

### メディアアップロードSaga（成功フロー）
1. ユーザーがメディアをアップロード → gateway → media-command
2. media-command: ファイル保存 → MediaUploadedイベント発行
3. saga: MediaUploadedを受信 → サムネイル生成を依頼
4. media-command: サムネイル生成完了 → MediaProcessedイベント発行
5. saga: MediaProcessedを受信 → デフォルトアルバムへの追加を依頼
6. album: メディア追加完了 → MediaAddedToAlbumイベント発行
7. saga: 完了通知を依頼 → NotificationSentイベント発行
8. saga: Saga完了

### メディアアップロードSaga（失敗フロー - 補償アクション）
- Step 6 失敗: album追加失敗 → media-commandにメディア無効化を依頼（補償）
- Step 4 失敗: サムネイル生成失敗 → media-commandにアップロード済みファイル削除を依頼（補償）
- 各補償アクションもイベントとして記録される

## セキュリティ上の注意点

- JWT署名検証: gatewayが発行したJWTを各サービスで検証する。署名鍵はHMAC-SHA256で共有秘密鍵を使用
- サービス間通信: Docker内部ネットワークで閉じる。外部からは gateway のみアクセス可能
- ユーザーID伝播: JWTのclaimsにuser_idを含め、サービス間リクエストのX-User-IDヘッダーで伝播
- OAuth2: state パラメータによるCSRF対策、redirect_uriの厳密な検証
- ファイルアップロード: Content-Type検証、ファイルサイズ制限、パストラバーサル防止
- CORS: gatewayでOriginを制限

## 作業メモ（compact後の再開用）

### 完了済み
- [x] プロジェクト初期化（CLAUDE.md、README.md、ディレクトリ構成、go.mod、docker-compose.yml）
- [x] 共有パッケージ（event型定義、middleware、httpclient）
- [x] DBスキーマとsqlc設定
- [x] 各サービスのエントリポイント骨格
- [x] sqlcによるコード生成（6サービス分）。sqlc v1.30.0はquery.sql内の日本語コメントを処理できないため、`-- name:`アノテーションのみ残した
- [x] Event Storeサービスの完全実装（イベント永続化・配信API、楽観的並行制御）
- [x] media-commandサービス実装（ファイルアップロード50MB制限、画像サムネイル200x200生成、補償エンドポイント）
- [x] media-queryサービス実装（Read Model、Projectorがeventstoreを2秒間隔でポーリング、フルリビルド対応）
- [x] albumサービス実装（CRUD、デフォルトアルバム自動作成、所有権チェック）
- [x] sagaサービス実装（Orchestration Saga、eventstoreを3秒間隔でポーリング、補償アクション）
- [x] notificationサービス実装（通知CRUD、既読管理、内部送信エンドポイント）
- [x] gatewayサービス実装（OAuth2スタブ、dev-tokenエンドポイント、リバースプロキシ、JWTミドルウェア）
- [x] フロントエンドUI（デバッグ用HTML、メディア/アルバム/Saga/イベント/通知タブ）
- [x] ビルド検証通過（`go build ./...` および `go vet ./...`）

### 未着手
- [ ] Docker環境の動作確認（`docker compose up --build`）
- [ ] テストコード（各サービスのユニットテスト）
- [ ] Lint通過確認（`make lint`）
- [ ] フロントエンドUIとバックエンドの統合テスト

### 実装上の注意点
- sqlcのquery.sqlに日本語コメントを入れるとパースエラーになる。`-- name:`アノテーションのみ使用すること
- 各サービスのスキーマ初期化は`internal/*/schema.go`の`initSchema()`で行う（sqlcのschema.sqlとは別に管理）
- サービス間通信はHTTPベース。メッセージブローカーなし。イベントサブスクリプションはポーリング方式
- Event StoreのURLはeventstoreサービスのhandleAppendEventで受け付ける（POST /api/v1/events）
- Sagaオーケストレーターはバックグラウンドゴルーチンでポーリングする（server.go内でgo s.orchestrator.Start()）
