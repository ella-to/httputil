package httputil

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReadLimiter(t *testing.T) {
	tests := []struct {
		name        string
		requestBody string
		sizeLimit   int64
		expectError bool
	}{
		{
			name:        "small body within limit",
			requestBody: "small body",
			sizeLimit:   100,
			expectError: false,
		},
		{
			name:        "empty body",
			requestBody: "",
			sizeLimit:   10,
			expectError: false,
		},
		{
			name:        "body at exact limit",
			requestBody: "exactly10c", // 10 characters
			sizeLimit:   10,
			expectError: false,
		},
		{
			name:        "body exceeds limit",
			requestBody: "this body is too long for the limit",
			sizeLimit:   5,
			expectError: true,
		},
		{
			name:        "zero limit",
			requestBody: "any content",
			sizeLimit:   0,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/", strings.NewReader(tt.requestBody))
			w := httptest.NewRecorder()

			// Apply the limiter
			ReadLimter(tt.sizeLimit, w, req)

			// Try to read the body
			body, err := io.ReadAll(req.Body)

			if tt.expectError && err == nil {
				t.Error("Expected error due to size limit, but got none")
			}

			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			if !tt.expectError {
				if string(body) != tt.requestBody {
					t.Errorf("Expected body %q, got %q", tt.requestBody, string(body))
				}
			}
		})
	}
}

func TestReadBodyLimiter(t *testing.T) {
	tests := []struct {
		name           string
		requestBody    string
		sizeLimit      int64
		expectError    bool
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "small body success",
			requestBody:    "hello world",
			sizeLimit:      100,
			expectError:    false,
			expectedStatus: 200, // Default status when not set
			expectedBody:   "hello world",
		},
		{
			name:           "empty body success",
			requestBody:    "",
			sizeLimit:      10,
			expectError:    false,
			expectedStatus: 200,
			expectedBody:   "",
		},
		{
			name:           "body at exact limit",
			requestBody:    "1234567890", // 10 bytes
			sizeLimit:      10,
			expectError:    false,
			expectedStatus: 200,
			expectedBody:   "1234567890",
		},
		{
			name:           "body exceeds limit",
			requestBody:    "this is too long",
			sizeLimit:      5,
			expectError:    true,
			expectedStatus: 400,
			expectedBody:   "",
		},
		{
			name:           "zero limit with content",
			requestBody:    "x",
			sizeLimit:      0,
			expectError:    true,
			expectedStatus: 400,
			expectedBody:   "",
		},
		{
			name:           "large body exceeds limit",
			requestBody:    strings.Repeat("a", 1000),
			sizeLimit:      500,
			expectError:    true,
			expectedStatus: 400,
			expectedBody:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/", strings.NewReader(tt.requestBody))
			w := httptest.NewRecorder()

			body, err := ReadBodyLimiter(tt.sizeLimit, w, req)

			// Check error expectation
			if tt.expectError && err == nil {
				t.Error("Expected error due to size limit, but got none")
			}

			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			// Check response status
			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			// Check returned body
			if !tt.expectError {
				if string(body) != tt.expectedBody {
					t.Errorf("Expected body %q, got %q", tt.expectedBody, string(body))
				}
			} else {
				// On error, body should be nil or empty
				if body != nil {
					t.Errorf("Expected nil body on error, got %q", string(body))
				}
			}
		})
	}
}

func TestReadBodyLimiterHTTPHandler(t *testing.T) {
	// Test ReadBodyLimiter in a realistic HTTP handler scenario
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		const maxSize = 100 // 100 bytes limit

		body, err := ReadBodyLimiter(maxSize, w, r)
		if err != nil {
			// ReadBodyLimiter already set the status to 400
			w.Write([]byte("Request too large"))
			return
		}

		// Process the body normally
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Received: " + string(body)))
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	tests := []struct {
		name           string
		requestBody    string
		expectedStatus int
		expectedInBody string
	}{
		{
			name:           "normal request",
			requestBody:    "normal request body",
			expectedStatus: 200,
			expectedInBody: "Received: normal request body",
		},
		{
			name:           "empty request",
			requestBody:    "",
			expectedStatus: 200,
			expectedInBody: "Received: ",
		},
		{
			name:           "request at limit",
			requestBody:    strings.Repeat("x", 100),
			expectedStatus: 200,
			expectedInBody: "Received: " + strings.Repeat("x", 100),
		},
		{
			name:           "request exceeds limit",
			requestBody:    strings.Repeat("x", 150),
			expectedStatus: 400,
			expectedInBody: "Request too large",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := http.Post(server.URL, "text/plain", strings.NewReader(tt.requestBody))
			if err != nil {
				t.Fatalf("Failed to make request: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, resp.StatusCode)
			}

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("Failed to read response body: %v", err)
			}

			if string(body) != tt.expectedInBody {
				t.Errorf("Expected response body %q, got %q", tt.expectedInBody, string(body))
			}
		})
	}
}

