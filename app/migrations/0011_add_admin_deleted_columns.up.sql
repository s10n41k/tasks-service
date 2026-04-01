-- Добавляем поля мягкого удаления администратором к таблице tasks (IF NOT EXISTS — безопасно)
ALTER TABLE tasks
    ADD COLUMN IF NOT EXISTS admin_deleted    BOOLEAN     DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS admin_deleted_at TIMESTAMPTZ;

-- Добавляем поля мягкого удаления администратором к таблице shared_tasks
ALTER TABLE shared_tasks
    ADD COLUMN IF NOT EXISTS admin_deleted    BOOLEAN     DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS admin_deleted_at TIMESTAMPTZ;
