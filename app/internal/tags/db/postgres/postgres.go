package postgres

import (
	"TODOLIST_Tasks/app/internal/tags/model"
	storage2 "TODOLIST_Tasks/app/internal/tags/storage"
	postgresql "TODOLIST_Tasks/app/pkg/client/postgres"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/google/uuid"
)

type repositoryTags struct {
	ClientPostgres1 postgresql.Client
}

func (r *repositoryTags) FindTagByID(ctx context.Context, id string) (bool, error) {
	// Подготавливаем запрос для поиска тега по ID
	var exists bool

	// Пытаемся найти тег с переданным id
	query := `SELECT EXISTS (SELECT 1 FROM custom_tag WHERE tag_id = $1)`

	// Выполняем запрос и проверяем результат
	err := r.ClientPostgres1.QueryRow(ctx, query, id).Scan(&exists)
	if err != nil {
		return false, err
	}

	// Возвращаем результат поиска
	return exists, nil
}

func (r *repositoryTags) CreateTags(ctx context.Context, tags model.Tags, userID string) (string, error) {
	err := r.ClientPostgres1.QueryRow(ctx,
		`INSERT INTO custom_tag(name, user_id) VALUES($1,$2) RETURNING tag_id`, tags.Name, userID).Scan(&tags.Id)
	if err != nil {
		return "", fmt.Errorf("не удалось создать тег '%s': %w", tags.Name, err)
	}

	return tags.Id, nil
}

func (r *repositoryTags) UpdateTags(ctx context.Context, id string, tagDTO model.TagsDTO, userID string) error {
	// Проверка на допустимый UUID
	if _, err := uuid.Parse(id); err != nil {
		return fmt.Errorf("недопустимый uuid: %s", id)
	}

	// Проверка, что поле name не пустое
	if tagDTO.Name == "" {
		return fmt.Errorf("name не может быть пустым")
	}

	// Получаем текущее имя тега

	// Проверка на существование тега с таким же именем, если имя изменилось
	var existingTagId string

	// Проверяем, существует ли тег с таким именем и отличным от текущего ID
	err := r.ClientPostgres1.QueryRow(ctx,
		`SELECT tag_id FROM custom_tag WHERE name = $1 AND user_id = $3 AND tag_id != $2 `,
		tagDTO.Name, id, userID).Scan(&existingTagId)

	// Проверяем, если ошибка не равна nil
	if err != nil && errors.Is(err, sql.ErrNoRows) {
		return err
	}
	// Если тег с таким именем уже существует, возвращаем ошибку
	if existingTagId != "" {
		return fmt.Errorf("тег с именем '%s' уже существует", tagDTO.Name)
	}

	// Если такого тега нет, обновляем имя
	_, err = r.ClientPostgres1.Exec(ctx,
		`UPDATE custom_tag SET name = $1 WHERE tag_id = $2 AND user_id = $3`, tagDTO.Name, id, userID)
	if err != nil {
		return fmt.Errorf("не удалось обновить тег: %w", err)
	}

	return nil
}

func (r *repositoryTags) DeleteTags(ctx context.Context, id string, userID string) error {
	if _, err := uuid.Parse(id); err != nil {
		return fmt.Errorf("Invalid uuid:%s", id)
	}
	query := `DELETE FROM custom_tag WHERE tag_id = $1 AND user_id = $2`
	_, err := r.ClientPostgres1.Exec(ctx, query, id, userID)
	if err != nil {
		return fmt.Errorf("failed to delete tags: %w", err)
	}
	return nil
}

func (r *repositoryTags) FindAllByUser(ctx context.Context, userId string) ([]model.Tags, error) {
	// SQL-запрос для дефолтных тегов с фиктивным user_id
	queryDefault := `
		SELECT tag_id, name, NULL AS user_id
		FROM public.default_tag
	`

	// SQL-запрос для пользовательских тегов
	queryCustom := `
		SELECT tag_id, name, user_id
		FROM public.custom_tag
		WHERE user_id = $1
	`

	// Объединяем оба запроса через UNION ALL
	finalQuery := fmt.Sprintf("(%s) UNION ALL (%s)", queryDefault, queryCustom)

	rows, err := r.ClientPostgres1.Query(ctx, finalQuery, userId)
	if err != nil {
		return nil, fmt.Errorf("ошибка выполнения запроса: %v", err)
	}
	defer rows.Close()

	var tags []model.Tags

	for rows.Next() {
		var tag model.Tags
		if err := rows.Scan(&tag.Id, &tag.Name, &tag.UserID); err != nil {
			return nil, fmt.Errorf("ошибка чтения строки: %v", err)
		}
		tags = append(tags, tag)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ошибка при обходе строк: %v", err)
	}

	return tags, nil
}

func (r *repositoryTags) FindOneTags(ctx context.Context, id string, userID string) (model.Tags, error) {
	var tag model.Tags

	// Попытка найти в default_tag
	queryDefault := `SELECT tag_id, name FROM default_tag WHERE tag_id = $1`
	err := r.ClientPostgres1.QueryRow(ctx, queryDefault, id).Scan(&tag.Id, &tag.Name)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			// Не нашли в default_tag — ищем в custom_tag
			queryCustom := `SELECT tag_id, name, user_id FROM custom_tag WHERE tag_id = $1 AND user_id = $2`
			err = r.ClientPostgres1.QueryRow(ctx, queryCustom, id, userID).Scan(&tag.Id, &tag.Name, &tag.UserID)
			if err != nil {
				return model.Tags{}, err
			}
			return tag, nil
		}
		// Другие ошибки при запросе к default_tag
		return model.Tags{}, err
	}

	// Нашли в default_tag
	return tag, nil
}

func NewRepository(client postgresql.Client) storage2.Repository {
	return &repositoryTags{ClientPostgres1: client}
}
