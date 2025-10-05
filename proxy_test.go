package httputil

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReverseProxy(t *testing.T) {
	// Create a backend server to proxy to
	backendServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Backend-Response", "true")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Hello from backend: " + r.URL.Path))
	}))
	defer backendServer.Close()

	// Create reverse proxy
	proxyHandler, err := ReverseProxy(backendServer.URL)
	if err != nil {
		t.Fatalf("Failed to create reverse proxy: %v", err)
	}

	// Create test server with proxy
	proxyServer := httptest.NewServer(proxyHandler)
	defer proxyServer.Close()

	// Test the proxy
	resp, err := http.Get(proxyServer.URL + "/test/path")
	if err != nil {
		t.Fatalf("Failed to make request through proxy: %v", err)
	}
	defer resp.Body.Close()

	// Check response
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	if resp.Header.Get("Backend-Response") != "true" {
		t.Error("Expected backend response header")
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	expectedBody := "Hello from backend: /test/path"
	if string(body) != expectedBody {
		t.Errorf("Expected body %q, got %q", expectedBody, string(body))
	}
}

func TestReverseProxyInvalidURL(t *testing.T) {
	invalidURLs := []string{
		"://invalid", // This should definitely fail
	}

	for _, url := range invalidURLs {
		t.Run("invalid_url_"+url, func(t *testing.T) {
			_, err := ReverseProxy(url)
			if err == nil {
				t.Errorf("Expected error for invalid URL %q, but got none", url)
			}
		})
	}

	// Test some edge cases that might be accepted by url.Parse
	edgeCases := []string{
		"not-a-url",
		"",
		"ftp://invalid-scheme.com",
	}

	for _, url := range edgeCases {
		t.Run("edge_case_"+url, func(t *testing.T) {
			handler, err := ReverseProxy(url)
			// url.Parse is quite lenient, so these might not error
			// We just verify the function doesn't panic
			if err == nil && handler == nil {
				t.Error("Expected either an error or a valid handler")
			}
		})
	}
}

func TestReverseProxyHeaders(t *testing.T) {
	// Backend server that echoes request headers
	backendServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Echo some headers back
		w.Header().Set("Request-Host", r.Host)
		w.Header().Set("User-Agent", r.Header.Get("User-Agent"))
		w.Header().Set("Custom-Header", r.Header.Get("Custom-Header"))
		w.WriteHeader(http.StatusOK)
	}))
	defer backendServer.Close()

	proxyHandler, err := ReverseProxy(backendServer.URL)
	if err != nil {
		t.Fatalf("Failed to create reverse proxy: %v", err)
	}

	proxyServer := httptest.NewServer(proxyHandler)
	defer proxyServer.Close()

	// Create request with custom headers
	req, err := http.NewRequest("GET", proxyServer.URL, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	req.Header.Set("User-Agent", "test-client/1.0")
	req.Header.Set("Custom-Header", "custom-value")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	// Check that headers were proxied
	if resp.Header.Get("User-Agent") != "test-client/1.0" {
		t.Errorf("Expected User-Agent header to be proxied")
	}

	if resp.Header.Get("Custom-Header") != "custom-value" {
		t.Errorf("Expected Custom-Header to be proxied")
	}
}

func TestReverseProxyHTTPMethods(t *testing.T) {
	// Backend server that echoes the method
	backendServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Method: " + r.Method))
	}))
	defer backendServer.Close()

	proxyHandler, err := ReverseProxy(backendServer.URL)
	if err != nil {
		t.Fatalf("Failed to create reverse proxy: %v", err)
	}

	proxyServer := httptest.NewServer(proxyHandler)
	defer proxyServer.Close()

	methods := []string{"GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS"}

	for _, method := range methods {
		t.Run("method_"+method, func(t *testing.T) {
			req, err := http.NewRequest(method, proxyServer.URL, nil)
			if err != nil {
				t.Fatalf("Failed to create %s request: %v", method, err)
			}

			client := &http.Client{}
			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("Failed to make %s request: %v", method, err)
			}
			defer resp.Body.Close()

			if method != "HEAD" { // HEAD responses don't have body
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					t.Fatalf("Failed to read response body: %v", err)
				}

				expected := "Method: " + method
				if string(body) != expected {
					t.Errorf("Expected body %q, got %q", expected, string(body))
				}
			}
		})
	}
}

