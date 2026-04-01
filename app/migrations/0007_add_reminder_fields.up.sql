ALTER TABLE tasks
    ADD COLUMN reminder_60m_sent_at TIMESTAMP,
    ADD COLUMN reminder_15m_sent_at TIMESTAMP,
    ADD COLUMN reminder_5m_sent_at  TIMESTAMP;

CREATE INDEX idx_tasks_reminder_pending
    ON tasks(due_date, user_id)
    WHERE status != 3;
