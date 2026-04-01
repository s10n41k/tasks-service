DROP INDEX IF EXISTS idx_tasks_reminder_pending;

ALTER TABLE tasks
    DROP COLUMN IF EXISTS reminder_60m_sent_at,
    DROP COLUMN IF EXISTS reminder_15m_sent_at,
    DROP COLUMN IF EXISTS reminder_5m_sent_at;
