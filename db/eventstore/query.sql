-- name: AppendEvent :exec
-- イベントを追記する。Event Sourcingの核となる操作。
-- イベントは不変であり、一度書き込まれたら変更・削除しない。
INSERT INTO events (id, aggregate_id, aggregate_type, event_type, data, version, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: GetEventsByAggregateID :many
-- 指定されたAggregateIDのイベントをバージョン順に取得する。
-- Aggregateの現在の状態を再構築するために使用する。
SELECT id, aggregate_id, aggregate_type, event_type, data, version, created_at
FROM events
WHERE aggregate_id = ?
ORDER BY version ASC;

-- name: GetEventsByType :many
-- 指定されたイベントタイプのイベントを作成日時順に取得する。
-- Sagaオーケストレータが特定のイベントを購読する際に使用する。
SELECT id, aggregate_id, aggregate_type, event_type, data, version, created_at
FROM events
WHERE event_type = ?
ORDER BY created_at ASC;

-- name: GetEventsSince :many
-- 指定された日時以降のイベントを作成日時順に取得する。
-- Read Modelの増分更新やSagaのポーリングに使用する。
SELECT id, aggregate_id, aggregate_type, event_type, data, version, created_at
FROM events
WHERE created_at > ?
ORDER BY created_at ASC;

-- name: GetEventsByAggregateType :many
-- 指定されたAggregateTypeのイベントを作成日時順に取得する。
SELECT id, aggregate_id, aggregate_type, event_type, data, version, created_at
FROM events
WHERE aggregate_type = ?
ORDER BY created_at ASC;

-- name: GetLatestVersion :one
-- 指定されたAggregateIDの最新バージョン番号を取得する。
-- 楽観的排他制御のために使用する。
SELECT COALESCE(MAX(version), 0) AS latest_version
FROM events
WHERE aggregate_id = ?;

-- name: GetAllEvents :many
-- すべてのイベントを作成日時順に取得する。
-- Read Modelの完全再構築に使用する。注意: 大量データの場合は GetEventsSince を使用すること。
SELECT id, aggregate_id, aggregate_type, event_type, data, version, created_at
FROM events
ORDER BY created_at ASC;
