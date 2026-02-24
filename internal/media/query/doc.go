// Package query はCQRSのQuery側であるメディアクエリサービスの内部実装を提供する。
//
// Event Storeのイベントを購読してRead Model（SQLite）を構築・更新する。
// メディアの一覧・詳細・検索の読み取りクエリを処理する。
// Read Modelは非正規化データで構成され、検索性能に最適化されている。
// Read Modelはいつでも破棄してEvent Storeから再構築できる。
package query
