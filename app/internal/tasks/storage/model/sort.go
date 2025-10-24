package model

import (
	"TODOLIST_Tasks/app/internal/tasks/storage/postgres"
	"fmt"
)

type sortOptions struct {
	Fields, Order string
}

func NewSortOptions(fields, order string) postgres.SortOptions {
	return sortOptions{
		Fields: fields,
		Order:  order,
	}
}

func (so sortOptions) GetOrderBy() string {
	return fmt.Sprintf("%s %s", so.Fields, so.Order)
}
