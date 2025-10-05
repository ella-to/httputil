package httputil

import (
	"bytes"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestChain(t *testing.T) {
	// Create a base handler that writes "base"
	baseHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("base"))
	})

	// Create middleware that adds prefixes
	middleware1 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("1-"))
			next.ServeHTTP(w, r)
		})
	}

	middleware2 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("2-"))
			next.ServeHTTP(w, r)
		})
	}

	middleware3 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("3-"))
			next.ServeHTTP(w, r)
		})
	}

	tests := []struct {
		name        string
		middlewares []func(http.Handler) http.Handler
		expected    string
	}{
		{
			name:        "no middleware",
			middlewares: nil,
			expected:    "base",
		},
		{
			name:        "single middleware",
			middlewares: []func(http.Handler) http.Handler{middleware1},
			expected:    "1-base",
		},
		{
			name:        "multiple middlewares",
			middlewares: []func(http.Handler) http.Handler{middleware1, middleware2, middleware3},
			expected:    "1-2-3-base",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := Chain(baseHandler, tt.middlewares...)

			req := httptest.NewRequest("GET", "/", nil)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			if w.Body.String() != tt.expected {
				t.Errorf("Chain() output = %v, want %v", w.Body.String(), tt.expected)
			}
		})
	}
}

func TestWithLogging(t *testing.T) {
	// Capture slog output
	var logOutput bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logOutput, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	slog.SetDefault(logger)

	tests := []struct {
		name           string
		handlerFunc    http.HandlerFunc
		method         string
		path           string
		expectedStatus int
		expectLogLevel string
	}{
		{
			name: "successful request",
			handlerFunc: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("success"))
			},
			method:         "GET",
			path:           "/api/users",
			expectedStatus: 200,
			expectLogLevel: "INFO",
		},
		{
			name: "client error",
			handlerFunc: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte("bad request"))
			},
			method:         "POST",
			path:           "/api/create",
			expectedStatus: 400,
			expectLogLevel: "ERROR",
		},
		{
			name: "server error",
			handlerFunc: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte("server error"))
			},
			method:         "DELETE",
			path:           "/api/delete",
			expectedStatus: 500,
			expectLogLevel: "ERROR",
		},
		{
			name: "default status (200)",
			handlerFunc: func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte("default"))
			},
			method:         "PUT",
			path:           "/api/update",
			expectedStatus: 200,
			expectLogLevel: "INFO",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear log output
			logOutput.Reset()

			handler := WithLogging(tt.handlerFunc)

			req := httptest.NewRequest(tt.method, tt.path, nil)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			// Check status code
			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			// Check log output
			logStr := logOutput.String()
			if !strings.Contains(logStr, tt.expectLogLevel) {
				t.Errorf("Expected log level %s in output: %s", tt.expectLogLevel, logStr)
			}

			if !strings.Contains(logStr, tt.method) {
				t.Errorf("Expected method %s in log output: %s", tt.method, logStr)
			}

			if !strings.Contains(logStr, tt.path) {
				t.Errorf("Expected path %s in log output: %s", tt.path, logStr)
			}

			if !strings.Contains(logStr, "http called") {
				t.Errorf("Expected 'http called' message in log output: %s", logStr)
			}
		})
	}
}

func TestWithLoggingResponseSize(t *testing.T) {
	var logOutput bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logOutput, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	slog.SetDefault(logger)

	responseData := "this is a test response with some content"
	handler := WithLogging(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(responseData))
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Check that response size is logged
	logStr := logOutput.String()
	expectedSize := len(responseData)

	if !strings.Contains(logStr, "size") {
		t.Errorf("Expected 'size' field in log output: %s", logStr)
	}

	// Response should contain the expected content
	if w.Body.String() != responseData {
		t.Errorf("Expected response body %q, got %q", responseData, w.Body.String())
	}

	if w.Body.Len() != expectedSize {
		t.Errorf("Expected response size %d, got %d", expectedSize, w.Body.Len())
	}
}

