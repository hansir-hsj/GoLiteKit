package middleware

import "net/http"

// {{.Name}}Middleware is a custom HTTP middleware.
func {{.Name}}Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// TODO: add middleware logic here
		next.ServeHTTP(w, r)
	})
}
