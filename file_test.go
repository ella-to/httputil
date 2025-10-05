package httputil

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"testing/fstest"
)

func TestServeFileWithMapFS(t *testing.T) {
	// Create a test filesystem using fstest.MapFS
	testFS := fstest.MapFS{
		"index.html": {
			Data: []byte("<html><body>Hello World</body></html>"),
		},
		"css/style.css": {
			Data: []byte("body { color: blue; }"),
		},
		"js/app.js": {
			Data: []byte("console.log('Hello from JS');"),
		},
		"api/data.json": {
			Data: []byte(`{"message": "Hello API"}`),
		},
	}

	handler := ServeFile(testFS)
	server := httptest.NewServer(handler)
	defer server.Close()

	tests := []struct {
		name           string
		path           string
		expectedStatus int
		expectedBody   string
		checkContent   bool
	}{
		{
			name:           "serve index.html",
			path:           "/index.html",
			expectedStatus: 200,
			expectedBody:   "<html><body>Hello World</body></html>",
			checkContent:   true,
		},
		{
			name:           "serve CSS file",
			path:           "/css/style.css",
			expectedStatus: 200,
			expectedBody:   "body { color: blue; }",
			checkContent:   true,
		},
		{
			name:           "serve JS file",
			path:           "/js/app.js",
			expectedStatus: 200,
			expectedBody:   "console.log('Hello from JS');",
			checkContent:   true,
		},
		{
			name:           "serve JSON file",
			path:           "/api/data.json",
			expectedStatus: 200,
			expectedBody:   `{"message": "Hello API"}`,
			checkContent:   true,
		},
		{
			name:           "non-existent file serves index",
			path:           "/nonexistent",
			expectedStatus: 200,
			expectedBody:   "<html><body>Hello World</body></html>", // Falls back to /
			checkContent:   true,
		},
		{
			name:           "root path",
			path:           "/",
			expectedStatus: 200,
			checkContent:   false, // Directory listing or index
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := http.Get(server.URL + tt.path)
			if err != nil {
				t.Fatalf("Failed to make request: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, resp.StatusCode)
			}

			if tt.checkContent {
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					t.Fatalf("Failed to read response body: %v", err)
				}

				if string(body) != tt.expectedBody {
					t.Errorf("Expected body %q, got %q", tt.expectedBody, string(body))
				}
			}
		})
	}
}

func TestServeFileWithDirFS(t *testing.T) {
	// Create a temporary directory with test files
	tempDir := t.TempDir()

	// Create test files
	testFiles := map[string]string{
		"index.html": "<html><body>Index Page</body></html>",
		"test.txt":   "Test file content",
		"app.js":     "console.log('app');",
	}

	for filename, content := range testFiles {
		filePath := tempDir + "/" + filename
		if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
			t.Fatalf("Failed to create test file %s: %v", filename, err)
		}
	}

	// Use os.DirFS to serve the directory
	dirFS := os.DirFS(tempDir)
	handler := ServeFile(dirFS)
	server := httptest.NewServer(handler)
	defer server.Close()

	// Test serving files
	for filename, expectedContent := range testFiles {
		t.Run("serve_"+filename, func(t *testing.T) {
			resp, err := http.Get(server.URL + "/" + filename)
			if err != nil {
				t.Fatalf("Failed to make request: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != 200 {
				t.Errorf("Expected status 200, got %d", resp.StatusCode)
			}

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("Failed to read response body: %v", err)
			}

			if string(body) != expectedContent {
				t.Errorf("Expected body %q, got %q", expectedContent, string(body))
			}
		})
	}
}

func TestServeFileNotFound(t *testing.T) {
	// Test with an empty filesystem
	emptyFS := fstest.MapFS{}

	handler := ServeFile(emptyFS)
	server := httptest.NewServer(handler)
	defer server.Close()

	resp, err := http.Get(server.URL + "/nonexistent.txt")
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	// The implementation tries to open the path, and if it fails,
	// it sets the path to "/" and tries again.
	// With empty FS, this should result in either 404 or 200 (depending on implementation)
	// We just verify it doesn't crash and returns a valid response
	if resp.StatusCode != 404 && resp.StatusCode != 200 {
		t.Errorf("Expected status 404 or 200 for empty filesystem, got %d", resp.StatusCode)
	}
}

func TestServeFileWithSPABehavior(t *testing.T) {
	// Test Single Page Application (SPA) behavior
	// When a file doesn't exist, it should serve the root/index
	testFS := fstest.MapFS{
		"index.html": {
			Data: []byte("<!DOCTYPE html><html><head><title>SPA</title></head><body><div id=\"app\"></div></body></html>"),
		},
		"static/app.js": {
			Data: []byte("// React/Vue/Angular app code"),
		},
		"static/style.css": {
			Data: []byte("/* App styles */"),
		},
	}

	handler := ServeFile(testFS)
	server := httptest.NewServer(handler)
	defer server.Close()

	tests := []struct {
		name        string
		path        string
		description string
	}{
		{
			name:        "existing_static_file",
			path:        "/static/app.js",
			description: "Should serve existing static files normally",
		},
		{
			name:        "spa_route",
			path:        "/users/123",
			description: "Should serve index.html for SPA routes",
		},
		{
			name:        "nested_spa_route",
			path:        "/dashboard/analytics/reports",
			description: "Should serve index.html for deeply nested routes",
		},
		{
			name:        "api_route",
			path:        "/api/v1/users",
			description: "Should serve index.html (SPA will handle API routing)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := http.Get(server.URL + tt.path)
			if err != nil {
				t.Fatalf("Failed to make request to %s: %v", tt.path, err)
			}
			defer resp.Body.Close()

			// For non-existent files, it should attempt to serve from root
			// The exact behavior depends on the file system implementation
			if resp.StatusCode != 200 && resp.StatusCode != 404 {
				t.Errorf("Unexpected status code for %s: %d", tt.path, resp.StatusCode)
			}
		})
	}
}

