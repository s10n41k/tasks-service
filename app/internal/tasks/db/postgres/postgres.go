package postgres

import (
	model2 "TODOLIST_Tasks/app/internal/tasks/model"
	"TODOLIST_Tasks/app/internal/tasks/storage/postgres"
	postgresql "TODOLIST_Tasks/app/pkg/client/postgres"
	"TODOLIST_Tasks/app/pkg/logging"
	"TODOLIST_Tasks/app/pkg/utils/translator"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"strings"
	"time"
)

type repositoryTasks struct {
	ClientPostgres1 postgresql.Client
	logger          logging.Logger
}

func (r *repositoryTasks) CreateTask(ctx context.Context, task model2.Task) error {
	tx, err := r.ClientPostgres1.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback(context.Background())
	}()

	// INSERT в основную таблицу tasks
	_, err = tx.Exec(ctx, `
        INSERT INTO tasks (task_id, title, description, status, priory, due_date, created_at, user_id, tag_id)
        VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
    `, task.Id, task.Title, task.Description, task.Status, task.Priory, task.DueDate, time.Now(), task.UserID, task.TagID)
	if err != nil {
		return fmt.Errorf("insert task: %w", err)
	}

	// INSERT в outbox для своих целей
	eventData, _ := json.Marshal(task)
	_, err = tx.Exec(ctx, `
    INSERT INTO outbox_events (aggregate_type, aggregate_id, event_type, event_data, created_at)
    VALUES ($1, $2, $3, $4, $5)
`, "task", task.Id, "created", string(eventData), time.Now())
	if err != nil {
		return fmt.Errorf("insert outbox: %w", err)
	}

	// COMMIT
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit failed: %w", err)
	}

	return nil
}

func (r *repositoryTasks) FindOneTask(ctx context.Context, id string) (model2.Task, error) {
	var task model2.Task
	var tagID sql.NullString
	var tagName sql.NullString

	query := `SELECT t.task_id, t.title, t.description, t.priory, t.status, t.due_date,
           t.created_at, t.user_id, t.tag_id, 
           COALESCE(dt.name, ct.name) AS tag_name
    FROM tasks t
    LEFT JOIN custom_tag ct ON t.tag_id = ct.tag_id
    LEFT JOIN default_tag dt ON t.tag_id = dt.tag_id
    WHERE t.task_id = $1`

	err := r.ClientPostgres1.QueryRow(ctx, query, id).Scan(
		&task.Id, &task.Title, &task.Description, &task.Priory,
		&task.Status, &task.DueDate, &task.CreatedAt,
		&task.UserID, &tagID, &tagName)

	if err != nil && !errors.Is(err, ctx.Err()) {
		return task, err
	}

	// Конвертируем sql.NullString в *string
	if tagID.Valid {
		tagIDStr := tagID.String
		task.TagID = &tagIDStr
	} else {
		task.TagID = nil
	}

	if tagName.Valid {
		task.TagsName = tagName.String
	}

	task.CreatedAt = task.CreatedAt.In(time.Local)
	return task, nil
}

func (r *repositoryTasks) FindAllTasks(ctx context.Context, sortOptions postgres.SortOptions, filterOptions postgres.FilterOptions, userId string) ([]model2.Task, error) {
	psql := sq.StatementBuilder.PlaceholderFormat(sq.Dollar)

	// Строим основной запрос
	qb := psql.
		Select(
			"t.task_id", "t.title", "t.description", "t.priory",
			"t.status", "t.due_date", "t.created_at", "t.user_id",
			"t.tag_id",
			"COALESCE(ct.name, dt.name) AS name",
		).
		From("public.tasks t").
		LeftJoin("custom_tag ct ON t.tag_id = ct.tag_id").
		LeftJoin("default_tag dt ON t.tag_id = dt.tag_id").
		Where(sq.Eq{"t.user_id": userId})

	// Добавляем фильтры, если есть
	if filterOptions != nil {
		filterQuery := filterOptions.CreateQuery()
		if filterQuery != "" {
			qb = qb.Where(filterQuery)
		}
	}

	// Добавляем сортировку
	if sortOptions != nil && sortOptions.GetOrderBy() != "" {
		qb = qb.OrderBy(sortOptions.GetOrderBy())
	}

	// Генерируем SQL и аргументы
	sqlStr, args, err := qb.ToSql()
	if err != nil {
		return nil, fmt.Errorf("ошибка генерации SQL-запроса: %v", err)
	}

	// Выполняем запрос
	rows, err := r.ClientPostgres1.Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("ошибка выполнения запроса: %v", err)
	}
	defer rows.Close()

	// Результат
	var tasks []model2.Task

	for rows.Next() {
		var task model2.Task
		var tagID sql.NullString
		var tagName sql.NullString

		if err := rows.Scan(
			&task.Id, &task.Title, &task.Description, &task.Priory,
			&task.Status, &task.DueDate, &task.CreatedAt, &task.UserID,
			&tagID, &tagName,
		); err != nil {
			return nil, fmt.Errorf("ошибка чтения строки: %v", err)
		}

		// Конвертируем sql.NullString в *string
		if tagID.Valid {
			tagIDStr := tagID.String
			task.TagID = &tagIDStr
		} else {
			task.TagID = nil
		}

		if tagName.Valid {
			task.TagsName = tagName.String
		}

		tasks = append(tasks, task)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ошибка при обходе строк: %v", err)
	}

	return tasks, nil
}

