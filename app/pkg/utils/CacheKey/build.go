package CacheKey

import (
	"fmt"
	"sort"
	"strings"

	"TODOLIST_Tasks/app/pkg/api/filter"
	sort2 "TODOLIST_Tasks/app/pkg/api/sort"
)

func BuildCacheKey(userId string, filterOptions filter.Option, sortOptions sort2.Options) string {
	var parts []string

	// Базовая часть
	parts = append(parts, fmt.Sprintf("tasks:user:%s", userId))

	// Добавляем фильтры
	if len(filterOptions.Fields) > 0 {
		// Чтобы порядок фильтров не влиял на ключ (стабильность!)
		sortedFields := make([]filter.Field, len(filterOptions.Fields))
		copy(sortedFields, filterOptions.Fields)

		sort.Slice(sortedFields, func(i, j int) bool {
			return sortedFields[i].Name < sortedFields[j].Name
		})

		for _, field := range sortedFields {
			parts = append(parts, fmt.Sprintf("%s:%s:%s", field.Name, field.Operator, field.Value))
		}
	}
	// Добавляем сортировку
	if sortOptions.Fields != "" && sortOptions.Order != "" {
		parts = append(parts, fmt.Sprintf("sort:%s:%s", sortOptions.Fields, strings.ToUpper(sortOptions.Order)))
	}

	// Добавляем лимит, если есть
	if filterOptions.Limit > 0 {
		parts = append(parts, fmt.Sprintf("limit:%d", filterOptions.Limit))
	}

	// Соединяем через точку с запятой
	return strings.Join(parts, ";")
}

func BuildCacheKeyWithTag(userId, tagId string, filterOptions filter.Option, sortOptions sort2.Options) string {
	var parts []string

	// Базовая часть: userId и tagId
	parts = append(parts, fmt.Sprintf("tasks:user:%s:tag:%s", userId, tagId))

	// Добавляем фильтры
	if len(filterOptions.Fields) > 0 {
		// Чтобы порядок фильтров был стабильным
		sortedFields := make([]filter.Field, len(filterOptions.Fields))
		copy(sortedFields, filterOptions.Fields)

		sort.Slice(sortedFields, func(i, j int) bool {
			return sortedFields[i].Name < sortedFields[j].Name
		})

		for _, field := range sortedFields {
			parts = append(parts, fmt.Sprintf("%s:%s:%s", field.Name, field.Operator, field.Value))
		}
	}

	// Добавляем сортировку
	if sortOptions.Fields != "" && sortOptions.Order != "" {
		parts = append(parts, fmt.Sprintf("sort:%s:%s", sortOptions.Fields, strings.ToUpper(sortOptions.Order)))
	}

	// Добавляем лимит, если есть
	if filterOptions.Limit > 0 {
		parts = append(parts, fmt.Sprintf("limit:%d", filterOptions.Limit))
	}

	// Склеиваем через точку с запятой
	return strings.Join(parts, ";")
}
