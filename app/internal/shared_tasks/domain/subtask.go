package domain

// SharedSubtask — Entity внутри агрегата SharedTask.
// В отличие от обычной Subtask (tasks domain), имеет assignee_id:
// только назначенный участник может отметить её выполненной.
// Это фундаментально другой инвариант — причина существования дублирования.
type SharedSubtask struct {
	ID           string
	SharedTaskID string
	Title        string
	AssigneeID   string
	AssigneeName string
	IsDone       bool
	Order        int
}
