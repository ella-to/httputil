```
██╗░░██╗████████╗████████╗██████╗░██╗░░░██╗████████╗██╗██╗░░░░░
██║░░██║╚══██╔══╝╚══██╔══╝██╔══██╗██║░░░██║╚══██╔══╝██║██║░░░░░
███████║░░░██║░░░░░░██║░░░██████╔╝██║░░░██║░░░██║░░░██║██║░░░░░
██╔══██║░░░██║░░░░░░██║░░░██╔═══╝░██║░░░██║░░░██║░░░██║██║░░░░░
██║░░██║░░░██║░░░░░░██║░░░██║░░░░░╚██████╔╝░░░██║░░░██║███████╗
╚═╝░░╚═╝░░░╚═╝░░░░░░╚═╝░░░╚═╝░░░░░░╚═════╝░░░░╚═╝░░░╚═╝╚══════╝
```

<div align="center">

[![Go Reference](https://pkg.go.dev/badge/ella.to/httputil.svg)](https://pkg.go.dev/ella.to/httputil)
[![Go Report Card](https://goreportcard.com/badge/ella.to/httputil)](https://goreportcard.com/report/ella.to/httputil)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

A comprehensive HTTP utilities library for Go that provides common HTTP functionality including JWT handling, middleware, proxying, file serving, and more.

</div>


## Features

- 🍪 **Cookie Management** - Easy cookie setting and retrieval with security options
- 🔐 **JWT Utilities** - JWT encoding/decoding with custom claims support  
- 🔄 **HTTP Retry Client** - Configurable retry logic with exponential backoff
- 🛠️ **Middleware** - Logging, session context, and middleware chaining
- 🔄 **Reverse Proxy** - Simple reverse proxy with development mode support
- 📁 **File Serving** - Static file serving with SPA fallback support
- 📤 **Streaming Upload** - Multipart upload server/client helpers with strict limits
- 📥 **Streaming Download** - Download server/client helpers with parsed metadata
- 📏 **Request Limiting** - Request body size limiting for security
- ⚡ **Zero Dependencies** - Minimal external dependencies (only JWT library)

## Installation

```bash
go get ella.to/httputil@0.1.0
```

## Quick Start

```go
package main

import (
    "net/http"
    "time"

    "ella.to/httputil"
)

func main() {
    // Create a retry client
    client, _ := httputil.NewRetryClient(
        httputil.WithMaxRetries(3),
        httputil.WithHeaders(map[string]string{
            "User-Agent": "my-app/1.0",
        }),
    )
    
    // Set up middleware
    mux := http.NewServeMux()
    mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        httputil.SetCookie(w, "session", "abc123", 24*time.Hour, true)
        w.Write([]byte("Hello World"))
    })
    
    // Chain middleware
    handler := httputil.Chain(mux, httputil.WithLogging)
    
    http.ListenAndServe(":8080", handler)
}
```

## API Reference

### Cookie Management

#### SetCookie

Sets an HTTP cookie with security options.

```go
func SetCookie(w http.ResponseWriter, key CookieKey, value string, maxAge time.Duration, secure bool)
```

**Parameters:**
- `w` - HTTP response writer
- `key` - Cookie name (typed as CookieKey for type safety)
- `value` - Cookie value
- `maxAge` - Cookie expiration duration (0 deletes the cookie)
- `secure` - Whether cookie should only be sent over HTTPS

**Example:**
```go
// Set a secure session cookie for 24 hours
httputil.SetCookie(w, httputil.CookieKey("session"), "user123", 24*time.Hour, true)

// Delete a cookie
httputil.SetCookie(w, httputil.CookieKey("old_session"), "", 0, false)
```

#### GetCookie

Retrieves a cookie value from the request.

```go
func GetCookie(key CookieKey, r *http.Request) (string, error)
```

**Returns:** Cookie value and error (wrapped with context if cookie not found)

**Example:**
```go
sessionID, err := httputil.GetCookie(httputil.CookieKey("session"), r)
if err != nil {
    // Handle missing or invalid cookie
    http.Error(w, "Unauthorized", http.StatusUnauthorized)
    return
}
// Use sessionID...
```

### JWT Utilities

#### Creating a JWT Handler

```go
func New(secretKey string) *Jwt
```

**Example:**
```go
jwtHandler := httputil.New("your-secret-key")
```

#### Encoding JWT

```go
func (j *Jwt) Encode(claims JwtClaims) (string, error)
```

**Example:**
```go
type UserClaims struct {
    *httputil.JwtRegisteredClaims
    UserID string `json:"user_id"`
    Role   string `json:"role"`
}

func (c *UserClaims) ParseToken(token *httputil.JwtToken) error {
    if !token.Valid {
        return errors.New("invalid token")
    }
    return nil
}

claims := &UserClaims{
    JwtRegisteredClaims: &httputil.JwtRegisteredClaims{
        ExpiresAt: httputil.JwtNewNumericDate(time.Now().Add(24 * time.Hour)),
        Subject:   "user123",
    },
    UserID: "12345",
    Role:   "admin",
}

token, err := jwtHandler.Encode(claims)
```

#### Decoding JWT

```go
func (j *Jwt) Decode(jwt string, claims JwtClaims) error
```

**Note:** Claims must implement the `TokenParser` interface:

```go
type TokenParser interface {
    ParseToken(token *JwtToken) error
}
```

**Example:**
```go
decodedClaims := &UserClaims{JwtRegisteredClaims: &httputil.JwtRegisteredClaims{}}
err := jwtHandler.Decode(token, decodedClaims)
if err != nil {
    // Handle invalid token
}
// Use decodedClaims.UserID, decodedClaims.Role, etc.
```

### HTTP Retry Client

#### Creating a Retry Client

```go
func NewRetryClient(opts ...retryTransportOpt) (*http.Client, error)
```

**Available Options:**
- `WithMaxRetries(int)` - Maximum number of retries (default: 3)
- `WithInitialDelay(time.Duration)` - Initial delay between retries (default: 1s)
- `WithMaxDelay(time.Duration)` - Maximum delay cap (default: 30s)
- `WithHeaders(map[string]string)` - Headers to inject into every request

**Example:**
```go
client, err := httputil.NewRetryClient(
    httputil.WithMaxRetries(5),
    httputil.WithInitialDelay(500*time.Millisecond),
    httputil.WithMaxDelay(10*time.Second),
    httputil.WithHeaders(map[string]string{
        "User-Agent": "my-service/1.0",
        "Accept":     "application/json",
    }),
)

// Use like any http.Client
resp, err := client.Get("https://api.example.com/data")
```

**Retry Behavior:**
- Retries on 5xx server errors and 429 (Too Many Requests)
- Uses exponential backoff with jitter
- Preserves request body for retries
- No retry on 4xx client errors (except 429)

### Middleware

#### Chain

Chains multiple middleware functions together.

```go
func Chain(h http.Handler, middleware ...func(http.Handler) http.Handler) http.Handler
```

**Example:**
```go
handler := httputil.Chain(
    yourHandler,
    httputil.WithLogging,
    sessionMiddleware,
    authMiddleware,
)
```

#### WithLogging

Logs HTTP requests with method, path, status code, and response size.

```go
func WithLogging(next http.Handler) http.Handler
```

**Example:**
```go
handler := httputil.WithLogging(yourHandler)
```

**Log Output:**
```
INFO http called method=GET path=/api/users code=200 size=1024
ERROR http called method=POST path=/api/users code=500 size=0
```

#### WithSessionContext

Extracts session information from Bearer tokens or cookies and adds it to request context.

```go
func WithSessionContext[T any](
    cookieKey CookieKey, 
    ctxKey ContextKey, 
    ctxTokenKey ContextKey, 
    parseSession func(token string) (T, error)
) func(next http.Handler) http.Handler
```

**Parameters:**
- `cookieKey` - Cookie name to check for token
- `ctxKey` - Context key to store parsed session
- `ctxTokenKey` - Context key to store raw token (empty string to skip)
- `parseSession` - Function to parse token into session data

**Example:**
```go
type User struct {
    ID   string
    Role string
}

parseSession := func(token string) (User, error) {
    // Parse your token (JWT, database lookup, etc.)
    return User{ID: "123", Role: "admin"}, nil
}

middleware := httputil.WithSessionContext(
    httputil.CookieKey("auth_token"),
    httputil.ContextKey("user"),
    httputil.ContextKey("token"), // Store raw token
    parseSession,
)

handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    if user, ok := r.Context().Value(httputil.ContextKey("user")).(User); ok {
        fmt.Fprintf(w, "Hello, %s (Role: %s)", user.ID, user.Role)
    } else {
        http.Error(w, "Unauthorized", http.StatusUnauthorized)
    }
}))
```

**Token Sources** (in priority order):
1. `Authorization: Bearer <token>` header
2. Cookie specified by `cookieKey`

### Reverse Proxy

#### ReverseProxy

Creates a simple reverse proxy to another HTTP service.

```go
func ReverseProxy(rawURL string) (http.HandlerFunc, error)
```

**Example:**
```go
// Proxy all requests to another server
proxyHandler, err := httputil.ReverseProxy("http://backend:8080")
if err != nil {
    log.Fatal(err)
}

http.Handle("/api/", proxyHandler)
```

#### DevProxy

Sets up development-mode proxying with exceptions for certain paths.

```go
func DevProxy(mux *http.ServeMux, service http.Handler, isDev bool, proxyAddr string, exceptions []string) error
```

**Parameters:**
- `mux` - HTTP mux to configure
- `service` - Handler for your main service
- `isDev` - Whether in development mode
- `proxyAddr` - Address of development server (e.g., frontend dev server)
- `exceptions` - Paths that should go to service instead of proxy

**Example:**
```go
mux := http.NewServeMux()
serviceHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    w.Write([]byte("API Response"))
})

// In development: proxy UI requests to dev server, API requests to service
err := httputil.DevProxy(
    mux,
    serviceHandler,
    true,                    // isDev
    "http://localhost:3000", // frontend dev server
    []string{"/api", "/auth"}, // exceptions - these go to service
)

// Requests to /api/* -> serviceHandler
// Requests to /* -> proxy to localhost:3000
```

### File Serving

#### ServeFile

Serves static files from a filesystem with SPA (Single Page Application) fallback.

```go
func ServeFile(fs fs.FS) http.Handler
```

**Example with embedded files:**
```go
//go:embed static/*
var staticFiles embed.FS

func main() {
    staticFS, _ := fs.Sub(staticFiles, "static")
    http.Handle("/", httputil.ServeFile(staticFS))
}
```

**Example with directory:**
```go
http.Handle("/static/", httputil.ServeFile(os.DirFS("./public")))
```

**SPA Behavior:**
- If requested file exists, serves it normally
- If file doesn't exist, sets path to "/" and serves that (typically index.html)
- Perfect for React/Vue/Angular SPAs with client-side routing

### Request Limiting

#### ReadLimiter

Applies size limit to request body (note: function name has typo but works correctly).

```go
func ReadLimter(size int64, w http.ResponseWriter, r *http.Request)
```

**Example:**
```go
func handler(w http.ResponseWriter, r *http.Request) {
    // Limit request body to 1MB
    httputil.ReadLimter(1024*1024, w, r)
    
    body, err := io.ReadAll(r.Body)
    if err != nil {
        // Handle size limit exceeded
        return
    }
    // Process body...
}
```

#### ReadBodyLimiter

Applies size limit and reads the body in one operation.

```go
func ReadBodyLimiter(size int64, w http.ResponseWriter, r *http.Request) ([]byte, error)
```

**Example:**
```go
func apiHandler(w http.ResponseWriter, r *http.Request) {
    // Limit and read body (max 10KB)
    body, err := httputil.ReadBodyLimiter(10*1024, w, r)
    if err != nil {
        // ReadBodyLimiter already set 400 status
        w.Write([]byte("Request too large"))
        return
    }
    
    // Parse JSON, etc.
    var data map[string]interface{}
    json.Unmarshal(body, &data)
    // Process data...
}
```

### Upload

#### StreamMultipartUpload

Streams multipart file uploads on the server with hard limits.

```go
func StreamMultipartUpload(
        r *http.Request,
        limits UploadLimits,
        onFile func(file UploadedFile, content io.Reader) error,
) (UploadSummary, error)
```

**Limits:**
- `MaxFileSize` - Maximum bytes per file (`0` means unlimited)
- `MaxFiles` - Maximum number of files (`0` means unlimited)
- `MaxTotalBytes` - Maximum bytes across all files (`0` means unlimited)

**Example (server):**
```go
func uploadHandler(w http.ResponseWriter, r *http.Request) {
        summary, err := httputil.StreamMultipartUpload(r, httputil.UploadLimits{
                MaxFileSize:   10 << 20, // 10MB per file
                MaxFiles:      5,
                MaxTotalBytes: 25 << 20, // 25MB total
        }, func(file httputil.UploadedFile, content io.Reader) error {
                // Stream directly to storage (disk, S3, etc.) without buffering full file.
                dst, err := os.Create("./uploads/" + file.FileName)
                if err != nil {
                        return err
                }
                defer dst.Close()

                _, err = io.Copy(dst, content)
                return err
        })
        if err != nil {
                var limitErr *httputil.UploadLimitError
                if errors.As(err, &limitErr) {
                        http.Error(w, limitErr.Error(), http.StatusRequestEntityTooLarge)
                        return
                }
                http.Error(w, err.Error(), http.StatusBadRequest)
                return
        }

        fmt.Fprintf(w, "uploaded files=%d bytes=%d", summary.Files, summary.TotalBytes)
}
```

#### NewMultipartUploadRequest

Creates a streaming multipart upload request on the client.

```go
func NewMultipartUploadRequest(
        ctx context.Context,
        method string,
        url string,
        fields map[string]string,
        files []UploadRequestFile,
        limits UploadLimits,
) (*http.Request, error)
```

**Example (client):**
```go
req, err := httputil.NewMultipartUploadRequest(
        context.Background(),
        http.MethodPost,
        "https://api.example.com/upload",
        map[string]string{"folder": "avatars"},
        []httputil.UploadRequestFile{
                {
                        FieldName:   "files",
                        FileName:    "profile.png",
                        ContentType: "image/png",
                        Reader:      fileReader,
                        Size:        fileSize, // set -1 if unknown
                },
        },
        httputil.UploadLimits{MaxFileSize: 10 << 20, MaxFiles: 3, MaxTotalBytes: 20 << 20},
)
if err != nil {
        // Includes UploadLimitError when limits are exceeded
}

resp, err := http.DefaultClient.Do(req)
```

### Download

#### ServeDownload

Server helper for streamed file downloads. The provider inspects request data and returns an `io.ReadCloser` stream.

```go
func ServeDownload(
        w http.ResponseWriter,
        r *http.Request,
        provider func(r *http.Request) (DownloadSource, error),
) error
```

**Example (server):**
```go
func downloadHandler(w http.ResponseWriter, r *http.Request) {
        err := httputil.ServeDownload(w, r, func(req *http.Request) (httputil.DownloadSource, error) {
                fileID := req.URL.Query().Get("id")
                rc, size, err := openFileFromStore(fileID)
                if err != nil {
                        return httputil.DownloadSource{}, err
                }
                return httputil.DownloadSource{
                        Name:        "report.csv",
                        ContentType: "text/csv",
                        Size:        size,
                        Reader:      rc,
                }, nil
        })
        if err != nil {
                http.Error(w, err.Error(), http.StatusNotFound)
        }
}
```

#### Download

Client helper that executes a request and parses `Content-Disposition`, `Content-Type`, and `Content-Length`.

```go
func Download(client *http.Client, req *http.Request) (*DownloadResponse, error)
```

**Example (client):**
```go
req, _ := http.NewRequest(http.MethodGet, "https://api.example.com/download?id=123", nil)
resp, err := httputil.Download(http.DefaultClient, req)
if err != nil {
        return
}
defer resp.Body.Close()

fmt.Println("name:", resp.Metadata.FileName)
fmt.Println("type:", resp.Metadata.ContentType)
fmt.Println("size:", resp.Metadata.Size)

_, _ = io.Copy(dstFile, resp.Body)
```

## Browser Upload & Download (TypeScript)

The browser can enforce the same constraints before sending to the server, and can parse download metadata from response headers.

### Upload Example (TypeScript)

```ts
type UploadLimits = {
    maxFileSize: number;
    maxFiles: number;
    maxTotalBytes: number;
};

async function uploadFiles(endpoint: string, files: File[], limits: UploadLimits) {
    if (files.length > limits.maxFiles) {
        throw new Error(`too many files: ${files.length} > ${limits.maxFiles}`);
    }

    let total = 0;
    for (const file of files) {
        if (file.size > limits.maxFileSize) {
            throw new Error(`file too large: ${file.name}`);
        }
        total += file.size;
    }

    if (total > limits.maxTotalBytes) {
        throw new Error(`total bytes too large: ${total} > ${limits.maxTotalBytes}`);
    }

    const formData = new FormData();
    for (const file of files) {
        formData.append("files", file, file.name);
    }

    const res = await fetch(endpoint, {
        method: "POST",
        body: formData,
    });

    if (!res.ok) {
        throw new Error(`upload failed: ${res.status}`);
    }

    return res.text();
}
```

### Download Example (TypeScript)

```ts
function parseFileName(contentDisposition: string | null): string | undefined {
    if (!contentDisposition) return undefined;
    const utf8Match = contentDisposition.match(/filename\*=UTF-8''([^;]+)/i);
    if (utf8Match?.[1]) return decodeURIComponent(utf8Match[1]);
    const plainMatch = contentDisposition.match(/filename="?([^";]+)"?/i);
    return plainMatch?.[1];
}

async function downloadFile(endpoint: string) {
    const res = await fetch(endpoint, { method: "GET" });
    if (!res.ok) {
        throw new Error(`download failed: ${res.status}`);
    }

    const fileName = parseFileName(res.headers.get("content-disposition")) ?? "download.bin";
    const contentType = res.headers.get("content-type") ?? "application/octet-stream";
    const size = Number(res.headers.get("content-length") ?? "-1");

    const blob = await res.blob();
    const url = URL.createObjectURL(new Blob([blob], { type: contentType }));

    const a = document.createElement("a");
    a.href = url;
    a.download = fileName;
    a.click();
    URL.revokeObjectURL(url);

    return { fileName, contentType, size };
}
```

## Error Handling

### Cookie Errors

```go
value, err := httputil.GetCookie(key, r)
if err != nil {
    if errors.Is(err, httputil.ErrParsingCookie) {
        // Handle cookie parsing error
    }
}
```

### JWT Errors

```go
err := jwtHandler.Decode(token, claims)
if err != nil {
    // Could be: invalid signature, expired token, malformed token, etc.
    log.Printf("JWT decode error: %v", err)
}
```

### Retry Client Errors

```go
client, err := httputil.NewRetryClient(
    httputil.WithMaxRetries(-1), // Invalid!
)
if err != nil {
    // Handle configuration error
}
```

## Complete Example

Here's a complete example showing multiple features working together:

```go
package main

import (
    "encoding/json"
    "fmt"
    "log"
    "net/http"
    "time"
    
    "ella.to/httputil"
)

type User struct {
    ID   string `json:"id"`
    Name string `json:"name"`
    Role string `json:"role"`
}

type UserClaims struct {
    *httputil.JwtRegisteredClaims
    UserID string `json:"user_id"`
    Role   string `json:"role"`
}

func (c *UserClaims) ParseToken(token *httputil.JwtToken) error {
    if !token.Valid {
        return fmt.Errorf("invalid token")
    }
    return nil
}

func main() {
    // Setup JWT
    jwtHandler := httputil.New("super-secret-key")
    
    // Setup middleware
    parseSession := func(token string) (User, error) {
        claims := &UserClaims{JwtRegisteredClaims: &httputil.JwtRegisteredClaims{}}
        err := jwtHandler.Decode(token, claims)
        if err != nil {
            return User{}, err
        }
        return User{ID: claims.UserID, Role: claims.Role}, nil
    }
    
    sessionMiddleware := httputil.WithSessionContext(
        httputil.CookieKey("session"),
        httputil.ContextKey("user"),
        httputil.ContextKey(""),
        parseSession,
    )
    
    // Setup routes
    mux := http.NewServeMux()
    
    // Login endpoint
    mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
        // Limit request size
        body, err := httputil.ReadBodyLimiter(1024, w, r)
        if err != nil {
            w.Write([]byte("Request too large"))
            return
        }
        
        var loginReq struct {
            Username string `json:"username"`
            Password string `json:"password"`
        }
        
        if err := json.Unmarshal(body, &loginReq); err != nil {
            http.Error(w, "Invalid JSON", http.StatusBadRequest)
            return
        }
        
        // Authenticate user (simplified)
        if loginReq.Username == "admin" && loginReq.Password == "secret" {
            claims := &UserClaims{
                JwtRegisteredClaims: &httputil.JwtRegisteredClaims{
                    ExpiresAt: httputil.JwtNewNumericDate(time.Now().Add(24 * time.Hour)),
                    Subject:   loginReq.Username,
                },
                UserID: "admin123",
                Role:   "admin",
            }
            
            token, err := jwtHandler.Encode(claims)
            if err != nil {
                http.Error(w, "Token generation failed", http.StatusInternalServerError)
                return
            }
            
            // Set secure cookie
            httputil.SetCookie(w, httputil.CookieKey("session"), token, 24*time.Hour, true)
            
            w.Header().Set("Content-Type", "application/json")
            json.NewEncoder(w).Encode(map[string]string{"token": token})
        } else {
            http.Error(w, "Invalid credentials", http.StatusUnauthorized)
        }
    })
    
    // Protected endpoint
    mux.HandleFunc("/profile", func(w http.ResponseWriter, r *http.Request) {
        user, ok := r.Context().Value(httputil.ContextKey("user")).(User)
        if !ok {
            http.Error(w, "Unauthorized", http.StatusUnauthorized)
            return
        }
        
        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(user)
    })
    
    // Setup development proxy for frontend
    err := httputil.DevProxy(
        mux,
        http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            http.Error(w, "API endpoint not found", http.StatusNotFound)
        }),
        true, // development mode
        "http://localhost:3000", // frontend dev server
        []string{"/api", "/login", "/profile"}, // API exceptions
    )
    if err != nil {
        log.Fatal("DevProxy setup failed:", err)
    }
    
    // Chain all middleware
    handler := httputil.Chain(
        mux,
        sessionMiddleware,
        httputil.WithLogging,
    )
    
    log.Println("Server starting on :8080")
    log.Fatal(http.ListenAndServe(":8080", handler))
}
```

## Type Safety

The library uses custom types for better type safety:

```go
type CookieKey string    // For cookie names
type ContextKey string   // For context keys
```

This prevents mixing up string parameters and provides better IDE support.

## Testing

The library includes comprehensive tests with no mocking - all tests use real HTTP servers and actual functionality. Run tests with:

```bash
go test ./...
```

## License

MIT License - see LICENSE.md for details.