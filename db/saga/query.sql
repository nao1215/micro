-- name: CreateSaga :exec
INSERT INTO sagas (id, saga_type, current_step, status, payload, started_at, updated_at)
VALUES (?, ?, ?, 'started', ?, datetime('now'), datetime('now'));

-- name: GetSagaByID :one
SELECT id, saga_type, current_step, status, payload, started_at, updated_at, completed_at
FROM sagas
WHERE id = ?;

-- name: UpdateSagaStep :exec
UPDATE sagas
SET current_step = ?, status = ?, payload = ?, updated_at = datetime('now')
WHERE id = ?;

-- name: CompleteSaga :exec
UPDATE sagas
SET status = 'completed', updated_at = datetime('now'), completed_at = datetime('now')
WHERE id = ?;

-- name: FailSaga :exec
UPDATE sagas
SET status = 'failed', updated_at = datetime('now'), completed_at = datetime('now')
WHERE id = ?;

-- name: ListActiveSagas :many
SELECT id, saga_type, current_step, status, payload, started_at, updated_at, completed_at
FROM sagas
WHERE status IN ('started', 'in_progress', 'compensating')
ORDER BY started_at ASC;

-- name: CreateSagaStep :exec
INSERT INTO saga_steps (id, saga_id, step_name, status, result, started_at)
VALUES (?, ?, ?, ?, '{}', datetime('now'));

-- name: UpdateSagaStepStatus :exec
UPDATE saga_steps
SET status = ?, result = ?, completed_at = datetime('now')
WHERE id = ?;

-- name: ListSagaSteps :many
SELECT id, saga_id, step_name, status, result, started_at, completed_at, retry_count, last_error
FROM saga_steps
WHERE saga_id = ?
ORDER BY started_at ASC;

-- name: UpdateSagaStepRetry :exec
UPDATE saga_steps
SET retry_count = ?, last_error = ?, status = ?
WHERE id = ?;

-- name: ListStuckSagas :many
SELECT id, saga_type, current_step, status, payload, started_at, updated_at, completed_at
FROM sagas
WHERE status IN ('in_progress', 'compensating')
  AND updated_at < ?
ORDER BY updated_at ASC;

-- name: GetProjectorOffset :one
SELECT last_timestamp FROM projector_offsets WHERE id = 'default';

-- name: UpsertProjectorOffset :exec
INSERT INTO projector_offsets (id, last_timestamp, updated_at)
VALUES ('default', ?, datetime('now'))
ON CONFLICT(id) DO UPDATE SET last_timestamp = excluded.last_timestamp, updated_at = datetime('now');