func TestWithSessionContext(t *testing.T) {
	cookieKey := CookieKey("session")
	ctxKey := ContextKey("user")
	ctxTokenKey := ContextKey("token")

	// Mock session parser
	parseSession := func(token string) (string, error) {
		if token == "valid-token" {
			return "user-123", nil
		}
		if token == "admin-token" {
			return "admin-456", nil
		}
		return "", errors.New("invalid token")
	}

	// Test handler that checks context
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if user, ok := r.Context().Value(ctxKey).(string); ok {
			w.Write([]byte("user:" + user))
		} else {
			w.Write([]byte("no-user"))
		}

		if token, ok := r.Context().Value(ctxTokenKey).(string); ok {
			w.Write([]byte(",token:" + token))
		}
	})

	middleware := WithSessionContext(cookieKey, ctxKey, ctxTokenKey, parseSession)
	handler := middleware(testHandler)

	tests := []struct {
		name             string
		authHeader       string
		cookieValue      string
		expectedResponse string
	}{
		{
			name:             "no token",
			authHeader:       "",
			cookieValue:      "",
			expectedResponse: "no-user",
		},
		{
			name:             "valid bearer token",
			authHeader:       "Bearer valid-token",
			cookieValue:      "",
			expectedResponse: "user:user-123,token:valid-token",
		},
		{
			name:             "invalid bearer token",
			authHeader:       "Bearer invalid-token",
			cookieValue:      "",
			expectedResponse: "no-user",
		},
		{
			name:             "valid cookie token",
			authHeader:       "",
			cookieValue:      "admin-token",
			expectedResponse: "user:admin-456,token:admin-token",
		},
		{
			name:             "bearer takes precedence over cookie",
			authHeader:       "Bearer valid-token",
			cookieValue:      "admin-token",
			expectedResponse: "user:user-123,token:valid-token",
		},
		{
			name:             "malformed bearer header",
			authHeader:       "Bearer",
			cookieValue:      "valid-token",
			expectedResponse: "user:user-123,token:valid-token",
		},
		{
			name:             "wrong auth type",
			authHeader:       "Basic dGVzdA==",
			cookieValue:      "valid-token",
			expectedResponse: "user:user-123,token:valid-token",
		},
		{
			name:             "lowercase bearer",
			authHeader:       "bearer valid-token",
			cookieValue:      "",
			expectedResponse: "user:user-123,token:valid-token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)

			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}

			if tt.cookieValue != "" {
				cookie := &http.Cookie{
					Name:  string(cookieKey),
					Value: tt.cookieValue,
				}
				req.AddCookie(cookie)
			}

			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			if w.Body.String() != tt.expectedResponse {
				t.Errorf("Expected response %q, got %q", tt.expectedResponse, w.Body.String())
			}
		})
	}
}

func TestWithSessionContextWithoutTokenKey(t *testing.T) {
	cookieKey := CookieKey("session")
	ctxKey := ContextKey("user")
	ctxTokenKey := ContextKey("") // Empty token key

	parseSession := func(token string) (string, error) {
		if token == "valid-token" {
			return "user-123", nil
		}
		return "", nil
	}

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if user, ok := r.Context().Value(ctxKey).(string); ok {
			w.Write([]byte("user:" + user))
		} else {
			w.Write([]byte("no-user"))
		}

		// Token should not be in context
		if token, ok := r.Context().Value(ctxTokenKey).(string); ok {
			w.Write([]byte(",token:" + token))
		} else {
			w.Write([]byte(",no-token"))
		}
	})

	middleware := WithSessionContext(cookieKey, ctxKey, ctxTokenKey, parseSession)
	handler := middleware(testHandler)

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer valid-token")

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	expected := "user:user-123,no-token"
	if w.Body.String() != expected {
		t.Errorf("Expected response %q, got %q", expected, w.Body.String())
	}
}

func TestWithSessionContextParseError(t *testing.T) {
	cookieKey := CookieKey("session")
	ctxKey := ContextKey("user")
	ctxTokenKey := ContextKey("token")

	// Parser that always returns an error
	parseSession := func(token string) (string, error) {
		return "", errors.New("parse error")
	}

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if user, ok := r.Context().Value(ctxKey).(string); ok {
			w.Write([]byte("user:" + user))
		} else {
			w.Write([]byte("no-user"))
		}
	})

	middleware := WithSessionContext(cookieKey, ctxKey, ctxTokenKey, parseSession)
	handler := middleware(testHandler)

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer any-token")

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	// Should proceed without session when parse fails
	if w.Body.String() != "no-user" {
		t.Errorf("Expected 'no-user', got %q", w.Body.String())
	}
}

func TestCompleteMiddlewareStack(t *testing.T) {
	// Test a complete middleware stack with logging and session
	var logOutput bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logOutput, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	slog.SetDefault(logger)

	cookieKey := CookieKey("auth")
	ctxKey := ContextKey("user")
	ctxTokenKey := ContextKey("token")

	parseSession := func(token string) (string, error) {
		if token == "user-token" {
			return "john-doe", nil
		}
		return "", nil
	}

	// Final handler
	finalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if user, ok := r.Context().Value(ctxKey).(string); ok {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Hello, " + user))
		} else {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte("Unauthorized"))
		}
	})

	// Build middleware stack
	sessionMiddleware := WithSessionContext(cookieKey, ctxKey, ctxTokenKey, parseSession)

	handler := Chain(finalHandler, sessionMiddleware, WithLogging)

	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer user-token")

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	// Check response
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	if w.Body.String() != "Hello, john-doe" {
		t.Errorf("Expected 'Hello, john-doe', got %q", w.Body.String())
	}

	// Check that logging occurred
	logStr := logOutput.String()
	if !strings.Contains(logStr, "http called") {
		t.Error("Expected logging to occur")
	}

	if !strings.Contains(logStr, "INFO") {
		t.Error("Expected INFO level log")
	}
}
