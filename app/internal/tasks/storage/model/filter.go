package model

import (
	"TODOLIST_Tasks/app/internal/tasks/storage/postgres"
	"TODOLIST_Tasks/app/pkg/api/filter"
	"TODOLIST_Tasks/app/pkg/utils/operator"
	"fmt"
	"strings"
	"time"
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

	return &storageOptions
}

func (o *StorageOptions) CreateQuery() string {
	var queryParts []string

	for _, field := range o.Fields {
		sqlCondition, err := o.createSQLCondition(field)
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		queryParts = append(queryParts, sqlCondition)
	}

	return strings.Join(queryParts, " AND ")
}

func (o *StorageOptions) createSQLCondition(field StorageField) (string, error) {
	sqlOperator, err := operator.GetSQLOperator(field.Operator)
	if err != nil {
		return "", err
	}

	// Обрабатываем специальные операторы
	switch sqlOperator {
	case "ILIKE":
		return o.createILIKECondition(field.Name, field.Value), nil
	case "BETWEEN":
		return o.createBETWEENCondition(field.Name, field.Value), nil
	default:
		formattedValue := o.formatValue(field.Value, sqlOperator)
		return fmt.Sprintf("%s %s %s", field.Name, sqlOperator, formattedValue), nil
	}
}

func (o *StorageOptions) createILIKECondition(fieldName, value string) string {
	// Для ILIKE создаем несколько вариантов поиска
	conditions := []string{
		fmt.Sprintf("%s ILIKE '%s'", fieldName, value),     // Точное совпадение
		fmt.Sprintf("%s ILIKE '%%%s'", fieldName, value),   // Заканчивается на
		fmt.Sprintf("%s ILIKE '%s%%'", fieldName, value),   // Начинается с
		fmt.Sprintf("%s ILIKE '%%%s%%'", fieldName, value), // Содержит
	}
	return fmt.Sprintf("(%s)", strings.Join(conditions, " OR "))
}

func (o *StorageOptions) createBETWEENCondition(fieldName, value string) string {
	// Ожидаем формат "start:end" для BETWEEN
	parts := strings.Split(value, ":")
	if len(parts) == 2 {
		start := o.formatSimpleValue(parts[0])
		end := o.formatSimpleValue(parts[1])
		return fmt.Sprintf("%s BETWEEN %s AND %s", fieldName, start, end)
	}
	// Fallback - если формат неправильный
	return fmt.Sprintf("%s = %s", fieldName, o.formatSimpleValue(value))
}

func (o *StorageOptions) formatValue(value, operator string) string {
	// Для числовых операторов не добавляем кавычки
	if o.isNumericOperator(operator) {
		return value
	}
	// Для строковых операторов добавляем кавычки
	return o.formatSimpleValue(value)
}

func (o *StorageOptions) isNumericOperator(operator string) bool {
	numericOperators := []string{"=", ">", "<", ">=", "<=", "!="}
	for _, op := range numericOperators {
		if operator == op {
			return true
		}
	}
	return false
}

func (o *StorageOptions) formatSimpleValue(value string) string {
	// Пытаемся распарсить как дату
	if formatted, ok := o.tryParseDate(value); ok {
		return formatted
	}
	// Для обычных строк - добавляем кавычки
	return fmt.Sprintf("'%s'", value)
}

func (o *StorageOptions) tryParseDate(value string) (string, bool) {
	// Пробуем разные форматы дат
	formats := []string{
		"2006-01-02-15-04", // ГГГГ-ММ-ДД-ЧЧ-ММ
		"2006-01-02",       // ГГГГ-ММ-ДД
		time.RFC3339,       // Стандартный формат
	}

	for _, format := range formats {
		if parsed, err := time.Parse(format, value); err == nil {
			// Возвращаем в SQL формате
			return fmt.Sprintf("'%s'", parsed.Format("2006-01-02 15:04:05")), true
		}
	}

	return "", false
}
