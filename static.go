package httputil

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
)

type ContentConfig struct {
	Dev            bool
	WebAddr        string
	APIHandler     http.Handler
	APIPrefixPaths []string
	Intercept      func(w http.ResponseWriter, r *http.Request, isExist func(path string) bool) bool
	Files          fs.FS
}

func NewContentHandler(ctx context.Context, config ContentConfig) (http.Handler, error) {
	if config.Dev {
		slog.InfoContext(ctx, "serving web ui from dev server", "addr", config.WebAddr)
		mux := http.NewServeMux()
		err := DevProxy(
			mux,
			config.APIHandler,
			config.Dev,
			config.WebAddr,
			config.APIPrefixPaths,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create dev proxy: %w", err)
		}

		return mux, nil
	}

	contentStatic, _ := fs.Sub(config.Files, "dist")
	fileExistenceCache, sizeCache := buildStaticCaches(contentStatic)
	fileServer := gzipFileServer(sizeCache, ServeFile(contentStatic))

	isFileExists := func(path string) bool {
		_, ok := fileExistenceCache[path]
		return ok
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, prefix := range config.APIPrefixPaths {
			if strings.HasPrefix(r.URL.Path, prefix) {
				config.APIHandler.ServeHTTP(w, r)
				return
			}
		}

		// just normalize the url path
		if r.URL.Path == "" {
			r.URL.Path = "/"
		}

		if config.Intercept != nil && config.Intercept(w, r, isFileExists) {
			return
		}

		fileServer.ServeHTTP(w, r)
	}), nil
}

func buildStaticCaches(staticFS fs.FS) (map[string]struct{}, map[string][]int) {
	fileExistenceCache := make(map[string]struct{})
	fileSizeByPath := make(map[string]int)

	err := fs.WalkDir(staticFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}

		info, infoErr := d.Info()
		if infoErr != nil {
			return nil
		}

		fileExistenceCache[path] = struct{}{}
		fileSizeByPath[path] = int(info.Size())
		return nil
	})
	if err != nil {
		slog.Warn("failed to precache static assets", "err", err)
	}

	sizeCache := make(map[string][]int)
	for path, compressedSize := range fileSizeByPath {
		originalPath, isCompressed := originalPathFromCompressed(path)
		if !isCompressed {
			continue
		}

		originalSize, ok := fileSizeByPath[originalPath]
		if !ok {
			continue
		}

		sizeCache[path] = []int{compressedSize, originalSize}
	}

	return fileExistenceCache, sizeCache
}

func originalPathFromCompressed(path string) (string, bool) {
	switch {
	case strings.HasSuffix(path, ".br"):
		return strings.TrimSuffix(path, ".br"), true
	case strings.HasSuffix(path, ".zst"):
		return strings.TrimSuffix(path, ".zst"), true
	case strings.HasSuffix(path, ".gz"):
		return strings.TrimSuffix(path, ".gz"), true
	case strings.HasSuffix(path, ".deflate"):
		return strings.TrimSuffix(path, ".deflate"), true
	default:
		return "", false
	}
}

func gzipFileServer(sizeCache map[string][]int, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		compressionExt, isCompressionSupported := getCompressionType(r)

		// Check if the client supports gzip encoding
		if !isCompressionSupported {
			// Fallback to the regular file server
			next.ServeHTTP(w, r)
			return
		}

		// Construct the compressed file path and ensure it exists first.
		compressionPath := r.URL.Path + "." + compressionExt
		compressedFilePath := strings.TrimPrefix(compressionPath, "/")

		sizes, err := getSize(sizeCache, compressedFilePath)
		if err != nil {
			slog.Debug("compressed file not found, falling back to uncompressed", "path", compressedFilePath)
			next.ServeHTTP(w, r)
			return
		}

		w.Header().Set("Content-Encoding", compressionExt)
		w.Header().Set("Vary", "Accept-Encoding")
		w.Header().Set("Content-Length", strconv.Itoa(sizes[0]))
		w.Header().Set("X-Uncompressed-Content-Length", strconv.Itoa(sizes[1]))

		// Determine the original content type based on the file extension
		ext := filepath.Ext(r.URL.Path)
		contentType := mimeTypeByExtenstion(ext)
		if contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}

		r.URL.Path = compressionPath

		next.ServeHTTP(w, r)
	})
}

func getSize(sizeCache map[string][]int, compressedFilePath string) ([]int, error) {
	sizes, ok := sizeCache[compressedFilePath]
	if !ok {
		return nil, fs.ErrNotExist
	}

	return sizes, nil
}

func getCompressionType(r *http.Request) (string, bool) {
	// gzip, deflate, br, zstd

	acceptEncoding := r.Header.Get("Accept-Encoding")
	if strings.Contains(acceptEncoding, "br") {
		return "br", true
	} else if strings.Contains(acceptEncoding, "zstd") {
		return "zst", true
	} else if strings.Contains(acceptEncoding, "gzip") {
		return "gz", true
	} else if strings.Contains(acceptEncoding, "deflate") {
		return "deflate", true
	}

	return "", false
}

func mimeTypeByExtenstion(ext string) string {
	// return mime.TypeByExtension(ext)
	switch ext {
	case ".html":
		return "text/html"
	case ".css":
		return "text/css"
	case ".js":
		return "application/javascript"
	case ".json":
		return "application/json"
	case ".xml":
		return "application/xml"
	case ".pdf":
		return "application/pdf"
	case ".zip":
		return "application/zip"
	case ".tar":
		return "application/x-tar"
	case ".gif":
		return "image/gif"
	case ".jpeg", ".jpg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".svg":
		return "image/svg+xml"
	case ".mp4":
		return "video/mp4"
	case ".webm":
		return "video/webm"
	case ".mp3":
		return "audio/mpeg"
	case ".wav":
		return "audio/wav"
	case ".ogg":
		return "audio/ogg"
	case ".woff":
		return "font/woff"
	case ".woff2":
		return "font/woff2"
	case ".wasm":
		return "application/wasm"
	case ".ico":
		return "image/x-icon"
	case ".txt":
		return "text/plain"
	default:
		return "application/octet-stream"
	}
}
