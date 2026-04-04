package httputil

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strconv"
	"strings"
)

var ErrMultipartRequired = errors.New("request must be multipart/form-data")

// UploadLimits controls multipart upload limits.
// A zero value for each field means "no limit".
type UploadLimits struct {
	MaxFileSize   int64
	MaxFiles      int
	MaxTotalBytes int64
}

// UploadLimitError is returned when one of the upload limits is exceeded.
type UploadLimitError struct {
	LimitType string
	Limit     int64
	Current   int64
}

func (e *UploadLimitError) Error() string {
	return fmt.Sprintf("upload limit exceeded: %s (limit=%d current=%d)", e.LimitType, e.Limit, e.Current)
}

// UploadedFile contains metadata for a streamed multipart file.
type UploadedFile struct {
	FieldName   string
	FileName    string
	ContentType string
	Header      textproto.MIMEHeader
	Size        int64
}

// UploadSummary contains aggregate upload stats.
type UploadSummary struct {
	Files      int
	TotalBytes int64
}

// StreamMultipartUpload streams files from a multipart/form-data request.
// It does not buffer full files in memory and enforces limits while streaming.
func StreamMultipartUpload(r *http.Request, limits UploadLimits, onFile func(file UploadedFile, content io.Reader) error) (UploadSummary, error) {
	if onFile == nil {
		return UploadSummary{}, errors.New("onFile callback is required")
	}

	contentType := r.Header.Get("Content-Type")
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil || mediaType != "multipart/form-data" {
		return UploadSummary{}, ErrMultipartRequired
	}

	mr, err := r.MultipartReader()
	if err != nil {
		return UploadSummary{}, err
	}

	summary := UploadSummary{}

	for {
		part, nextErr := mr.NextPart()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			return summary, nextErr
		}

		if part.FileName() == "" {
			_, _ = io.Copy(io.Discard, part)
			part.Close()
			continue
		}

		nextFileCount := summary.Files + 1
		if limits.MaxFiles > 0 && nextFileCount > limits.MaxFiles {
			part.Close()
			return summary, &UploadLimitError{
				LimitType: "files",
				Limit:     int64(limits.MaxFiles),
				Current:   int64(nextFileCount),
			}
		}

		reader := &limitedCountingReader{
			r:            part,
			maxFileBytes: limits.MaxFileSize,
			maxTotal:     limits.MaxTotalBytes,
			totalRead:    &summary.TotalBytes,
		}

		meta := UploadedFile{
			FieldName:   part.FormName(),
			FileName:    part.FileName(),
			ContentType: part.Header.Get("Content-Type"),
			Header:      part.Header,
		}
		if meta.ContentType == "" {
			meta.ContentType = "application/octet-stream"
		}

		if err = onFile(meta, reader); err != nil {
			part.Close()
			return summary, err
		}

		_, err = io.Copy(io.Discard, reader)
		if err != nil {
			part.Close()
			return summary, err
		}

		meta.Size = reader.fileRead
		summary.Files++

		part.Close()
	}

	return summary, nil
}

// UploadRequestFile describes one file for multipart upload requests.
type UploadRequestFile struct {
	FieldName   string
	FileName    string
	ContentType string
	Reader      io.Reader
	// Size can be set to a non-negative value for strict pre-validation.
	// Use -1 if unknown.
	Size int64
}

// NewMultipartUploadRequest builds a streaming multipart request.
// Limits are enforced before send (for known sizes) and while streaming.
func NewMultipartUploadRequest(ctx context.Context, method string, url string, fields map[string]string, files []UploadRequestFile, limits UploadLimits) (*http.Request, error) {
	if method == "" {
		method = http.MethodPost
	}

	if limits.MaxFiles > 0 && len(files) > limits.MaxFiles {
		return nil, &UploadLimitError{
			LimitType: "files",
			Limit:     int64(limits.MaxFiles),
			Current:   int64(len(files)),
		}
	}

	var knownTotal int64
	for i, file := range files {
		if file.FieldName == "" {
			return nil, fmt.Errorf("file %d: FieldName is required", i)
		}
		if file.FileName == "" {
			return nil, fmt.Errorf("file %d: FileName is required", i)
		}
		if file.Reader == nil {
			return nil, fmt.Errorf("file %d: Reader is required", i)
		}
		if file.Size >= 0 {
			if limits.MaxFileSize > 0 && file.Size > limits.MaxFileSize {
				return nil, &UploadLimitError{
					LimitType: "file-bytes",
					Limit:     limits.MaxFileSize,
					Current:   file.Size,
				}
			}
			knownTotal += file.Size
		}
	}

	if limits.MaxTotalBytes > 0 && knownTotal > limits.MaxTotalBytes {
		return nil, &UploadLimitError{
			LimitType: "total-bytes",
			Limit:     limits.MaxTotalBytes,
			Current:   knownTotal,
		}
	}

	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)

	request, err := http.NewRequestWithContext(ctx, method, url, pr)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", mw.FormDataContentType())

	go func() {
		var total int64
		for key, value := range fields {
			if writeErr := mw.WriteField(key, value); writeErr != nil {
				_ = pw.CloseWithError(writeErr)
				return
			}
		}

		for _, file := range files {
			header := make(textproto.MIMEHeader)
			header.Set("Content-Disposition", fmt.Sprintf(`form-data; name=%q; filename=%q`, file.FieldName, file.FileName))
			if file.ContentType != "" {
				header.Set("Content-Type", file.ContentType)
			} else {
				header.Set("Content-Type", "application/octet-stream")
			}

			part, createErr := mw.CreatePart(header)
			if createErr != nil {
				_ = pw.CloseWithError(createErr)
				return
			}

			reader := &limitedCountingReader{
				r:            file.Reader,
				maxFileBytes: limits.MaxFileSize,
				maxTotal:     limits.MaxTotalBytes,
				totalRead:    &total,
			}

			if _, copyErr := io.Copy(part, reader); copyErr != nil {
				_ = pw.CloseWithError(copyErr)
				return
			}
		}

		if closeErr := mw.Close(); closeErr != nil {
			_ = pw.CloseWithError(closeErr)
			return
		}

		_ = pw.Close()
	}()

	return request, nil
}

