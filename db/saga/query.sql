-- name: CreateSaga :exec
-- 新しいSagaインスタンスを作成する。
INSERT INTO sagas (id, saga_type, current_step, status, payload, started_at, updated_at)
VALUES (?, ?, ?, 'started', ?, datetime('now'), datetime('now'));

-- name: GetSagaByID :one
-- SagaインスタンスをIDで取得する。
SELECT id, saga_type, current_step, status, payload, started_at, updated_at, completed_at
FROM sagas
WHERE id = ?;

-- name: UpdateSagaStep :exec
-- Sagaの現在のステップとステータスを更新する。
UPDATE sagas
SET current_step = ?, status = ?, payload = ?, updated_at = datetime('now')
WHERE id = ?;

-- name: CompleteSaga :exec
-- Sagaを完了状態に更新する。
UPDATE sagas
SET status = 'completed', updated_at = datetime('now'), completed_at = datetime('now')
WHERE id = ?;

-- name: FailSaga :exec
-- Sagaを失敗状態に更新する。
UPDATE sagas
SET status = 'failed', updated_at = datetime('now'), completed_at = datetime('now')
WHERE id = ?;

-- name: ListActiveSagas :many
-- アクティブな（未完了の）Sagaインスタンス一覧を取得する。
-- サービス再起動時に中断されたSagaを再開するために使用する。
SELECT id, saga_type, current_step, status, payload, started_at, updated_at, completed_at
FROM sagas
WHERE status IN ('started', 'in_progress', 'compensating')
ORDER BY started_at ASC;

-- name: CreateSagaStep :exec
-- Sagaのステップ実行履歴を記録する。
INSERT INTO saga_steps (id, saga_id, step_name, status, result, started_at)
VALUES (?, ?, ?, ?, '{}', datetime('now'));

-- name: UpdateSagaStepStatus :exec
-- Sagaステップのステータスと結果を更新する。
UPDATE saga_steps
SET status = ?, result = ?, completed_at = datetime('now')
WHERE id = ?;

-- name: ListSagaSteps :many
-- Sagaのステップ実行履歴を取得する。
SELECT id, saga_id, step_name, status, result, started_at, completed_at
FROM saga_steps
WHERE saga_id = ?
ORDER BY started_at ASC;
