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

// devProxy is a function which modifies the mount response
// to redirect the traffic to localhost:19006 which is ui development server
// we need this because CORS cookie is not working as expected and causes
// login and logout to fail
func DevProxy(r Router, service http.Handler, isDev bool, proxyAddr string, exceptions []string) error {
	if !isDev {
		r.Mount("/", service)
		return nil
	}

	// staticFile := httputil.ServeFile(os.DirFS("./app/web-build"))
	staticFile, err := ReverseProxy(proxyAddr)
	if err != nil {
		return err
	}

	r.Mount("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
