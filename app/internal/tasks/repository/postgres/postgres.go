package postgres

import (
	"TODOLIST_Tasks/app/internal/tasks/domain"
	"TODOLIST_Tasks/app/internal/tasks/port"
	postgresql "TODOLIST_Tasks/app/pkg/client/postgres"
	"TODOLIST_Tasks/app/pkg/api/filter"
	"TODOLIST_Tasks/app/pkg/api/sort"
	"TODOLIST_Tasks/app/pkg/logging"
	"TODOLIST_Tasks/app/pkg/utils/operator"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
)

type repo struct {
	db     postgresql.Client
	logger logging.Logger
}

func NewRepository(db postgresql.Client, logger logging.Logger) port.TaskRepository {
	return &repo{db: db, logger: logger}
}

func (r *repo) Create(ctx context.Context, task domain.Task) error {
	eventData, err := json.Marshal(toTaskPayload(task))
	if err != nil {
		return fmt.Errorf("marshal task for outbox: %w", err)
	}

	numStatus := task.Status.StorageCode()
	outboxID := uuid.New().String()
	now := time.Now().UTC()

	if strings.TrimSpace(task.TagName) != "" {
		_, err = r.db.Exec(ctx, `
WITH
tag_resolved AS (
    SELECT tag_id FROM default_tag WHERE name = $1
    UNION ALL
    SELECT tag_id FROM custom_tag WHERE name = $1 AND user_id = $2
    LIMIT 1
),
tag_upserted AS (
    INSERT INTO custom_tag (name, user_id)
    SELECT $1, $2 WHERE NOT EXISTS (SELECT 1 FROM tag_resolved)
    ON CONFLICT (name, user_id) DO UPDATE SET name = EXCLUDED.name
    RETURNING tag_id
),
task_ins AS (
    INSERT INTO tasks (task_id, title, description, status, priory, due_date, created_at, user_id, tag_id)
    VALUES ($3,$4,$5,$6,$7,$8,$9,$2,
        COALESCE(
            (SELECT tag_id FROM tag_resolved),
            (SELECT tag_id FROM tag_upserted),
            'a1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d'::uuid
        ))
)
INSERT INTO outbox_events (id, aggregate_type, aggregate_id, event_type, event_data, created_at)
VALUES ($10,'task',$3,'created',$11,$12)`,
			task.TagName, task.UserID,
			task.ID, task.Title, task.Description, numStatus, task.Priority, task.DueDate, now,
			outboxID, string(eventData), now,
		)
	} else {
		_, err = r.db.Exec(ctx, `
WITH task_ins AS (
    INSERT INTO tasks (task_id, title, description, status, priory, due_date, created_at, user_id, tag_id)
    VALUES ($1,$2,$3,$4,$5,$6,$7,$8, COALESCE($9::uuid, 'a1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d'))
)
INSERT INTO outbox_events (id, aggregate_type, aggregate_id, event_type, event_data, created_at)
VALUES ($10,'task',$1,'created',$11,$12)`,
			task.ID, task.Title, task.Description, numStatus, task.Priority, task.DueDate, now, task.UserID, task.TagID,
			outboxID, string(eventData), now,
		)
	}

	if err != nil {
		r.logger.Errorf("Create: exec CTE task %s: %v", task.ID, err)
		return fmt.Errorf("create task: %w", err)
	}
	return nil
}