func TestReadLimiterWithDifferentHTTPMethods(t *testing.T) {
	methods := []string{"POST", "PUT", "PATCH"}

	for _, method := range methods {
		t.Run("method_"+method, func(t *testing.T) {
			requestBody := "test body for " + method
			req := httptest.NewRequest(method, "/", strings.NewReader(requestBody))
			w := httptest.NewRecorder()

			ReadLimter(100, w, req)

			body, err := io.ReadAll(req.Body)
			if err != nil {
				t.Errorf("Unexpected error for %s method: %v", method, err)
			}

			if string(body) != requestBody {
				t.Errorf("Body mismatch for %s method: expected %q, got %q", 
					method, requestBody, string(body))
			}
		})
	}
}

func TestReadLimiterMultipleReads(t *testing.T) {
	// Test that the limiter works correctly when the body is read multiple times
	requestBody := "test content"
	req := httptest.NewRequest("POST", "/", strings.NewReader(requestBody))
	w := httptest.NewRecorder()

	ReadLimter(100, w, req)

	// First read
	body1, err1 := io.ReadAll(req.Body)
	if err1 != nil {
		t.Fatalf("First read failed: %v", err1)
	}

	// Second read should return empty (body is consumed)
	body2, err2 := io.ReadAll(req.Body)
	if err2 != nil {
		t.Fatalf("Second read failed: %v", err2)
	}

	if string(body1) != requestBody {
		t.Errorf("First read: expected %q, got %q", requestBody, string(body1))
	}

	if len(body2) != 0 {
		t.Errorf("Second read: expected empty body, got %q", string(body2))
	}
}

func TestReadBodyLimiterWithJSONPayload(t *testing.T) {
	// Test with a realistic JSON payload
	jsonPayload := `{
		"name": "John Doe",
		"email": "john@example.com",
		"age": 30,
		"preferences": {
			"theme": "dark",
			"notifications": true
		}
	}`

	req := httptest.NewRequest("POST", "/api/users", strings.NewReader(jsonPayload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// Allow for reasonable JSON size
	body, err := ReadBodyLimiter(1024, w, req)
	if err != nil {
		t.Fatalf("Failed to read JSON payload: %v", err)
	}

	if string(body) != jsonPayload {
		t.Errorf("JSON payload mismatch")
	}

	if w.Code != 200 { // Should remain default status
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestReadBodyLimiterErrorPropagation(t *testing.T) {
	// Test that errors from the underlying reader are properly propagated
	requestBody := strings.Repeat("x", 200) // 200 bytes
	req := httptest.NewRequest("POST", "/", strings.NewReader(requestBody))
	w := httptest.NewRecorder()

	// Set a limit smaller than the body
	body, err := ReadBodyLimiter(50, w, req)

	// Should get an error
	if err == nil {
		t.Error("Expected error when body exceeds limit")
	}

	// Body should be nil on error
	if body != nil {
		t.Error("Expected nil body on error")
	}

	// Status should be set to 400
	if w.Code != 400 {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

func TestReadLimiterTypoInFunctionName(t *testing.T) {
	// Note: The function is named "ReadLimter" instead of "ReadLimiter" 
	// This test ensures the typo doesn't affect functionality
	req := httptest.NewRequest("POST", "/", strings.NewReader("test"))
	w := httptest.NewRecorder()

	// Should work despite the typo in the function name
	ReadLimter(10, w, req)

	body, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("ReadLimter (typo) failed: %v", err)
	}

	if string(body) != "test" {
		t.Errorf("Expected 'test', got %q", string(body))
	}
}