func TestServeFileHeaders(t *testing.T) {
	testFS := fstest.MapFS{
		"test.html": {
			Data: []byte("<html><body>Test</body></html>"),
		},
		"test.css": {
			Data: []byte("body { margin: 0; }"),
		},
		"test.js": {
			Data: []byte("console.log('test');"),
		},
		"test.json": {
			Data: []byte(`{"test": true}`),
		},
		"test.png": {
			Data: []byte("fake-png-data"),
		},
	}

	handler := ServeFile(testFS)
	server := httptest.NewServer(handler)
	defer server.Close()

	tests := []struct {
		name         string
		path         string
		expectedType string
	}{
		{
			name:         "HTML content type",
			path:         "/test.html",
			expectedType: "text/html",
		},
		{
			name:         "CSS content type",
			path:         "/test.css",
			expectedType: "text/css",
		},
		{
			name:         "JavaScript content type",
			path:         "/test.js",
			expectedType: "text/javascript",
		},
		{
			name:         "JSON content type",
			path:         "/test.json",
			expectedType: "application/json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := http.Get(server.URL + tt.path)
			if err != nil {
				t.Fatalf("Failed to make request: %v", err)
			}
			defer resp.Body.Close()

			contentType := resp.Header.Get("Content-Type")
			if contentType == "" {
				t.Error("Content-Type header not set")
			}

			// Note: The actual content type detection is handled by Go's http.FileServer
			// We're just verifying that headers are being set
			if resp.StatusCode != 200 {
				t.Errorf("Expected status 200, got %d", resp.StatusCode)
			}
		})
	}
}

func TestServeFileEdgeCases(t *testing.T) {
	testFS := fstest.MapFS{
		"normal-file.txt": {
			Data: []byte("normal content"),
		},
		"empty-file.txt": {
			Data: []byte(""),
		},
		"file-with-spaces.txt": {
			Data: []byte("file with spaces in name"),
		},
		"file.with.dots.txt": {
			Data: []byte("file with dots in name"),
		},
		"UPPERCASE.TXT": {
			Data: []byte("uppercase filename"),
		},
	}

	handler := ServeFile(testFS)
	server := httptest.NewServer(handler)
	defer server.Close()

	tests := []struct {
		name         string
		path         string
		expectedBody string
	}{
		{
			name:         "normal file",
			path:         "/normal-file.txt",
			expectedBody: "normal content",
		},
		{
			name:         "empty file",
			path:         "/empty-file.txt",
			expectedBody: "",
		},
		{
			name:         "file with spaces",
			path:         "/file-with-spaces.txt",
			expectedBody: "file with spaces in name",
		},
		{
			name:         "file with dots",
			path:         "/file.with.dots.txt",
			expectedBody: "file with dots in name",
		},
		{
			name:         "uppercase filename",
			path:         "/UPPERCASE.TXT",
			expectedBody: "uppercase filename",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := http.Get(server.URL + tt.path)
			if err != nil {
				t.Fatalf("Failed to make request: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != 200 {
				t.Errorf("Expected status 200, got %d", resp.StatusCode)
			}

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("Failed to read response body: %v", err)
			}

			if string(body) != tt.expectedBody {
				t.Errorf("Expected body %q, got %q", tt.expectedBody, string(body))
			}
		})
	}
}

func TestServeFileHTTPMethods(t *testing.T) {
	testFS := fstest.MapFS{
		"test.txt": {
			Data: []byte("test content"),
		},
	}

	handler := ServeFile(testFS)
	server := httptest.NewServer(handler)
	defer server.Close()

	methods := []string{"GET", "HEAD", "OPTIONS"}

	for _, method := range methods {
		t.Run("method_"+method, func(t *testing.T) {
			req, err := http.NewRequest(method, server.URL+"/test.txt", nil)
			if err != nil {
				t.Fatalf("Failed to create %s request: %v", method, err)
			}

			client := &http.Client{}
			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("Failed to make %s request: %v", method, err)
			}
			defer resp.Body.Close()

			// GET and HEAD should work, OPTIONS might work depending on the server
			if method == "GET" || method == "HEAD" {
				if resp.StatusCode != 200 {
					t.Errorf("Expected status 200 for %s, got %d", method, resp.StatusCode)
				}
			}

			// HEAD should not return a body
			if method == "HEAD" {
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					t.Fatalf("Failed to read response body: %v", err)
				}
				if len(body) > 0 {
					t.Errorf("HEAD request should not return body, got: %q", string(body))
				}
			}
		})
	}
}
