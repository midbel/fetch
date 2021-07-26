package fetch

import (
	"net/http"
)

type Handler func(r *http.Request) (interface{}, error)

func Wrap(h Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		data, err := h(r)
		if err != nil {
			code := http.StatusBadRequest
			if err, ok := err.(Error); ok {
				code = err.Code
			}
			w.WriteHeader(code)
			return
		}
		if data == nil {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		code := http.StatusOK
		if r.Method == http.MethodPost {
			code = http.StatusCreated
		}
		w.WriteHeader(code)
	}
	return http.HandlerFunc(fn)
}
