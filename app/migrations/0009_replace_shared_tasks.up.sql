-- Удаляем старые таблицы (схема была неправильной — ссылалась на tasks и users,
-- которых нет в tasks-service). Заменяем на автономные совместные задачи.
DROP TABLE IF EXISTS shared_subtasks;
DROP TABLE IF EXISTS shared_tasks;

-- Автономные совместные задачи (не зависят от таблицы tasks)
CREATE TABLE IF NOT EXISTS shared_tasks (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    proposer_id  UUID NOT NULL,
    addressee_id UUID NOT NULL,
    title        TEXT NOT NULL,
    description  TEXT NOT NULL DEFAULT '',
    priority     VARCHAR(10) NOT NULL DEFAULT 'red',
    due_date     TIMESTAMPTZ,
    status       VARCHAR(20) NOT NULL DEFAULT 'pending',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Подзадачи совместной задачи
CREATE TABLE IF NOT EXISTS shared_subtasks (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    shared_task_id UUID NOT NULL REFERENCES shared_tasks(id) ON DELETE CASCADE,
    title          TEXT NOT NULL,
    assignee_id    UUID NOT NULL,
    is_done        BOOLEAN NOT NULL DEFAULT FALSE,
    order_num      INT NOT NULL DEFAULT 0,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_st_proposer  ON shared_tasks(proposer_id);
CREATE INDEX IF NOT EXISTS idx_st_addressee ON shared_tasks(addressee_id);
CREATE INDEX IF NOT EXISTS idx_sst_task     ON shared_subtasks(shared_task_id);

-- Добавляем имя пользователя для отображения в совместных задачах
ALTER TABLE user_subscriptions ADD COLUMN IF NOT EXISTS name TEXT NOT NULL DEFAULT '';
