package domain

// Subtask — Entity внутри агрегата Task.
// Не является самостоятельным доменом: не существует без родительского Task.
// В отличие от SharedSubtask, не имеет assignee — выполняется владельцем задачи.
type Subtask struct {
	ID     string
	TaskID string
	Title  string
	IsDone bool
	Order  int
}