// DownloadSource describes a server-side download stream.
type DownloadSource struct {
	Name        string
	ContentType string
	Size        int64
	Reader      io.ReadCloser
}

// ServeDownload writes a streamed file download response.
// The provider can inspect the request and return the download stream.
func ServeDownload(w http.ResponseWriter, r *http.Request, provider func(r *http.Request) (DownloadSource, error)) error {
	if provider == nil {
		return errors.New("provider is required")
	}

	source, err := provider(r)
	if err != nil {
		return err
	}
	if source.Reader == nil {
		return errors.New("provider returned nil Reader")
	}
	defer source.Reader.Close()

	if source.ContentType == "" {
		source.ContentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", source.ContentType)

	if source.Name != "" {
		disposition := mime.FormatMediaType("attachment", map[string]string{"filename": source.Name})
		if disposition == "" {
			disposition = fmt.Sprintf(`attachment; filename=%q`, source.Name)
		}
		w.Header().Set("Content-Disposition", disposition)
	}

	if source.Size >= 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(source.Size, 10))
	}

	_, err = io.Copy(w, source.Reader)
	return err
}

// DownloadMetadata contains parsed metadata from a download response.
type DownloadMetadata struct {
	FileName    string
	ContentType string
	Size        int64
}

// ParseDownloadMetadata extracts name/content-type/size from HTTP headers.
func ParseDownloadMetadata(header http.Header) DownloadMetadata {
	meta := DownloadMetadata{Size: -1}

	meta.ContentType = header.Get("Content-Type")
	if meta.ContentType == "" {
		meta.ContentType = "application/octet-stream"
	}

	if contentLength := strings.TrimSpace(header.Get("Content-Length")); contentLength != "" {
		if size, err := strconv.ParseInt(contentLength, 10, 64); err == nil {
			meta.Size = size
		}
	}

	if raw := header.Get("Content-Disposition"); raw != "" {
		_, params, err := mime.ParseMediaType(raw)
		if err == nil {
			if filename, ok := params["filename*"]; ok {
				meta.FileName = filename
			} else if filename, ok := params["filename"]; ok {
				meta.FileName = filename
			}
		}
	}

	return meta
}

// DownloadResponse is a client-side download response wrapper.
type DownloadResponse struct {
	Metadata   DownloadMetadata
	Body       io.ReadCloser
	StatusCode int
	Header     http.Header
}

// Download executes request and parses common download headers.
func Download(client *http.Client, req *http.Request) (*DownloadResponse, error) {
	if req == nil {
		return nil, errors.New("request is required")
	}
	if client == nil {
		client = http.DefaultClient
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= http.StatusBadRequest {
		defer resp.Body.Close()
		return nil, fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	return &DownloadResponse{
		Metadata:   ParseDownloadMetadata(resp.Header),
		Body:       resp.Body,
		StatusCode: resp.StatusCode,
		Header:     resp.Header,
	}, nil
}

type limitedCountingReader struct {
	r            io.Reader
	maxFileBytes int64
	maxTotal     int64
	fileRead     int64
	totalRead    *int64
}

func (r *limitedCountingReader) Read(p []byte) (int, error) {
	n, err := r.r.Read(p)
	if n > 0 {
		r.fileRead += int64(n)
		*r.totalRead += int64(n)

		if r.maxFileBytes > 0 && r.fileRead > r.maxFileBytes {
			return n, &UploadLimitError{
				LimitType: "file-bytes",
				Limit:     r.maxFileBytes,
				Current:   r.fileRead,
			}
		}

		if r.maxTotal > 0 && *r.totalRead > r.maxTotal {
			return n, &UploadLimitError{
				LimitType: "total-bytes",
				Limit:     r.maxTotal,
				Current:   *r.totalRead,
			}
		}
	}

	return n, err
}
