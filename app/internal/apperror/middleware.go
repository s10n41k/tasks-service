package apperror

import (
	"errors"
	"net/http"
)

type appHandler func(http.ResponseWriter, *http.Request) error

func Middleware(handler appHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var appErr *AppError

		err := handler(w, r)
		if err != nil {
			if errors.As(err, &appErr) {
				if errors.Is(err, ErrNotFound) {
					w.WriteHeader(http.StatusNotFound)
					w.Write(ErrNotFound.Marshal())
					return
				}
				w.WriteHeader(http.StatusBadRequest)
				w.Write(appErr.Marshal())
				return
			}
			w.WriteHeader(http.StatusInternalServerError)
			w.Write(systemError(err).Marshal())

		}
	}
}
