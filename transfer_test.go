package httputil

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"
)

func TestStreamMultipartUploadSuccess(t *testing.T) {
	req := newMultipartRequest(t, map[string]string{"owner": "alice"}, []testMultipartFile{
		{FieldName: "files", FileName: "a.txt", ContentType: "text/plain", Content: "alpha"},
		{FieldName: "files", FileName: "b.txt", ContentType: "text/plain", Content: "beta"},
	})

	var seen []UploadedFile
	contents := map[string]string{}

	summary, err := StreamMultipartUpload(req, UploadLimits{
		MaxFileSize:   16,
		MaxFiles:      4,
		MaxTotalBytes: 64,
	}, func(file UploadedFile, content io.Reader) error {
		body, readErr := io.ReadAll(content)
		if readErr != nil {
			return readErr
		}
		file.Size = int64(len(body))
		seen = append(seen, file)
		contents[file.FileName] = string(body)
		return nil
	})
	if err != nil {
		t.Fatalf("StreamMultipartUpload() error = %v", err)
	}

	if summary.Files != 2 {
		t.Fatalf("expected 2 files, got %d", summary.Files)
	}
	if summary.TotalBytes != int64(len("alpha")+len("beta")) {
		t.Fatalf("expected total bytes 9, got %d", summary.TotalBytes)
	}
	if len(seen) != 2 {
		t.Fatalf("expected to see 2 files, got %d", len(seen))
	}
	if contents["a.txt"] != "alpha" || contents["b.txt"] != "beta" {
		t.Fatalf("unexpected file contents: %#v", contents)
	}
}

