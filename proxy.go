package httputil

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

func ReverseProxy(rawURL string) (http.HandlerFunc, error) {
	remote, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	handler := func(p *httputil.ReverseProxy) func(http.ResponseWriter, *http.Request) {
		return func(w http.ResponseWriter, r *http.Request) {
			r.Host = remote.Host
			p.ServeHTTP(w, r)
		}
	}

	proxy := httputil.NewSingleHostReverseProxy(remote)
	return handler(proxy), nil
}

// DevProxy is for serving static files in development mode.
// mainly used for UI development with a separate server.
func DevProxy(mux *http.ServeMux, service http.Handler, isDev bool, proxyAddr string, exceptions []string) error {
	if !isDev {
		mux.Handle("/", service)
		return nil
	}

	// staticFile := httputil.ServeFile(os.DirFS("./app/web-build"))
	staticFile, err := ReverseProxy(proxyAddr)
	if err != nil {
		return err
	}

	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, e := range exceptions {
			if strings.HasPrefix(r.URL.Path, e) {
				service.ServeHTTP(w, r)
				return
			}
		}

		staticFile.ServeHTTP(w, r)
	}))

	return nil
}
