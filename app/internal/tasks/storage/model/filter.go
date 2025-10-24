package model

import (
	"TODOLIST_Tasks/app/internal/tasks/storage/postgres"
	"TODOLIST_Tasks/app/pkg/api/filter"
	"TODOLIST_Tasks/app/pkg/utils/operator"
	"fmt"
	"strings"
)

type StorageOptions struct {
	Fields []StorageField
}

type StorageField struct {
	Name     string
	Operator string
	Value    string
}

func NewFilterOptions(option filter.Option) postgres.FilterOptions {
	storageOptions := StorageOptions{
		Fields: make([]StorageField, len(option.Fields)),
	}

	for i, field := range option.Fields {
		storageOptions.Fields[i] = StorageField{
			Name:     field.Name,
			Operator: field.Operator,
			Value:    field.Value,
		}
	}

	return storageOptions
}

func (o StorageOptions) CreateQuery() string {
	var queryParts []string

	for _, field := range o.Fields {
		op, err := operator.GetSQLOperator(field.Operator)
		if err != nil {
			return fmt.Sprintf("%v", err)
		}

		field.Operator = op
		field.Value = parseAndConvert(field.Value)

		if field.Operator == "ILIKE" {
			// Генерируем условия для ILIKE
			ilikeConditions := []string{
				fmt.Sprintf("%s ILIKE %v", field.Name, field.Value),                 // Полное совпадение
				fmt.Sprintf("%s ILIKE '%%' || %v || '%%'", field.Name, field.Value), // Содержит
			}
			// Добавляем все ILIKE условия к queryParts, оборачивая в скобки
			queryParts = append(queryParts, fmt.Sprintf("(%s)", strings.Join(ilikeConditions, " OR ")))
		} else {
			// Для остальных операторов
			queryParts = append(queryParts, fmt.Sprintf("%s %s %v", field.Name, field.Operator, field.Value))
		}
	}

	// Формируем итоговое условие с использованием AND
	return strings.Join(queryParts, " AND ")
}
func parseAndConvert(input string) string {
	parts := strings.Split(input, ":")

	if len(parts) == 1 { // Одна дата
		if strings.Count(parts[0], "-") == 4 { // Формат "ГГГГ-ММ-ДД-ЧЧ-ММ"
			date := parts[0]
			return "'" + date[:10] + " " + strings.Replace(date[11:], "-", ":", 1) + "'" // Формат: "'ГГГГ-ММ-ДД ЧЧ:ММ'"
		} else if strings.Count(parts[0], "-") == 3 { // Формат "ГГГГ-ММ-ДД"
			return "'" + parts[0] + "'" // Возвращаем как есть с кавычками
		}
	} else if len(parts) == 2 { // Две даты
		startDate := parts[0]
		endDate := parts[1]

		startDateTime := "'" + startDate[:10] + " " + strings.Replace(startDate[11:], "-", ":", 1) + "'" // "'ГГГГ-ММ-ДД ЧЧ:ММ'"
		endDateTime := "'" + endDate[:10] + " " + strings.Replace(endDate[11:], "-", ":", 1) + "'"       // "'ГГГГ-ММ-ДД ЧЧ:ММ'"
		return fmt.Sprintf("%s AND %s", startDateTime, endDateTime)
	}
	return fmt.Sprintf("'%s'", input) // Возвращаем как есть, если формат не соответствует
}