func TestStreamMultipartUploadLimitExceeded(t *testing.T) {
	t.Run("max files", func(t *testing.T) {
		req := newMultipartRequest(t, nil, []testMultipartFile{
			{FieldName: "files", FileName: "a.txt", ContentType: "text/plain", Content: "a"},
			{FieldName: "files", FileName: "b.txt", ContentType: "text/plain", Content: "b"},
		})

		_, err := StreamMultipartUpload(req, UploadLimits{MaxFiles: 1}, func(file UploadedFile, content io.Reader) error {
			_, _ = io.Copy(io.Discard, content)
			return nil
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		var limitErr *UploadLimitError
		if !errors.As(err, &limitErr) {
			t.Fatalf("expected UploadLimitError, got %T", err)
		}
		if limitErr.LimitType != "files" {
			t.Fatalf("expected files limit error, got %s", limitErr.LimitType)
		}
	})

	t.Run("max file bytes", func(t *testing.T) {
		req := newMultipartRequest(t, nil, []testMultipartFile{
			{FieldName: "files", FileName: "big.txt", ContentType: "text/plain", Content: "123456"},
		})

		_, err := StreamMultipartUpload(req, UploadLimits{MaxFileSize: 4}, func(file UploadedFile, content io.Reader) error {
			_, readErr := io.Copy(io.Discard, content)
			return readErr
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		var limitErr *UploadLimitError
		if !errors.As(err, &limitErr) {
			t.Fatalf("expected UploadLimitError, got %T", err)
		}
		if limitErr.LimitType != "file-bytes" {
			t.Fatalf("expected file-bytes limit error, got %s", limitErr.LimitType)
		}
	})

	t.Run("max total bytes", func(t *testing.T) {
		req := newMultipartRequest(t, nil, []testMultipartFile{
			{FieldName: "files", FileName: "a.txt", ContentType: "text/plain", Content: "abcd"},
			{FieldName: "files", FileName: "b.txt", ContentType: "text/plain", Content: "efgh"},
		})

		_, err := StreamMultipartUpload(req, UploadLimits{MaxTotalBytes: 6}, func(file UploadedFile, content io.Reader) error {
			_, readErr := io.Copy(io.Discard, content)
			return readErr
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		var limitErr *UploadLimitError
		if !errors.As(err, &limitErr) {
			t.Fatalf("expected UploadLimitError, got %T", err)
		}
		if limitErr.LimitType != "total-bytes" {
			t.Fatalf("expected total-bytes limit error, got %s", limitErr.LimitType)
		}
	})
}

func TestStreamMultipartUploadRequiresMultipart(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/upload", strings.NewReader("nope"))
	req.Header.Set("Content-Type", "text/plain")

	_, err := StreamMultipartUpload(req, UploadLimits{}, func(file UploadedFile, content io.Reader) error {
		return nil
	})
	if !errors.Is(err, ErrMultipartRequired) {
		t.Fatalf("expected ErrMultipartRequired, got %v", err)
	}
}

func TestNewMultipartUploadRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		summary, err := StreamMultipartUpload(r, UploadLimits{
			MaxFileSize:   8,
			MaxFiles:      2,
			MaxTotalBytes: 16,
		}, func(file UploadedFile, content io.Reader) error {
			_, readErr := io.Copy(io.Discard, content)
			return readErr
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		_, _ = fmt.Fprintf(w, "%d:%d", summary.Files, summary.TotalBytes)
	}))
	defer server.Close()

	req, err := NewMultipartUploadRequest(context.Background(), http.MethodPost, server.URL, map[string]string{
		"owner": "alice",
	}, []UploadRequestFile{
		{
			FieldName:   "files",
			FileName:    "a.txt",
			ContentType: "text/plain",
			Reader:      strings.NewReader("hello"),
			Size:        5,
		},
		{
			FieldName:   "files",
			FileName:    "b.txt",
			ContentType: "text/plain",
			Reader:      strings.NewReader("go"),
			Size:        2,
		},
	}, UploadLimits{
		MaxFileSize:   8,
		MaxFiles:      2,
		MaxTotalBytes: 16,
	})
	if err != nil {
		t.Fatalf("NewMultipartUploadRequest() error = %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d (%s)", resp.StatusCode, string(body))
	}
	if string(body) != "2:7" {
		t.Fatalf("unexpected server summary: %s", string(body))
	}
}

func TestNewMultipartUploadRequestLimits(t *testing.T) {
	t.Run("pre validation known size", func(t *testing.T) {
		_, err := NewMultipartUploadRequest(context.Background(), http.MethodPost, "http://example.com", nil, []UploadRequestFile{
			{
				FieldName: "files",
				FileName:  "too-big.txt",
				Reader:    strings.NewReader("12345"),
				Size:      5,
			},
		}, UploadLimits{MaxFileSize: 4})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		var limitErr *UploadLimitError
		if !errors.As(err, &limitErr) {
			t.Fatalf("expected UploadLimitError, got %T", err)
		}
	})

	t.Run("streaming validation unknown size", func(t *testing.T) {
		req, err := NewMultipartUploadRequest(context.Background(), http.MethodPost, "http://example.com", nil, []UploadRequestFile{
			{
				FieldName: "files",
				FileName:  "unknown.txt",
				Reader:    strings.NewReader("12345"),
				Size:      -1,
			},
		}, UploadLimits{MaxFileSize: 4})
		if err != nil {
			t.Fatalf("NewMultipartUploadRequest() error = %v", err)
		}

		_, readErr := io.ReadAll(req.Body)
		if readErr == nil {
			t.Fatal("expected streaming error, got nil")
		}

		var limitErr *UploadLimitError
		if !errors.As(readErr, &limitErr) {
			t.Fatalf("expected UploadLimitError, got %T (%v)", readErr, readErr)
		}
	})
}

func TestServeDownload(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			err := ServeDownload(w, r, func(req *http.Request) (DownloadSource, error) {
				if req.URL.Query().Get("name") != "report" {
					return DownloadSource{}, errors.New("unexpected query")
				}
				return DownloadSource{
					Name:        "report.txt",
					ContentType: "text/plain",
					Size:        int64(len("hello")),
					Reader:      io.NopCloser(strings.NewReader("hello")),
				}, nil
			})
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		})

		req := httptest.NewRequest(http.MethodGet, "/download?name=report", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
		if got := w.Header().Get("Content-Type"); got != "text/plain" {
			t.Fatalf("unexpected content type: %q", got)
		}
		if got := w.Header().Get("Content-Disposition"); !strings.Contains(got, "filename=report.txt") {
			t.Fatalf("unexpected content disposition: %q", got)
		}
		if got := w.Header().Get("Content-Length"); got != "5" {
			t.Fatalf("unexpected content length: %q", got)
		}
		if w.Body.String() != "hello" {
			t.Fatalf("unexpected body: %q", w.Body.String())
		}
	})

	t.Run("provider error", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/download", nil)
		w := httptest.NewRecorder()

		err := ServeDownload(w, req, func(req *http.Request) (DownloadSource, error) {
			return DownloadSource{}, errors.New("not found")
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestParseDownloadMetadata(t *testing.T) {
	headers := make(http.Header)
	headers.Set("Content-Type", "application/pdf")
	headers.Set("Content-Length", "123")
	headers.Set("Content-Disposition", `attachment; filename="doc.pdf"`)

	meta := ParseDownloadMetadata(headers)
	if meta.ContentType != "application/pdf" {
		t.Fatalf("unexpected content type: %s", meta.ContentType)
	}
	if meta.Size != 123 {
		t.Fatalf("unexpected size: %d", meta.Size)
	}
	if meta.FileName != "doc.pdf" {
		t.Fatalf("unexpected filename: %s", meta.FileName)
	}
}

func TestDownload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/err" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", `attachment; filename="data.json"`)
		w.Header().Set("Content-Length", "12")
		_, _ = w.Write([]byte(`{"ok": true}`))
	}))
	defer server.Close()

	t.Run("success", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, server.URL, nil)
		if err != nil {
			t.Fatalf("NewRequest() error = %v", err)
		}

		resp, err := Download(nil, req)
		if err != nil {
			t.Fatalf("Download() error = %v", err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("ReadAll() error = %v", err)
		}

		if string(body) != `{"ok": true}` {
			t.Fatalf("unexpected body: %s", string(body))
		}
		if resp.Metadata.FileName != "data.json" {
			t.Fatalf("unexpected filename: %s", resp.Metadata.FileName)
		}
		if resp.Metadata.ContentType != "application/json" {
			t.Fatalf("unexpected content type: %s", resp.Metadata.ContentType)
		}
		if resp.Metadata.Size != 12 {
			t.Fatalf("unexpected size: %d", resp.Metadata.Size)
		}
	})

	t.Run("http error status", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, server.URL+"/err", nil)
		if err != nil {
			t.Fatalf("NewRequest() error = %v", err)
		}

		_, err = Download(nil, req)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "status 400") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

type testMultipartFile struct {
	FieldName   string
	FileName    string
	ContentType string
	Content     string
}

func newMultipartRequest(t *testing.T, fields map[string]string, files []testMultipartFile) *http.Request {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	for k, v := range fields {
		if err := writer.WriteField(k, v); err != nil {
			t.Fatalf("WriteField() error = %v", err)
		}
	}

	for _, file := range files {
		header := make(textproto.MIMEHeader)
		header.Set("Content-Disposition", fmt.Sprintf(`form-data; name=%q; filename=%q`, file.FieldName, file.FileName))
		header.Set("Content-Type", file.ContentType)

		part, err := writer.CreatePart(header)
		if err != nil {
			t.Fatalf("CreatePart() error = %v", err)
		}

		if _, err = io.Copy(part, strings.NewReader(file.Content)); err != nil {
			t.Fatalf("Copy() error = %v", err)
		}
	}

	if err := writer.Close(); err != nil {
		t.Fatalf("writer.Close() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/upload", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}
