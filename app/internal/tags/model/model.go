package model

type Tags struct {
	Id     string  `json:"id"`
	Name   string  `json:"name"`
	UserID *string `json:"user_id,omitempty"`
}
