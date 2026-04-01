-- Подзадачи обычных задач (не путать с shared_subtasks совместных задач).
-- Являются Entity внутри агрегата Task (ON DELETE CASCADE).
-- Не имеют assignee: выполняются владельцем задачи.
-- Бизнес-правило: если все подзадачи выполнены — задача автоматически завершается.
CREATE TABLE IF NOT EXISTS subtasks (
    id        UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id   UUID NOT NULL REFERENCES tasks(task_id) ON DELETE CASCADE,
    title     TEXT NOT NULL,
    is_done   BOOLEAN NOT NULL DEFAULT FALSE,
    order_num INT NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_subtasks_task ON subtasks(task_id);
