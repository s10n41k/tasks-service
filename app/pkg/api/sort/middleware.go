package sort

import (
	"context"
	"net/http"
	"strings"
)

const (
	ASC               = "ASC"
	DESC              = "DESC"
	OptionsContextKey = "sort_options"
)

func MiddleWare(h http.HandlerFunc, defaultSortFields, defaultSortOrder string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sortBy := r.URL.Query().Get("sort_by")
		sortOrder := r.URL.Query().Get("sort_order")

		if sortBy == "" {
			sortBy = defaultSortFields
		}
		if sortOrder == "" {
			sortOrder = defaultSortOrder
		} else {
			sortOrder = strings.ToUpper(sortOrder)
			if sortOrder != ASC && sortOrder != DESC {
				http.Error(w, "bad sort order", http.StatusBadRequest)
				return
			}
		}

		options := Options{
			Fields: sortBy,
			Order:  sortOrder,
		}
		ctx := context.WithValue(r.Context(), OptionsContextKey, options)
		r = r.WithContext(ctx)
		h(w, r)
	}
}

type Options struct {
	Fields, Order string
}
