package model

import (
	"encoding/json"
	"fmt"
	"time"
)

type TaskCreateDTO struct {
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Priory      string    `json:"priory"`
	Status      string    `json:"status"`
	Completed   string    `json:"completed"`
	DueDate     time.Time `json:"due_date"`
	UserId      string    `json:"user_id"`
	TagID       string    `json:"tag_id"`
	TagName     string    `json:"tag_name"`
}

type TaskUpdateDTO struct {
	Title       *string     `json:"title"`
	Description *string     `json:"description"`
	Status      *string     `json:"status"`
	Priory      *string     `json:"priory"`
	DueDate     *CustomTime `json:"due_date"`
	TagName     *string     `json:"tag_name"`
}

type CustomTime time.Time

func (ct *CustomTime) UnmarshalJSON(b []byte) error {
	s := string(b)
	s = s[1 : len(s)-1] // Убираем кавычки

	t, err := time.Parse("2006-01-02 15:04", s)
	if err != nil {
		return fmt.Errorf("failed to parse %q: %w", s, err)
	}

	*ct = CustomTime(t)
	return nil
}

type TaskResponse struct {
	Id          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Priory      string `json:"priory"`
	Status      string `json:"status"`
	DueDate     string `json:"due_date"`
	CreatedAt   string `json:"created_at"`
	UserId      string `json:"user_id"`
	TagsName    string `json:"tags_name"`
}

func (t *TaskCreateDTO) UnmarshalJSON(data []byte) error {
	type Alias TaskCreateDTO
	aux := &struct {
		DueDate string `json:"due_date"`
		*Alias
	}{
		Alias: (*Alias)(t),
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	// Parse the date
	var err error
	t.DueDate, err = time.Parse("2006-01-02 15:04", aux.DueDate)
	if err != nil {
		return fmt.Errorf("invalid due_date format: %v", err)
	}

	return nil
}
