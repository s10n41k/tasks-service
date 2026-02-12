-- TASKS - оптимизированная таблица
CREATE TABLE tasks (
    task_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title TEXT NOT NULL,
    description TEXT,
    status SMALLINT DEFAULT 2 CHECK (status IN (1, 2, 3)),
    priory VARCHAR(10) DEFAULT 'green' CHECK (priory IN ('red', 'blue', 'green')),
    due_date TIMESTAMP DEFAULT NOW() + INTERVAL '1 day',
    created_at TIMESTAMP DEFAULT NOW(),
    user_id UUID NOT NULL,
    tag_id UUID DEFAULT 'a1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d',
    CONSTRAINT valid_status CHECK (status BETWEEN 1 AND 3)
);

-- ОБЫЧНЫЙ индекс (без CONCURRENTLY в миграции!)
CREATE INDEX idx_tasks_user_status_created
    ON tasks(user_id, status, created_at DESC)
    WHERE status != 3;