func (r *repo) CreateBatch(ctx context.Context, tasks []domain.Task) error {
	if len(tasks) == 0 {
		return nil
	}

	now := time.Now().UTC()
	taskValues := make([]string, 0, len(tasks))
	taskArgs := make([]interface{}, 0, len(tasks)*9)
	outboxValues := make([]string, 0, len(tasks))
	outboxArgs := make([]interface{}, 0, len(tasks)*4)
	ti, oi := 1, 1

	for _, task := range tasks {
		var tagID interface{}
		if task.TagID != nil && *task.TagID != "" {
			tagID = *task.TagID
		}
		taskValues = append(taskValues, fmt.Sprintf(
			"($%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,COALESCE($%d::uuid,'a1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d'::uuid))",
			ti, ti+1, ti+2, ti+3, ti+4, ti+5, ti+6, ti+7, ti+8,
		))
		taskArgs = append(taskArgs, task.ID, task.Title, task.Description, task.Status.StorageCode(), task.Priority, task.DueDate, now, task.UserID, tagID)
		ti += 9

		eventData, err := json.Marshal(toTaskPayload(task))
		if err != nil {
			return fmt.Errorf("marshal task %s: %w", task.ID, err)
		}
		outboxValues = append(outboxValues, fmt.Sprintf("($%d,'task',$%d,'created',$%d,$%d)", oi, oi+1, oi+2, oi+3))
		outboxArgs = append(outboxArgs, uuid.New().String(), task.ID, string(eventData), now)
		oi += 4
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("create batch begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	_, err = tx.Exec(ctx,
		fmt.Sprintf(`INSERT INTO tasks (task_id, title, description, status, priory, due_date, created_at, user_id, tag_id) VALUES %s`,
			strings.Join(taskValues, ",")),
		taskArgs...,
	)
	if err != nil {
		return fmt.Errorf("create batch insert tasks: %w", err)
	}

	_, err = tx.Exec(ctx,
		fmt.Sprintf(`INSERT INTO outbox_events (id, aggregate_type, aggregate_id, event_type, event_data, created_at) VALUES %s`,
			strings.Join(outboxValues, ",")),
		outboxArgs...,
	)
	if err != nil {
		return fmt.Errorf("create batch insert outbox: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("create batch commit: %w", err)
	}
	return nil
}

func (r *repo) FindByID(ctx context.Context, id string) (domain.Task, error) {
	const q = `
		SELECT t.task_id, t.title, t.description, t.priory, t.status,
		       t.due_date, t.created_at, t.user_id, t.tag_id,
		       COALESCE(dt.name, ct.name) AS tag_name
		FROM tasks t
		LEFT JOIN custom_tag ct ON t.tag_id = ct.tag_id
		LEFT JOIN default_tag dt ON t.tag_id = dt.tag_id
		WHERE t.task_id = $1`

	var m TaskModel
	err := r.db.QueryRow(ctx, q, id).Scan(
		&m.ID, &m.Title, &m.Description, &m.Priority,
		&m.Status, &m.DueDate, &m.CreatedAt, &m.UserID,
		&m.TagID, &m.TagName,
	)
	if err != nil {
		r.logger.Errorf("FindByID: scan task %s: %v", id, err)
		return domain.Task{}, err
	}
	return modelToEntity(m), nil
}

func (r *repo) FindByUser(ctx context.Context, userID string, sortOpts sort.Options, filterOpts filter.Option) ([]domain.Task, error) {
	psql := sq.StatementBuilder.PlaceholderFormat(sq.Dollar)

	qb := psql.
		Select("t.task_id", "t.title", "t.description", "t.priory", "t.status",
			"t.due_date", "t.created_at", "t.user_id", "t.tag_id",
			"COALESCE(ct.name, dt.name) AS name").
		From("public.tasks t").
		LeftJoin("custom_tag ct ON t.tag_id = ct.tag_id").
		LeftJoin("default_tag dt ON t.tag_id = dt.tag_id").
		Where(sq.Eq{"t.user_id": userID})

	conditions, args := buildFilterConditions(filterOpts)
	for i, cond := range conditions {
		resolved := strings.ReplaceAll(cond, "$%d", "?")
		argCount := strings.Count(resolved, "?")
		qb = qb.Where(resolved, args[:argCount]...)
		args = args[argCount:]
		_ = i
	}

	orderBy := buildOrderBy(sortOpts)
	if orderBy != "" {
		qb = qb.OrderBy(orderBy)
	}

	sqlStr, qArgs, err := qb.ToSql()
	if err != nil {
		return nil, fmt.Errorf("build SQL FindByUser: %w", err)
	}

	rows, err := r.db.Query(ctx, sqlStr, qArgs...)
	if err != nil {
		r.logger.Errorf("FindByUser: query user %s: %v", userID, err)
		return nil, fmt.Errorf("query FindByUser: %w", err)
	}
	defer rows.Close()

	return scanTasks(rows, r.logger, userID)
}

func (r *repo) FindByTag(ctx context.Context, userID, tagID string) ([]domain.Task, error) {
	psql := sq.StatementBuilder.PlaceholderFormat(sq.Dollar)

	qb := psql.
		Select("t.task_id", "t.title", "t.description", "t.priory", "t.status",
			"t.due_date", "t.created_at", "t.user_id", "t.tag_id",
			"COALESCE(ct.name, dt.name) AS name").
		From("public.tasks t").
		LeftJoin("custom_tag ct ON t.tag_id = ct.tag_id AND ct.user_id = ?", userID).
		LeftJoin("default_tag dt ON t.tag_id = dt.tag_id").
		Where(sq.Eq{"t.tag_id": tagID}).
		Where(sq.Or{sq.NotEq{"ct.tag_id": nil}, sq.NotEq{"dt.tag_id": nil}})

	sqlStr, args, err := qb.ToSql()
	if err != nil {
		return nil, fmt.Errorf("build SQL FindByTag: %w", err)
	}

	rows, err := r.db.Query(ctx, sqlStr, args...)
	if err != nil {
		r.logger.Errorf("FindByTag: query user %s tag %s: %v", userID, tagID, err)
		return nil, err
	}
	defer rows.Close()

	return scanTasks(rows, r.logger, userID)
}

func (r *repo) Update(ctx context.Context, id string, patch port.UpdatePatch) (domain.Task, error) {
	if _, err := uuid.Parse(id); err != nil {
		return domain.Task{}, fmt.Errorf("invalid uuid: %s", id)
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return domain.Task{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	setClauses := []string{}
	values := []interface{}{}
	idx := 1

	if patch.Title != nil {
		values = append(values, *patch.Title)
		setClauses = append(setClauses, fmt.Sprintf("title = $%d", idx))
		idx++
	}
	if patch.Description != nil {
		values = append(values, *patch.Description)
		setClauses = append(setClauses, fmt.Sprintf("description = $%d", idx))
		idx++
	}
	if patch.DueDate != nil {
		values = append(values, *patch.DueDate)
		setClauses = append(setClauses, fmt.Sprintf("due_date = $%d", idx))
		idx++
	}
	if patch.Priority != nil {
		values = append(values, *patch.Priority)
		setClauses = append(setClauses, fmt.Sprintf("priory = $%d", idx))
		idx++
	}
	if patch.Status != nil {
		// domain.Status.StorageCode() → "1"/"2"/"3" для SMALLINT
		values = append(values, patch.Status.StorageCode())
		setClauses = append(setClauses, fmt.Sprintf("status = $%d", idx))
		idx++
	}
	if patch.TagName != nil && strings.TrimSpace(*patch.TagName) != "" {
		var userID string
		if err := tx.QueryRow(ctx, `SELECT user_id FROM tasks WHERE task_id = $1`, id).Scan(&userID); err != nil {
			return domain.Task{}, fmt.Errorf("get user_id for task: %w", err)
		}

		var tagID string
		err := tx.QueryRow(ctx, `SELECT tag_id FROM default_tag WHERE name = $1`, *patch.TagName).Scan(&tagID)
		if err == sql.ErrNoRows {
			err = tx.QueryRow(ctx, `SELECT tag_id FROM custom_tag WHERE name = $1 AND user_id = $2`, *patch.TagName, userID).Scan(&tagID)
			if err == sql.ErrNoRows {
				err = tx.QueryRow(ctx, `INSERT INTO custom_tag (name, user_id) VALUES ($1, $2) RETURNING tag_id`, *patch.TagName, userID).Scan(&tagID)
			}
		}
		if err != nil {
			return domain.Task{}, fmt.Errorf("resolve tag: %w", err)
		}
		values = append(values, tagID)
		setClauses = append(setClauses, fmt.Sprintf("tag_id = $%d", idx))
		idx++
	}

	if len(setClauses) == 0 {
		return domain.Task{}, fmt.Errorf("нет полей для обновления")
	}

	query := fmt.Sprintf("UPDATE tasks SET %s WHERE task_id = $%d", strings.Join(setClauses, ", "), idx)
	values = append(values, id)

	res, err := tx.Exec(ctx, query, values...)
	if err != nil {
		return domain.Task{}, fmt.Errorf("exec update task %s: %w", id, err)
	}
	if res.RowsAffected() == 0 {
		return domain.Task{}, fmt.Errorf("task %s не найдена", id)
	}

	var m TaskModel
	var status string
	err = tx.QueryRow(ctx, `
		SELECT task_id, title, description, status, priory, due_date, user_id, tag_id, created_at
		FROM tasks WHERE task_id = $1`, id,
	).Scan(&m.ID, &m.Title, &m.Description, &status, &m.Priority, &m.DueDate, &m.UserID, &m.TagID, &m.CreatedAt)
	if err != nil {
		return domain.Task{}, fmt.Errorf("fetch updated task: %w", err)
	}
	m.Status = status
	updated := modelToEntity(m)

	eventData, err := json.Marshal(toTaskPayload(updated))
	if err != nil {
		return domain.Task{}, fmt.Errorf("marshal task for outbox: %w", err)
	}

	outboxQ, outboxArgs, err := sq.Insert("outbox_events").
		Columns("id", "aggregate_type", "aggregate_id", "event_type", "event_data", "created_at").
		Values(uuid.New().String(), "task", id, "update", eventData, time.Now().UTC()).
		PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		return domain.Task{}, fmt.Errorf("build outbox query: %w", err)
	}
	if _, err = tx.Exec(ctx, outboxQ, outboxArgs...); err != nil {
		return domain.Task{}, fmt.Errorf("insert outbox event: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.Task{}, fmt.Errorf("commit update tx: %w", err)
	}
	return updated, nil
}

func (r *repo) Delete(ctx context.Context, id string) error {
	if _, err := uuid.Parse(id); err != nil {
		return fmt.Errorf("invalid uuid: %s", id)
	}

	outboxID := uuid.New().String()
	now := time.Now().UTC()

	tag, err := r.db.Exec(ctx, `
WITH deleted AS (
    DELETE FROM tasks WHERE task_id = $1
    RETURNING task_id, title, description, status, priory, due_date, created_at, user_id, tag_id
)
INSERT INTO outbox_events (id, aggregate_type, aggregate_id, event_type, event_data, created_at)
SELECT $2, 'task', task_id, 'deleted',
    json_build_object(
        'id',          task_id::text,
        'title',       title,
        'description', description,
        'priory',      priory,
        'status',      CASE status::text
                           WHEN '1' THEN 'not_completed'
                           WHEN '2' THEN 'in_progress'
                           WHEN '3' THEN 'completed'
                           ELSE status::text
                       END,
        'due_date',    to_json(due_date),
        'created_at',  to_json(created_at),
        'user_id',     user_id::text,
        'tag_id',      COALESCE(tag_id::text, ''),
        'tags_name',   ''
    )::text,
    $3
FROM deleted`,
		id, outboxID, now,
	)
	if err != nil {
		r.logger.Errorf("Delete: exec CTE task %s: %v", id, err)
		return fmt.Errorf("delete task: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("task %s не найдена", id)
	}
	return nil
}

func (r *repo) DeleteBatch(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}

	now := time.Now().UTC()
	placeholders := make([]string, len(ids))
	args := make([]interface{}, 0, len(ids)+1)
	for i, id := range ids {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args = append(args, id)
	}
	nowIdx := len(ids) + 1
	args = append(args, now)

	query := fmt.Sprintf(`
WITH deleted AS (
    DELETE FROM tasks WHERE task_id IN (%s)
    RETURNING task_id, title, description, status, priory, due_date, created_at, user_id, tag_id
)
INSERT INTO outbox_events (id, aggregate_type, aggregate_id, event_type, event_data, created_at)
SELECT gen_random_uuid(), 'task', task_id, 'deleted',
    json_build_object(
        'id',          task_id::text,
        'title',       title,
        'description', description,
        'priory',      priory,
        'status',      CASE status::text
                           WHEN '1' THEN 'not_completed'
                           WHEN '2' THEN 'in_progress'
                           WHEN '3' THEN 'completed'
                           ELSE status::text
                       END,
        'due_date',    to_json(due_date),
        'created_at',  to_json(created_at),
        'user_id',     user_id::text,
        'tag_id',      COALESCE(tag_id::text, ''),
        'tags_name',   ''
    )::text,
    $%d
FROM deleted`,
		strings.Join(placeholders, ","), nowIdx,
	)

	if _, err := r.db.Exec(ctx, query, args...); err != nil {
		r.logger.Errorf("DeleteBatch: %d tasks: %v", len(ids), err)
		return fmt.Errorf("delete batch: %w", err)
	}
	return nil
}

// --- внутренние хелперы ---

// toTaskPayload создаёт JSON-сериализуемое представление задачи для outbox.
// Status хранится как human-readable строка (совместимость с форматом кэша).
func toTaskPayload(t domain.Task) taskPayloadJSON {
	return taskPayloadJSON{
		ID:          t.ID,
		Title:       t.Title,
		Description: t.Description,
		Priority:    t.Priority,
		Status:      string(t.Status),
		DueDate:     t.DueDate,
		UserID:      t.UserID,
		TagID:       t.TagID,
		TagName:     t.TagName,
		CreatedAt:   t.CreatedAt,
	}
}

type taskPayloadJSON struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Priority    string    `json:"priory"`
	Status      string    `json:"status"`
	DueDate     time.Time `json:"due_date"`
	UserID      string    `json:"user_id"`
	TagID       *string   `json:"tag_id,omitempty"`
	TagName     string    `json:"tags_name"`
	CreatedAt   time.Time `json:"created_at"`
}

func scanTasks(rows interface {
	Next() bool
	Scan(...interface{}) error
	Err() error
}, logger logging.Logger, context string) ([]domain.Task, error) {
	var tasks []domain.Task
	for rows.Next() {
		var m TaskModel
		if err := rows.Scan(
			&m.ID, &m.Title, &m.Description, &m.Priority,
			&m.Status, &m.DueDate, &m.CreatedAt, &m.UserID,
			&m.TagID, &m.TagName,
		); err != nil {
			logger.Errorf("scanTasks %s: %v", context, err)
			return nil, fmt.Errorf("scan row: %w", err)
		}
		tasks = append(tasks, modelToEntity(m))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return tasks, nil
}

// --- query builders (внутренние, не экспортируются) ---

var allowedColumns = map[string]bool{
	"title": true, "description": true, "status": true,
	"priory": true, "due_date": true, "created_at": true, "tag_id": true,
}

func buildFilterConditions(opt filter.Option) ([]string, []interface{}) {
	var conditions []string
	var args []interface{}

	for _, field := range opt.Fields {
		if !allowedColumns[field.Name] {
			continue
		}
		sqlOp, err := operator.GetSQLOperator(field.Operator)
		if err != nil {
			continue
		}
		col := "t." + field.Name
		switch sqlOp {
		case "ILIKE":
			conditions = append(conditions, fmt.Sprintf("%s ILIKE $%%d", col))
			args = append(args, "%"+field.Value+"%")
		case "BETWEEN":
			parts := strings.Split(field.Value, ":")
			if len(parts) == 2 {
				conditions = append(conditions, fmt.Sprintf("%s BETWEEN $%%d AND $%%d", col))
				args = append(args, parseValue(parts[0]), parseValue(parts[1]))
			}
		default:
			conditions = append(conditions, fmt.Sprintf("%s %s $%%d", col, sqlOp))
			args = append(args, parseValue(field.Value))
		}
	}
	return conditions, args
}

func buildOrderBy(opt sort.Options) string {
	if opt.Fields == "" {
		return ""
	}
	return fmt.Sprintf("%s %s", opt.Fields, opt.Order)
}

func parseValue(value string) interface{} {
	formats := []string{"2006-01-02-15-04", "2006-01-02", time.RFC3339}
	for _, f := range formats {
		if t, err := time.Parse(f, value); err == nil {
			return t
		}
	}
	return value
}
