package filter

import (
	"context"
	"net/http"
	"strings"
)

type contextKey string

const (
	OptionsContextKey contextKey = "filter_options"
)

func Middleware(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()

		var filters []Field

		// Обрабатываем каждый ключ в запросе
		for key, values := range query {
			for _, value := range values {
				// Разделяем на оператор и значение
				parts := strings.SplitN(value, ":", 2)
				if len(parts) != 2 {
					continue
				}

				operator := parts[0]
				val := parts[1]

				// Создаем экземпляр RequestFilter
				filterData := Field{
					Name:     key,      // поле — это имя ключа
					Operator: operator, // оператор
					Value:    val,      // значение
				}

				// Добавляем в список фильтров
				filters = append(filters, filterData)
			}
		}

		// Сохраняем фильтры в контексте
		ctx := context.WithValue(r.Context(), OptionsContextKey, filters)
		r = r.WithContext(ctx)
		h(w, r)
	}
}

type Field struct {
	Name     string
	Value    string
	Operator string
}

type Option struct {
	Fields []Field
	Limit  int
}