func (r *repositoryTasks) FindAllByTag(ctx context.Context, userId string, tagId string) ([]model2.Task, error) {
	psql := sq.StatementBuilder.PlaceholderFormat(sq.Dollar)

	qb := psql.
		Select(
			"t.task_id", "t.title", "t.description", "t.priory",
			"t.status", "t.due_date", "t.created_at", "t.user_id",
			"t.tag_id",
			"COALESCE(ct.name, dt.name) AS name",
		).
		From("public.tasks t").
		LeftJoin("custom_tag ct ON t.tag_id = ct.tag_id AND ct.user_id = ?", userId).
		LeftJoin("default_tag dt ON t.tag_id = dt.tag_id").
		Where(sq.Eq{"t.tag_id": tagId}).
		Where(sq.Or{
			sq.NotEq{"ct.tag_id": nil},
			sq.NotEq{"dt.tag_id": nil},
		})

	sqlStr, args, err := qb.ToSql()
	if err != nil {
		return nil, err
	}

	rows, err := r.ClientPostgres1.Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []model2.Task
	for rows.Next() {
		var task model2.Task
		var tagID sql.NullString
		var tagName sql.NullString

		if err := rows.Scan(&task.Id, &task.Title, &task.Description, &task.Priory, &task.Status,
			&task.DueDate, &task.CreatedAt, &task.UserID, &tagID, &tagName); err != nil {
			return nil, err
		}

		// Конвертируем sql.NullString в *string
		if tagID.Valid {
			tagIDStr := tagID.String
			task.TagID = &tagIDStr
		} else {
			task.TagID = nil
		}

		if tagName.Valid {
			task.TagsName = tagName.String
		}

		tasks = append(tasks, task)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return tasks, nil
}

func (r *repositoryTasks) UpdateTask(ctx context.Context, id string, task model2.TaskUpdateDTO) (model2.Task, error) {
	if _, err := uuid.Parse(id); err != nil {
		return model2.Task{}, fmt.Errorf("недопустимый uuid: %s", id)
	}

	// Начинаем транзакцию
	tx, err := r.ClientPostgres1.Begin(ctx)
	if err != nil {
		return model2.Task{}, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	setClauses := []string{}
	values := []interface{}{}
	valueIndex := 1

	if task.Title != nil {
		values = append(values, *task.Title)
		setClauses = append(setClauses, fmt.Sprintf("title = $%d", valueIndex))
		valueIndex++
	}

	if task.Description != nil {
		values = append(values, *task.Description)
		setClauses = append(setClauses, fmt.Sprintf("description = $%d", valueIndex))
		valueIndex++
	}

	if task.DueDate != nil {
		values = append(values, *task.DueDate)
		setClauses = append(setClauses, fmt.Sprintf("due_date = $%d", valueIndex))
		valueIndex++
	}

	if task.Priory != nil {
		values = append(values, *task.Priory)
		setClauses = append(setClauses, fmt.Sprintf("priory = $%d", valueIndex))
		valueIndex++
	}

	if task.Status != nil {
		values = append(values, translator.AntiTranslator(*task.Status))
		setClauses = append(setClauses, fmt.Sprintf("status = $%d", valueIndex))
		valueIndex++
	}

	// Обработка изменения тега по имени
	if task.TagName != nil && strings.TrimSpace(*task.TagName) != "" {
		var tagId string

		// Пытаемся найти тег по имени
		err := tx.QueryRow(ctx, `SELECT tag_id FROM tags WHERE name = $1`, *task.TagName).Scan(&tagId)

		// Если тега нет — создаём
		if err == sql.ErrNoRows {
			err = tx.QueryRow(ctx, `INSERT INTO tags (name) VALUES ($1) RETURNING tag_id`, *task.TagName).Scan(&tagId)
			if err != nil {
				return model2.Task{}, fmt.Errorf("не удалось создать тег: %w", err)
			}
		} else if err != nil {
			return model2.Task{}, fmt.Errorf("ошибка при получении тега: %w", err)
		}

		values = append(values, tagId)
		setClauses = append(setClauses, fmt.Sprintf("tag_id = $%d", valueIndex))
		valueIndex++
	}

	if len(setClauses) == 0 {
		return model2.Task{}, fmt.Errorf("нет полей для обновления")
	}

	// 1. Обновляем задачу
	query := fmt.Sprintf("UPDATE tasks SET %s WHERE task_id = $%d", strings.Join(setClauses, ", "), valueIndex)
	values = append(values, id)

	result, err := tx.Exec(ctx, query, values...)
	if err != nil {
		return model2.Task{}, fmt.Errorf("не удалось обновить задачу: %w", err)
	}

	if result.RowsAffected() == 0 {
		return model2.Task{}, fmt.Errorf("задача с ID %s не найдена", id)
	}

	// 2. Получаем обновленную задачу
	var updatedTask model2.Task
	var status string
	var tagID sql.NullString

	err = tx.QueryRow(ctx, `
		SELECT task_id, title, description, status, priory, due_date, user_id, tag_id, created_at 
		FROM tasks WHERE task_id = $1
	`, id).Scan(
		&updatedTask.Id, &updatedTask.Title, &updatedTask.Description, &status,
		&updatedTask.Priory, &updatedTask.DueDate, &updatedTask.UserID, &tagID, &updatedTask.CreatedAt,
	)
	if err != nil {
		return model2.Task{}, fmt.Errorf("ошибка при получении обновленной задачи: %w", err)
	}

	// Конвертируем sql.NullString в *string
	if tagID.Valid {
		tagIDStr := tagID.String
		updatedTask.TagID = &tagIDStr
	} else {
		updatedTask.TagID = nil
	}

	updatedTask.Status = translator.Translator(status)

	// 3. Вставляем событие в outbox
	eventData, err := json.Marshal(updatedTask)
	if err != nil {
		return model2.Task{}, fmt.Errorf("failed to marshal task for outbox: %w", err)
	}

	outboxQuery, outboxArgs, err := sq.
		Insert("outbox_events").
		Columns("id", "aggregate_type", "aggregate_id", "event_type", "event_data", "created_at").
		Values(uuid.New().String(), "task", id, "update", eventData, time.Now().UTC()).
		PlaceholderFormat(sq.Dollar).
		ToSql()

	if err != nil {
		return model2.Task{}, fmt.Errorf("failed to build outbox insert query: %w", err)
	}

	_, err = tx.Exec(ctx, outboxQuery, outboxArgs...)
	if err != nil {
		return model2.Task{}, fmt.Errorf("failed to insert outbox event: %w", err)
	}

	// Коммитим транзакцию
	if err := tx.Commit(ctx); err != nil {
		return model2.Task{}, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return updatedTask, nil
}

func (r *repositoryTasks) DeleteTask(ctx context.Context, id string) (string, error) {
	if _, err := uuid.Parse(id); err != nil {
		return "", fmt.Errorf("Invalid uuid:%s", id)
	}

	// Начинаем транзакцию
	tx, err := r.ClientPostgres1.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// 1. Сначала получаем задачу для outbox
	var task model2.Task
	var status string
	var tagID sql.NullString

	err = tx.QueryRow(ctx, `
		SELECT task_id, title, description, status, priory, due_date, user_id, tag_id, created_at 
		FROM tasks WHERE task_id = $1
	`, id).Scan(
		&task.Id, &task.Title, &task.Description, &status,
		&task.Priory, &task.DueDate, &task.UserID, &tagID, &task.CreatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("задача с ID %s не найдена", id)
		}
		return "", fmt.Errorf("ошибка при поиске задачи: %w", err)
	}

	// Конвертируем sql.NullString в *string
	if tagID.Valid {
		tagIDStr := tagID.String
		task.TagID = &tagIDStr
	} else {
		task.TagID = nil
	}

	task.Status = translator.Translator(status)

	// 2. Удаляем задачу
	query := `DELETE FROM tasks WHERE task_id = $1`
	result, err := tx.Exec(ctx, query, id)
	if err != nil {
		return "", fmt.Errorf("failed to delete task: %w", err)
	}

	if result.RowsAffected() == 0 {
		return "", fmt.Errorf("задача с ID %s не найдена", id)
	}

	// 3. Вставляем событие в outbox
	eventData, err := json.Marshal(task)
	if err != nil {
		return "", fmt.Errorf("failed to marshal task for outbox: %w", err)
	}

	outboxQuery, outboxArgs, err := sq.
		Insert("outbox_events").
		Columns("id", "aggregate_type", "aggregate_id", "event_type", "event_data", "created_at").
		Values(uuid.New().String(), "task", id, "delete", eventData, time.Now().UTC()).
		PlaceholderFormat(sq.Dollar).
		ToSql()

	if err != nil {
		return "", fmt.Errorf("failed to build outbox insert query: %w", err)
	}

	_, err = tx.Exec(ctx, outboxQuery, outboxArgs...)
	if err != nil {
		return "", fmt.Errorf("failed to insert outbox event: %w", err)
	}

	// Коммитим транзакцию
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("failed to commit transaction: %w", err)
	}

	return "delete ok", nil
}

func NewRepository(client postgresql.Client, logger logging.Logger) postgres.Repository {
	return &repositoryTasks{ClientPostgres1: client, logger: logger}
}
