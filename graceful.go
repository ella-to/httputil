package httputil

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const defaultShutdownTimeout = 10 * time.Second

// RunHttpServer starts server.ListenAndServe and blocks until the server exits.
//
// Usage:
//
//	err := RunHttpServer(ctx, srv, 5*time.Second)
//
// The function is intended to be called from main after wiring routes and middleware.
// It coordinates graceful shutdown in one place:
//   - the server is stopped when ctx is canceled (for example from signal handling
//     or a parent service shutdown),
//   - OS interrupts (SIGINT/SIGTERM) are also wired into ctx via signal.NotifyContext,
//   - if the server is closed externally, the function returns after the ListenAndServe
//     goroutine exits.
//
// shutdownTimeout limits how long Shutdown waits for in-flight requests to finish.
// If shutdownTimeout <= 0, a sensible default is used.
//
// context.WithoutCancel(ctx) is used for shutdown so cleanup can still run after ctx
// cancellation, while still preserving context values for logging/tracing.
func RunHttpServer(ctx context.Context, server *http.Server, shutdownTimeout time.Duration) error {
	if shutdownTimeout <= 0 {
		shutdownTimeout = defaultShutdownTimeout
	}

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	serverErrCh := make(chan error, 1)
	go func() {
		err := server.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrCh <- err
			return
		}
		serverErrCh <- nil
	}()

	select {
	case err := <-serverErrCh:
		return err
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	err := <-serverErrCh
	if err != nil {
		return err
	}

	return nil
}
