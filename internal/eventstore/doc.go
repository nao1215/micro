// Package eventstore はイベントストアサービスの内部実装を提供する。
//
// Event Sourcingの中核となるサービスで、すべてのサービスの状態変更を
// イベントとして永続化する。イベントは不変（immutable）であり、
// 追記のみ（append-only）で運用される。
//
// 主な機能:
//   - イベントの追記（Append）
//   - AggregateIDによるイベント取得（状態再構築用）
//   - イベントタイプによるイベント取得（Saga購読用）
//   - 日時指定によるイベント取得（Read Model増分更新用）
package eventstore
