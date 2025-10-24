package postgres

import (
	model2 "TODOLIST_Tasks/app/internal/tasks/model"
	"TODOLIST_Tasks/app/internal/tasks/storage/postgres"
	postgresql "TODOLIST_Tasks/app/pkg/client/postgres"
	"TODOLIST_Tasks/app/pkg/utils/translator"
	"context"
	"database/sql"
	"errors"
	"fmt"
	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"strings"
	"time"
)

type repositoryTasks struct {
	ClientPostgres1 postgresql.Client
}

func (r *repositoryTasks) CreateTask(ctx context.Context, task model2.Task) (string, error) {

	// Сохраняем задачу
	err := r.ClientPostgres1.QueryRow(ctx,
		`INSERT INTO tasks(task_id, title, description, status, priory, due_date, user_id, tag_id)
         VALUES($1, $2, $3, $4, $5, $6, $7, $8) RETURNING task_id`,
		task.Id, task.Title, task.Description, translator.AntiTranslator(task.Status), task.Priory, task.DueDate, task.UserID, task.TagID,
	).Scan(&task.Id)

	if err != nil && !errors.Is(err, ctx.Err()) {
		return "", fmt.Errorf("ошибка при сохранении задачи: %w", err)
	}
	return task.Id, nil
}

func (r *repositoryTasks) FindOneTask(ctx context.Context, id string) (model2.Task, error) {
	var task model2.Task

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
		&task.UserID, &task.TagID, &task.TagsName)
	if err != nil && !errors.Is(err, ctx.Err()) {
		return task, err
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
			"COALESCE(ct.name, dt.name) AS name", // Берем имя из любой таблицы тегов
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
		if err := rows.Scan(
			&task.Id, &task.Title, &task.Description, &task.Priory,
			&task.Status, &task.DueDate, &task.CreatedAt, &task.UserID,
			&task.TagID, &task.TagsName,
		); err != nil {
			return nil, fmt.Errorf("ошибка чтения строки: %v", err)
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
		LeftJoin("custom_tag ct ON t.tag_id = ct.tag_id AND ct.user_id = ?", userId). // Только кастомные теги этого пользователя
		LeftJoin("default_tag dt ON t.tag_id = dt.tag_id").                           // Все дефолтные теги
		Where(sq.Eq{"t.tag_id": tagId}).
		Where(sq.Or{
			sq.NotEq{"ct.tag_id": nil}, // Есть в кастомных тегах (у этого пользователя)
			sq.NotEq{"dt.tag_id": nil}, // Или есть в дефолтных тегах
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
		if err := rows.Scan(&task.Id, &task.Title, &task.Description, &task.Priory, &task.Status,
			&task.DueDate, &task.CreatedAt, &task.UserID, &task.TagID, &task.TagsName); err != nil {
			return nil, err
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

	setClauses := []string{}
	value := []interface{}{}
	valueIndex := 1

	if task.Title != nil {
		value = append(value, *task.Title)
		setClauses = append(setClauses, fmt.Sprintf("title = $%d", valueIndex))
		valueIndex++
	}

	if task.Description != nil {
		value = append(value, *task.Description)
		setClauses = append(setClauses, fmt.Sprintf("description = $%d", valueIndex))
		valueIndex++
	}

	if task.DueDate != nil {
		value = append(value, *task.DueDate)
		setClauses = append(setClauses, fmt.Sprintf("due_date = $%d", valueIndex))
		valueIndex++
	}

	if task.Priory != nil {
		value = append(value, *task.Priory)
		setClauses = append(setClauses, fmt.Sprintf("priory = $%d", valueIndex))
		valueIndex++
	}

	if task.Status != nil {
		value = append(value, *task.Status)
		setClauses = append(setClauses, fmt.Sprintf("status = $%d", valueIndex))
		valueIndex++
	}

	// Обработка изменения тега по имени
	if task.TagName != nil && strings.TrimSpace(*task.TagName) != "" {
		var tagId string

		// Пытаемся найти тег по имени
		err := r.ClientPostgres1.QueryRow(ctx,
			`SELECT tag_id FROM tags WHERE name = $1`, *task.TagName).Scan(&tagId)

		// Если тега нет — создаём
		if err == sql.ErrNoRows {
			err = r.ClientPostgres1.QueryRow(ctx,
				`INSERT INTO tags (name) VALUES ($1) RETURNING tag_id`, *task.TagName).Scan(&tagId)
			if err != nil {
				return model2.Task{}, fmt.Errorf("не удалось создать тег: %w", err)
			}
		} else if err != nil {
			return model2.Task{}, fmt.Errorf("ошибка при получении тега: %w", err)
		}

		value = append(value, tagId)
		setClauses = append(setClauses, fmt.Sprintf("tag_id = $%d", valueIndex))
		valueIndex++
	}

	if len(setClauses) == 0 {
		return model2.Task{}, fmt.Errorf("нет полей для обновления")
	}

	query := fmt.Sprintf("UPDATE tasks SET %s WHERE task_id = $%d", strings.Join(setClauses, ", "), valueIndex)
	value = append(value, id)

	_, err := r.ClientPostgres1.Exec(ctx, query, value...)
	if err != nil {
		return model2.Task{}, fmt.Errorf("не удалось обновить задачу: %w", err)
	}
	oneTask, err := r.FindOneTask(ctx, id)
	if err != nil {
		return model2.Task{}, err
	}
	return oneTask, nil
}

func (r *repositoryTasks) DeleteTask(ctx context.Context, id string) (string, error) {
	if _, err := uuid.Parse(id); err != nil {
		return "", fmt.Errorf("Invalid uuid:%s", id)
	}
	query := `DELETE FROM tasks WHERE task_id = $1`
	_, err := r.ClientPostgres1.Exec(ctx, query, id)
	if err != nil {
		return "", fmt.Errorf("failed to delete tags: %w", err)
	}
	return "delete ok", nil
}

func NewRepository(client postgresql.Client) postgres.Repository {
	return &repositoryTasks{ClientPostgres1: client}
}
