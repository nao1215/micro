CREATE TABLE IF NOT EXISTS sagas (
    id TEXT PRIMARY KEY,
    saga_type TEXT NOT NULL,
    current_step TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'started',
    payload TEXT NOT NULL DEFAULT '{}',
    started_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now')),
    completed_at DATETIME
);

CREATE TABLE IF NOT EXISTS saga_steps (
    id TEXT PRIMARY KEY,
    saga_id TEXT NOT NULL,
    step_name TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    result TEXT NOT NULL DEFAULT '{}',
    started_at DATETIME,
    completed_at DATETIME,
    FOREIGN KEY (saga_id) REFERENCES sagas(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_sagas_status ON sagas(status);
CREATE INDEX IF NOT EXISTS idx_sagas_type ON sagas(saga_type);
CREATE INDEX IF NOT EXISTS idx_saga_steps_saga_id ON saga_steps(saga_id);
