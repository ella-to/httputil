package httputil

import (
	"io"
	"net/http"
)

func ReadLimter(size int64, w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, size)
}

func ReadBodyLimiter(size int64, w http.ResponseWriter, r *http.Request) ([]byte, error) {
	ReadLimter(size, w, r)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return nil, err
	}
	return body, nil
}
