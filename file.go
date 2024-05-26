package httputil

import (
	"io/fs"
	"net/http"
)

func ServeFile(fs fs.FS) http.Handler {
	fileSystem := http.FS(fs)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fs, err := fileSystem.Open(r.URL.Path)
		if err != nil {
			r.URL.Path = "/"
		} else {
			fs.Close()
		}

		http.FileServer(fileSystem).ServeHTTP(w, r)
	})
}
