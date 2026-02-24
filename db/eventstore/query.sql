-- name: AppendEvent :exec
INSERT INTO events (id, aggregate_id, aggregate_type, event_type, data, version, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: GetEventsByAggregateID :many
SELECT id, aggregate_id, aggregate_type, event_type, data, version, created_at
FROM events
WHERE aggregate_id = ?
ORDER BY version ASC;

-- name: GetEventsByType :many
SELECT id, aggregate_id, aggregate_type, event_type, data, version, created_at
FROM events
WHERE event_type = ?
ORDER BY created_at ASC;

-- name: GetEventsSince :many
SELECT id, aggregate_id, aggregate_type, event_type, data, version, created_at
FROM events
WHERE created_at > ?
ORDER BY created_at ASC;

-- name: GetEventsByAggregateType :many
SELECT id, aggregate_id, aggregate_type, event_type, data, version, created_at
FROM events
WHERE aggregate_type = ?
ORDER BY created_at ASC;

-- name: GetLatestVersion :one
SELECT COALESCE(MAX(version), 0) AS latest_version
FROM events
WHERE aggregate_id = ?;

-- name: GetAllEvents :many
SELECT id, aggregate_id, aggregate_type, event_type, data, version, created_at
FROM events
ORDER BY created_at ASC;