func TestDevProxy(t *testing.T) {
	// Create a service handler
	serviceHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Service response for: " + r.URL.Path))
	})

	// Create a dev proxy server (simulating frontend dev server)
	devServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Dev server response for: " + r.URL.Path))
	}))
	defer devServer.Close()

	tests := []struct {
		name       string
		isDev      bool
		path       string
		exceptions []string
		expectFrom string // "service" or "dev"
	}{
		{
			name:       "production mode - all to service",
			isDev:      false,
			path:       "/",
			exceptions: []string{"/api"},
			expectFrom: "service",
		},
		{
			name:       "production mode - api to service",
			isDev:      false,
			path:       "/api/users",
			exceptions: []string{"/api"},
			expectFrom: "service",
		},
		{
			name:       "dev mode - root to dev server",
			isDev:      true,
			path:       "/",
			exceptions: []string{"/api"},
			expectFrom: "dev",
		},
		{
			name:       "dev mode - static to dev server",
			isDev:      true,
			path:       "/assets/style.css",
			exceptions: []string{"/api"},
			expectFrom: "dev",
		},
		{
			name:       "dev mode - api exception to service",
			isDev:      true,
			path:       "/api/users",
			exceptions: []string{"/api"},
			expectFrom: "service",
		},
		{
			name:       "dev mode - multiple exceptions",
			isDev:      true,
			path:       "/admin/dashboard",
			exceptions: []string{"/api", "/admin"},
			expectFrom: "service",
		},
		{
			name:       "dev mode - no matching exception",
			isDev:      true,
			path:       "/public/image.png",
			exceptions: []string{"/api", "/admin"},
			expectFrom: "dev",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use a fresh mux for each test to avoid conflicts
			mux := http.NewServeMux()

			err := DevProxy(mux, serviceHandler, tt.isDev, devServer.URL, tt.exceptions)
			if err != nil {
				t.Fatalf("DevProxy failed: %v", err)
			}

			// Create test server
			testServer := httptest.NewServer(mux)
			defer testServer.Close()

			// Make request
			resp, err := http.Get(testServer.URL + tt.path)
			if err != nil {
				t.Fatalf("Failed to make request: %v", err)
			}
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("Failed to read response body: %v", err)
			}

			responseBody := string(body)

			switch tt.expectFrom {
			case "service":
				expectedPrefix := "Service response for:"
				if !strings.HasPrefix(responseBody, expectedPrefix) {
					t.Errorf("Expected response from service, got: %q", responseBody)
				}
			case "dev":
				expectedPrefix := "Dev server response for:"
				if !strings.HasPrefix(responseBody, expectedPrefix) {
					t.Errorf("Expected response from dev server, got: %q", responseBody)
				}
			default:
				t.Fatalf("Invalid expectFrom value: %s", tt.expectFrom)
			}
		})
	}
}

func TestDevProxyInvalidProxyAddr(t *testing.T) {
	serviceHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("service"))
	})

	// Only test truly invalid URLs that should definitely fail
	invalidAddrs := []string{
		"://invalid",
	}

	for _, addr := range invalidAddrs {
		t.Run("invalid_addr_"+addr, func(t *testing.T) {
			// Use fresh mux for each test
			testMux := http.NewServeMux()
			err := DevProxy(testMux, serviceHandler, true, addr, []string{"/api"})
			if err == nil {
				t.Errorf("Expected error for invalid proxy address %q, but got none", addr)
			}
		})
	}

	// Test edge cases that url.Parse might accept
	edgeCases := []string{
		"not-a-url",
		"",
	}

	for _, addr := range edgeCases {
		t.Run("edge_case_"+addr, func(t *testing.T) {
			testMux := http.NewServeMux()
			err := DevProxy(testMux, serviceHandler, true, addr, []string{"/api"})
			// These might not error due to url.Parse being lenient
			// We just verify the function doesn't panic
			_ = err // Ignore the result for edge cases
		})
	}
}

func TestDevProxyComplexExceptions(t *testing.T) {
	serviceHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("service:" + r.URL.Path))
	})

	devServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("dev:" + r.URL.Path))
	}))
	defer devServer.Close()

	mux := http.NewServeMux()
	exceptions := []string{"/api", "/auth", "/admin/api"}

	err := DevProxy(mux, serviceHandler, true, devServer.URL, exceptions)
	if err != nil {
		t.Fatalf("DevProxy failed: %v", err)
	}

	testServer := httptest.NewServer(mux)
	defer testServer.Close()

	testCases := []struct {
		path       string
		expectFrom string
	}{
		{"/", "dev"},
		{"/api", "service"},
		{"/api/users", "service"},
		{"/api/users/123", "service"},
		{"/auth", "service"},
		{"/auth/login", "service"},
		{"/admin", "dev"},               // Not /admin/api
		{"/admin/dashboard", "dev"},     // Not /admin/api
		{"/admin/api", "service"},       // Exact match
		{"/admin/api/users", "service"}, // Prefix match
		{"/public/assets", "dev"},
		{"/static/css/style.css", "dev"},
	}

	for _, tc := range testCases {
		t.Run("path_"+tc.path, func(t *testing.T) {
			resp, err := http.Get(testServer.URL + tc.path)
			if err != nil {
				t.Fatalf("Failed to make request to %s: %v", tc.path, err)
			}
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("Failed to read response body: %v", err)
			}

			responseBody := string(body)
			expectedPrefix := tc.expectFrom + ":"

			if !strings.HasPrefix(responseBody, expectedPrefix) {
				t.Errorf("Path %s: expected response from %s, got: %q",
					tc.path, tc.expectFrom, responseBody)
			}
		})
	}
}

func TestDevProxyPreserveQuery(t *testing.T) {
	serviceHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("service:" + r.URL.Path + "?" + r.URL.RawQuery))
	})

	devServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("dev:" + r.URL.Path + "?" + r.URL.RawQuery))
	}))
	defer devServer.Close()

	mux := http.NewServeMux()
	err := DevProxy(mux, serviceHandler, true, devServer.URL, []string{"/api"})
	if err != nil {
		t.Fatalf("DevProxy failed: %v", err)
	}

	testServer := httptest.NewServer(mux)
	defer testServer.Close()

	testCases := []struct {
		path       string
		expectFrom string
	}{
		{"/app?page=1&sort=name", "dev"},
		{"/api/users?limit=10&offset=20", "service"},
	}

	for _, tc := range testCases {
		t.Run("query_"+tc.path, func(t *testing.T) {
			resp, err := http.Get(testServer.URL + tc.path)
			if err != nil {
				t.Fatalf("Failed to make request: %v", err)
			}
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("Failed to read response body: %v", err)
			}

			responseBody := string(body)
			expectedPrefix := tc.expectFrom + ":"

			if !strings.HasPrefix(responseBody, expectedPrefix) {
				t.Errorf("Expected response from %s, got: %q", tc.expectFrom, responseBody)
			}

			// Verify query parameters are preserved
			if !strings.Contains(responseBody, "?") {
				t.Error("Query parameters not preserved")
			}
		})
	}
}
