package httputil

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestRunHTTPServerStopsOnContextCancel(t *testing.T) {
	addr := freeAddr(t)
	server := &http.Server{
		Addr: addr,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- RunHttpServer(ctx, server, 0, nil)
	}()

	waitForServerUp(t, "http://"+addr)

	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunHttpServer returned unexpected error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("RunHttpServer did not return after context cancellation")
	}

	waitForServerDown(t, "http://"+addr)
}

func TestRunHTTPServerExitsWhenServerClosedExternally(t *testing.T) {
	addr := freeAddr(t)
	server := &http.Server{
		Addr: addr,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- RunHttpServer(context.Background(), server, 0, nil)
	}()

	waitForServerUp(t, "http://"+addr)

	if err := server.Close(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		t.Fatalf("failed to close server externally: %v", err)
	}

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunHttpServer returned unexpected error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("RunHttpServer goroutine was not cleaned up after external close")
	}
}

func TestRunHTTPServerShutdownTimeout(t *testing.T) {
	requestStarted := make(chan struct{}, 1)
	releaseRequest := make(chan struct{})

	addr := freeAddr(t)
	server := &http.Server{
		Addr: addr,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/block" {
				requestStarted <- struct{}{}
				<-releaseRequest
			}
			w.WriteHeader(http.StatusOK)
		}),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- RunHttpServer(ctx, server, 100*time.Millisecond, nil)
	}()

	baseURL := "http://" + addr
	waitForServerUp(t, baseURL)

	requestErrCh := make(chan error, 1)
	go func() {
		resp, err := http.Get(baseURL + "/block")
		if err != nil {
			requestErrCh <- err
			return
		}
		resp.Body.Close()
		requestErrCh <- nil
	}()

	select {
	case <-requestStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("blocking request did not start")
	}

	cancel()

	select {
	case err := <-errCh:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("expected context deadline exceeded from shutdown timeout, got: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("RunHttpServer did not return on shutdown timeout")
	}

	close(releaseRequest)
	select {
	case <-requestErrCh:
	case <-time.After(2 * time.Second):
		t.Fatal("blocking request did not complete after release")
	}
}

func TestRunHTTPServerServesProvidedListener(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to allocate test listener: %v", err)
	}
	addr := ln.Addr().String()

	server := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- RunHttpServer(ctx, server, 0, ln)
	}()

	waitForServerUp(t, "http://"+addr)

	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunHttpServer returned unexpected error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("RunHttpServer did not return after context cancellation")
	}

	waitForServerDown(t, "http://"+addr)
}

func freeAddr(t *testing.T) string {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to allocate test listener: %v", err)
	}
	defer ln.Close()

	return ln.Addr().String()
}

func waitForServerUp(t *testing.T, url string) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil {
			resp.Body.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}

	t.Fatalf("server did not start listening on %s", url)
}

func waitForServerDown(t *testing.T, url string) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err != nil {
			return
		}
		resp.Body.Close()
		time.Sleep(20 * time.Millisecond)
	}

	t.Fatalf("server is still reachable on %s", url)
}
