package httputil

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
)

func Chain(h http.Handler, middleware ...func(http.Handler) http.Handler) http.Handler {
	for i := range middleware {
		h = middleware[len(middleware)-1-i](h)
	}
	return h
}

//
// LOGGING MIDDLEWARE
//

type statusBytesRecorder struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int
}

func (sr *statusBytesRecorder) WriteHeader(code int) {
	sr.statusCode = code
	sr.ResponseWriter.WriteHeader(code)
}

func (sr *statusBytesRecorder) Write(b []byte) (int, error) {
	n, err := sr.ResponseWriter.Write(b)
	sr.bytesWritten += n
	return n, err
}

func WithLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Create a custom ResponseWriter
		sr := &statusBytesRecorder{
			ResponseWriter: w,
			statusCode:     http.StatusOK, // Default to OK
			bytesWritten:   0,
		}

		// Call the next handler in the chain
		next.ServeHTTP(sr, r)

		var fn func(context.Context, string, ...any)

		if sr.statusCode < 400 {
			fn = slog.InfoContext
		} else {
			fn = slog.ErrorContext
		}

		fn(r.Context(), "http called", "method", r.Method, "path", r.URL.Path, "code", sr.statusCode, "size", sr.bytesWritten)
	})
}

//
// SESSION CONTEXT MIDDLEWARE
//

func WithSessionContext[T any](cookieKey CookieKey, ctxKey ContextKey, parseSession func(token string) (T, error)) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			var token string

			// Get token from authorization header
			bearer := r.Header.Get("Authorization")
			if len(bearer) > 7 && strings.ToUpper(bearer[0:6]) == "BEARER" {
				token = bearer[7:]
			}

			if token == "" {
				// try to get token from cookie
				token, _ = GetCookie(cookieKey, r)
			}

			if token == "" {
				next.ServeHTTP(w, r)
				return
			}

			session, err := parseSession(token)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}

			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey, session)))
		}

		return http.HandlerFunc(fn)
	}
}
