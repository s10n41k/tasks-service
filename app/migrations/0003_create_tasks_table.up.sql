CREATE TABLE tasks (
    task_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title TEXT NOT NULL,
    description TEXT,
    status task_status DEFAULT '2',
    priory task_priory DEFAULT 'green',
    due_date TIMESTAMP DEFAULT NOW() + INTERVAL '1 day',
    created_at TIMESTAMP DEFAULT NOW(),
    user_id UUID NOT NULL,
    tag_id UUID
);

CREATE INDEX idx_tasks_user_id ON tasks(user_id);
CREATE INDEX idx_tasks_tag_id ON tasks(tag_id);
CREATE UNIQUE INDEX uniq_task_title_per_tag ON tasks(title, tag_id) WHERE tag_id IS NOT NULL;