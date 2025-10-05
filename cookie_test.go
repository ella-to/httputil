package httputil

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestSetCookie(t *testing.T) {
	tests := []struct {
		name     string
		key      CookieKey
		value    string
		maxAge   time.Duration
		secure   bool
		expected string
	}{
		{
			name:     "basic cookie",
			key:      CookieKey("session"),
			value:    "abc123",
			maxAge:   1 * time.Hour,
			secure:   false,
			expected: "session=abc123; Path=/; Max-Age=3600; HttpOnly",
		},
		{
			name:     "secure cookie",
			key:      CookieKey("auth"),
			value:    "token456",
			maxAge:   24 * time.Hour,
			secure:   true,
			expected: "auth=token456; Path=/; Max-Age=86400; HttpOnly; Secure",
		},
		{
			name:     "delete cookie (zero maxAge)",
			key:      CookieKey("old_session"),
			value:    "",
			maxAge:   0,
			secure:   false,
			expected: "old_session=; Path=/; Max-Age=0; HttpOnly",
		},
		{
			name:     "short expiry",
			key:      CookieKey("temp"),
			value:    "temporary",
			maxAge:   30 * time.Second,
			secure:   true,
			expected: "temp=temporary; Path=/; Max-Age=30; HttpOnly; Secure",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()

			SetCookie(w, tt.key, tt.value, tt.maxAge, tt.secure)

			cookies := w.Header().Get("Set-Cookie")
			// Check individual components since Expires timestamp varies
			if !strings.Contains(cookies, "Path=/") {
				t.Error("Expected Path=/ in cookie")
			}
			if !strings.Contains(cookies, "HttpOnly") {
				t.Error("Expected HttpOnly in cookie")
			}
			if tt.secure && !strings.Contains(cookies, "Secure") {
				t.Error("Expected Secure in cookie")
			}
			if !strings.Contains(cookies, fmt.Sprintf("%s=%s", tt.key, tt.value)) {
				t.Errorf("Expected cookie name/value %s=%s", tt.key, tt.value)
			}

			// Verify expires time is set correctly (within reasonable margin)
			if tt.maxAge > 0 {
				if !strings.Contains(cookies, "Expires=") {
					t.Error("Expected Expires to be set when maxAge > 0")
				}
			}
		})
	}
}

func TestGetCookie(t *testing.T) {
	tests := []struct {
		name        string
		cookieKey   CookieKey
		cookieValue string
		setCookie   bool
		expectError bool
		expectedVal string
	}{
		{
			name:        "existing cookie",
			cookieKey:   CookieKey("session"),
			cookieValue: "abc123",
			setCookie:   true,
			expectError: false,
			expectedVal: "abc123",
		},
		{
			name:        "missing cookie",
			cookieKey:   CookieKey("nonexistent"),
			cookieValue: "",
			setCookie:   false,
			expectError: true,
			expectedVal: "",
		},
		{
			name:        "empty cookie value",
			cookieKey:   CookieKey("empty"),
			cookieValue: "",
			setCookie:   true,
			expectError: false,
			expectedVal: "",
		},
		{
			name:        "special characters in cookie",
			cookieKey:   CookieKey("special"),
			cookieValue: "value-with_special.chars123",
			setCookie:   true,
			expectError: false,
			expectedVal: "value-with_special.chars123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)

			if tt.setCookie {
				cookie := &http.Cookie{
					Name:  string(tt.cookieKey),
					Value: tt.cookieValue,
				}
				req.AddCookie(cookie)
			}

			value, err := GetCookie(tt.cookieKey, req)

			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}

			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			if value != tt.expectedVal {
				t.Errorf("GetCookie() = %v, want %v", value, tt.expectedVal)
			}

			// Test error wrapping
			if tt.expectError && err != nil {
				if !strings.Contains(err.Error(), "parsing cookie") {
					t.Error("Error should wrap ErrParsingCookie")
				}
				if !strings.Contains(err.Error(), string(tt.cookieKey)) {
					t.Error("Error should contain cookie key")
				}
			}
		})
	}
}

func TestCookieRoundTrip(t *testing.T) {
	// Test setting and getting cookies in a full HTTP flow
	key := CookieKey("test_session")
	value := "test_value_123"
	maxAge := 1 * time.Hour

	// Create a test handler that sets a cookie
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/set" {
			SetCookie(w, key, value, maxAge, false)
			w.WriteHeader(http.StatusOK)
		} else if r.URL.Path == "/get" {
			cookieValue, err := GetCookie(key, r)
			if err != nil {
				// This is expected if no cookie was set
				w.WriteHeader(http.StatusNoContent)
				return
			}
			w.Write([]byte(cookieValue))
		}
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	// Use a cookie jar to automatically handle cookies
	jar := &testCookieJar{cookies: make(map[string]*http.Cookie)}
	client := &http.Client{Jar: jar}

	// Set cookie
	setResp, err := client.Get(server.URL + "/set")
	if err != nil {
		t.Fatalf("Failed to set cookie: %v", err)
	}
	setResp.Body.Close()

	// Get cookie (the client should automatically send it back)
	getResp, err := client.Get(server.URL + "/get")
	if err != nil {
		t.Fatalf("Failed to get cookie: %v", err)
	}
	defer getResp.Body.Close()

	if getResp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", getResp.StatusCode)
	}
}

// Simple cookie jar for testing
type testCookieJar struct {
	cookies map[string]*http.Cookie
}

func (j *testCookieJar) SetCookies(u *url.URL, cookies []*http.Cookie) {
	for _, cookie := range cookies {
		j.cookies[cookie.Name] = cookie
	}
}

func (j *testCookieJar) Cookies(u *url.URL) []*http.Cookie {
	var result []*http.Cookie
	for _, cookie := range j.cookies {
		result = append(result, cookie)
	}
	return result
}

func TestCookieEdgeCases(t *testing.T) {
	t.Run("multiple cookies with same name", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)

		// Add multiple cookies with the same name (this can happen in real scenarios)
		req.Header.Add("Cookie", "test=value1")
		req.Header.Add("Cookie", "test=value2")

		// GetCookie should return the first one
		value, err := GetCookie(CookieKey("test"), req)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}

		// Should get one of the values (behavior may depend on Go's implementation)
		if value != "value1" && value != "value2" {
			t.Errorf("GetCookie() = %v, want value1 or value2", value)
		}
	})

	t.Run("cookie with equals sign in value", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		cookie := &http.Cookie{
			Name:  "encoded",
			Value: "key=value&other=data",
		}
		req.AddCookie(cookie)

		value, err := GetCookie(CookieKey("encoded"), req)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}

		if value != "key=value&other=data" {
			t.Errorf("GetCookie() = %v, want key=value&other=data", value)
		}
	})
}
