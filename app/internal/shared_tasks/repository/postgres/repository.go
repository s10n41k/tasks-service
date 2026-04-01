package postgres

import (
	"TODOLIST_Tasks/app/internal/shared_tasks/domain"
	"TODOLIST_Tasks/app/internal/shared_tasks/port"
	postgresql "TODOLIST_Tasks/app/pkg/client/postgres"
	"TODOLIST_Tasks/app/pkg/logging"
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v4"
)

type repo struct {
	db     postgresql.Client
	logger logging.Logger
}

func New(db postgresql.Client, logger logging.Logger) port.SharedTaskRepository {
	return &repo{db: db, logger: logger}
}

func (r *repo) Propose(
	ctx context.Context,
	proposerID, addresseeID, title, desc, priority, dueDate string,
	subtasks []port.SubtaskInput,
) (string, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if priority == "" {
		priority = "red"
	}

	var dueDateArg interface{}
	if dueDate != "" {
		dueDateArg = dueDate
	}

	var taskID string
	err = tx.QueryRow(ctx, `
		INSERT INTO shared_tasks (proposer_id, addressee_id, title, description, priority, due_date)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6)
		RETURNING id`,
		proposerID, addresseeID, title, desc, priority, dueDateArg,
	).Scan(&taskID)
	if err != nil {
		return "", fmt.Errorf("insert shared_task: %w", err)
	}

	for i, s := range subtasks {
		if s.Title == "" || s.AssigneeID == "" {
			continue
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO shared_subtasks (shared_task_id, title, assignee_id, order_num)
			VALUES ($1::uuid, $2, $3::uuid, $4)`,
			taskID, s.Title, s.AssigneeID, i,
		)
		if err != nil {
			return "", fmt.Errorf("insert shared_subtask: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit tx: %w", err)
	}
	return taskID, nil
}

func (r *repo) FindByUser(ctx context.Context, userID string) ([]domain.SharedTask, error) {
	return r.findTasksWithSubtasks(ctx, `
		SELECT st.id, st.proposer_id, st.addressee_id, st.title, st.description,
		       st.status, st.priority, st.due_date, st.created_at,
		       COALESCE(up.name, '') AS proposer_name,
		       COALESCE(ua.name, '') AS addressee_name,
		       COALESCE(st.admin_deleted, FALSE) AS admin_deleted
		FROM shared_tasks st
		LEFT JOIN user_subscriptions up ON up.user_id = st.proposer_id
		LEFT JOIN user_subscriptions ua ON ua.user_id = st.addressee_id
		WHERE (st.proposer_id=$1::uuid OR st.addressee_id=$1::uuid)
		  AND NOT (COALESCE(st.admin_deleted, FALSE) AND COALESCE(st.user_ack, FALSE))
		ORDER BY st.created_at DESC`, userID)
}

func (r *repo) FindAll(ctx context.Context) ([]domain.SharedTask, error) {
	return r.findTasksWithSubtasks(ctx, `
		SELECT st.id, st.proposer_id, st.addressee_id, st.title, st.description,
		       st.status, st.priority, st.due_date, st.created_at,
		       COALESCE(up.name, '') AS proposer_name,
		       COALESCE(ua.name, '') AS addressee_name,
		       COALESCE(st.admin_deleted, FALSE) AS admin_deleted
		FROM shared_tasks st
		LEFT JOIN user_subscriptions up ON up.user_id = st.proposer_id
		LEFT JOIN user_subscriptions ua ON ua.user_id = st.addressee_id
		ORDER BY st.created_at DESC`)
}

// findTasksWithSubtasks выполняет произвольный запрос задач и подгружает их подзадачи одним запросом.
func (r *repo) findTasksWithSubtasks(ctx context.Context, query string, args ...interface{}) ([]domain.SharedTask, error) {
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query shared tasks: %w", err)
	}
	defer rows.Close()

	var tasks []domain.SharedTask
	for rows.Next() {
		t, err := scanSharedTask(rows)
		if err != nil {
			r.logger.Errorf("findTasksWithSubtasks scan: %v", err)
			return nil, err
		}
		t.Subtasks = []domain.SharedSubtask{}
		tasks = append(tasks, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows err: %w", err)
	}
	if len(tasks) == 0 {
		return tasks, nil
	}

	// Подгружаем подзадачи одним запросом для всех задач
	ids := make([]string, len(tasks))
	for i, t := range tasks {
		ids[i] = t.ID
	}
	subRows, err := r.db.Query(ctx, `
		SELECT ss.id, ss.shared_task_id, ss.title, ss.assignee_id,
		       COALESCE(us.name, '') AS assignee_name, ss.is_done, ss.order_num
		FROM shared_subtasks ss
		LEFT JOIN user_subscriptions us ON us.user_id = ss.assignee_id
		WHERE ss.shared_task_id = ANY($1::uuid[])
		ORDER BY ss.order_num`, ids)
	if err != nil {
		return nil, fmt.Errorf("get subtasks: %w", err)
	}
	defer subRows.Close()

	idx := make(map[string]int, len(tasks))
	for i, t := range tasks {
		idx[t.ID] = i
	}
	for subRows.Next() {
		s, err := scanSharedSubtask(subRows)
		if err != nil {
			r.logger.Errorf("findTasksWithSubtasks scan subtask: %v", err)
			return nil, err
		}
		if i, ok := idx[s.SharedTaskID]; ok {
			tasks[i].Subtasks = append(tasks[i].Subtasks, s)
		}
	}
	return tasks, subRows.Err()
}

func (r *repo) FindByID(ctx context.Context, taskID, userID string) (*domain.SharedTask, error) {
	row := r.db.QueryRow(ctx, `
		SELECT st.id, st.proposer_id, st.addressee_id, st.title, st.description,
		       st.status, st.priority, st.due_date, st.created_at,
		       COALESCE(up.name, '') AS proposer_name,
		       COALESCE(ua.name, '') AS addressee_name,
		       COALESCE(st.admin_deleted, FALSE) AS admin_deleted
		FROM shared_tasks st
		LEFT JOIN user_subscriptions up ON up.user_id = st.proposer_id
		LEFT JOIN user_subscriptions ua ON ua.user_id = st.addressee_id
		WHERE st.id=$1::uuid
		  AND (st.proposer_id=$2::uuid OR st.addressee_id=$2::uuid)`,
		taskID, userID,
	)

	t, err := scanSharedTask(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get shared task detail: %w", err)
	}

	subRows, err := r.db.Query(ctx, `
		SELECT ss.id, ss.shared_task_id, ss.title, ss.assignee_id,
		       COALESCE(us.name, '') AS assignee_name, ss.is_done, ss.order_num
		FROM shared_subtasks ss
		LEFT JOIN user_subscriptions us ON us.user_id = ss.assignee_id
		WHERE ss.shared_task_id=$1::uuid
		ORDER BY ss.order_num`, taskID)
	if err != nil {
		return nil, fmt.Errorf("get subtasks detail: %w", err)
	}
	defer subRows.Close()

	t.Subtasks = []domain.SharedSubtask{}
	for subRows.Next() {
		s, err := scanSharedSubtask(subRows)
		if err != nil {
			return nil, fmt.Errorf("scan subtask detail: %w", err)
		}
		t.Subtasks = append(t.Subtasks, s)
	}
	return &t, subRows.Err()
}

func (r *repo) Respond(ctx context.Context, taskID, userID, status string) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE shared_tasks SET status=$2, updated_at=NOW()
		WHERE id=$1::uuid AND addressee_id=$3::uuid AND status='pending'`,
		taskID, status, userID)
	if err != nil {
		return fmt.Errorf("respond shared task: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("задача не найдена или уже обработана")
	}
	return nil
}

func (r *repo) Update(ctx context.Context, taskID, userID string, input port.UpdateInput) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var dueDateArg interface{}
	if input.DueDate != "" {
		dueDateArg = input.DueDate
	}
	tag, err := tx.Exec(ctx, `
		UPDATE shared_tasks
		SET title=$1, description=$2, priority=$3, due_date=$4, updated_at=NOW()
		WHERE id=$5::uuid AND (proposer_id=$6::uuid OR addressee_id=$6::uuid)`,
		input.Title, input.Description, input.Priority, dueDateArg, taskID, userID)
	if err != nil {
		return fmt.Errorf("update shared_task: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("задача не найдена или нет прав на редактирование")
	}

	// Если переданы подзадачи — перезаписываем
	if input.Subtasks != nil {
		if _, err = tx.Exec(ctx, `DELETE FROM shared_subtasks WHERE shared_task_id=$1::uuid`, taskID); err != nil {
			return fmt.Errorf("delete subtasks: %w", err)
		}
		for i, s := range *input.Subtasks {
			if s.Title == "" || s.AssigneeID == "" {
				continue
			}
			if _, err = tx.Exec(ctx, `
				INSERT INTO shared_subtasks (shared_task_id, title, assignee_id, order_num)
				VALUES ($1::uuid, $2, $3::uuid, $4)`,
				taskID, s.Title, s.AssigneeID, i,
			); err != nil {
				return fmt.Errorf("insert subtask: %w", err)
			}
		}
	}

	return tx.Commit(ctx)
}

func (r *repo) CounterPropose(ctx context.Context, taskID, addresseeID string, input port.UpdateInput) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var dueDateArg interface{}
	if input.DueDate != "" {
		dueDateArg = input.DueDate
	}
	tag, err := tx.Exec(ctx, `
		UPDATE shared_tasks
		SET title=$1, description=$2, priority=$3, due_date=$4, updated_at=NOW(),
		    proposer_id = addressee_id, addressee_id = proposer_id
		WHERE id=$5::uuid AND addressee_id=$6::uuid AND status='pending'`,
		input.Title, input.Description, input.Priority, dueDateArg, taskID, addresseeID,
	)
	if err != nil {
		return fmt.Errorf("counter propose: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("задача не найдена или уже обработана")
	}

	if input.Subtasks != nil {
		if _, err = tx.Exec(ctx, `DELETE FROM shared_subtasks WHERE shared_task_id=$1::uuid`, taskID); err != nil {
			return fmt.Errorf("delete subtasks: %w", err)
		}
		for i, s := range *input.Subtasks {
			if s.Title == "" || s.AssigneeID == "" {
				continue
			}
			if _, err = tx.Exec(ctx, `
				INSERT INTO shared_subtasks (shared_task_id, title, assignee_id, order_num)
				VALUES ($1::uuid, $2, $3::uuid, $4)`,
				taskID, s.Title, s.AssigneeID, i,
			); err != nil {
				return fmt.Errorf("insert subtask: %w", err)
			}
		}
	}

	return tx.Commit(ctx)
}

func (r *repo) Delete(ctx context.Context, taskID, userID string) (string, string, error) {
	var proposerID, addresseeID, title, status string
	err := r.db.QueryRow(ctx, `
		SELECT proposer_id, addressee_id, title, status FROM shared_tasks
		WHERE id=$1::uuid AND (proposer_id=$2::uuid OR addressee_id=$2::uuid)`,
		taskID, userID,
	).Scan(&proposerID, &addresseeID, &title, &status)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", fmt.Errorf("задача не найдена или нет доступа")
		}
		return "", "", fmt.Errorf("get shared task for delete: %w", err)
	}

	// Адресат не может удалить задачу в статусе ожидания — только принять или отклонить
	if status == "pending" && userID == addresseeID {
		return "", "", fmt.Errorf("нельзя удалить задачу, которую вы ещё не приняли — примите или отклоните её")
	}

	partnerID := proposerID
	if userID == proposerID {
		partnerID = addresseeID
	}

	tag, err := r.db.Exec(ctx, `
		DELETE FROM shared_tasks
		WHERE id=$1::uuid AND (proposer_id=$2::uuid OR addressee_id=$2::uuid)`,
		taskID, userID)
	if err != nil {
		return "", "", fmt.Errorf("delete shared_task: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return "", "", fmt.Errorf("задача не найдена или нет прав на удаление")
	}

	// Уведомляем партнёра только если задача была принята (не при отзыве pending-предложения)
	if status != "accepted" {
		return "", "", nil
	}
	return partnerID, title, nil
}

func (r *repo) ToggleSubtaskDone(ctx context.Context, subtaskID, assigneeID string) (domain.SharedSubtask, error) {
	var s domain.SharedSubtask
	// Разрешаем обоим участникам (proposer и addressee) отмечать любую подзадачу
	err := r.db.QueryRow(ctx, `
		UPDATE shared_subtasks ss SET is_done = NOT ss.is_done
		FROM shared_tasks st
		WHERE ss.id = $1::uuid
		  AND ss.shared_task_id = st.id
		  AND (st.proposer_id = $2::uuid OR st.addressee_id = $2::uuid)
		RETURNING ss.id, ss.shared_task_id, ss.title, ss.assignee_id, ss.is_done, ss.order_num`,
		subtaskID, assigneeID,
	).Scan(&s.ID, &s.SharedTaskID, &s.Title, &s.AssigneeID, &s.IsDone, &s.Order)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.SharedSubtask{}, fmt.Errorf("подзадача не найдена или нет доступа")
		}
		return domain.SharedSubtask{}, fmt.Errorf("toggle subtask: %w", err)
	}
	return s, nil
}

// --- scanners ---

type scanner interface {
	Scan(dest ...interface{}) error
}

func scanSharedTask(s scanner) (domain.SharedTask, error) {
	var t domain.SharedTask
	var dueDate sql.NullTime
	var status string
	err := s.Scan(
		&t.ID, &t.ProposerID, &t.AddresseeID, &t.Title, &t.Description,
		&status, &t.Priority, &dueDate, &t.CreatedAt,
		&t.ProposerName, &t.AddresseeName, &t.AdminDeleted,
	)
	if err != nil {
		return domain.SharedTask{}, err
	}
	t.Status = domain.SharedTaskStatus(status)
	if dueDate.Valid {
		t.DueDate = &dueDate.Time
	}
	return t, nil
}

func scanSharedSubtask(s scanner) (domain.SharedSubtask, error) {
	var sub domain.SharedSubtask
	err := s.Scan(
		&sub.ID, &sub.SharedTaskID, &sub.Title, &sub.AssigneeID,
		&sub.AssigneeName, &sub.IsDone, &sub.Order,
	)
	return sub, err
}
