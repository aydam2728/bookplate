// Package httpserver builds the HTTP server: routing, timeouts, request
// logging and the operational endpoints.
//
// It is transport only. It knows nothing about the catalogue, and nothing here
// reads the environment: the address, the logger and the readiness probe all
// arrive from the caller.
package httpserver

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// Readiness reports whether the server can serve traffic: nil when it can, the
// reason when it cannot.
type Readiness func(ctx context.Context) error

// Timeouts guard against a client that opens a connection and never finishes
// its request. Without ReadHeaderTimeout in particular, one slow client can
// hold a connection open indefinitely, which matters on a Raspberry Pi.
const (
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 30 * time.Second
	writeTimeout      = 30 * time.Second
	idleTimeout       = 120 * time.Second

	// ShutdownGrace is how long in-flight requests are given to finish.
	ShutdownGrace = 15 * time.Second
)

// New builds the server.
func New(addr string, logger *slog.Logger, ready Readiness, register ...func(*http.ServeMux)) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", handleHealthz)
	mux.HandleFunc("GET /readyz", handleReadyz(logger, ready))

	for _, add := range register {
		add(mux)
	}

	return &http.Server{
		Addr:              addr,
		Handler:           logRequests(logger, mux),
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelWarn),
	}
}

// Run serves until ctx is cancelled, then shuts down gracefully.
func Run(ctx context.Context, srv *http.Server, logger *slog.Logger) error {
	served := make(chan error, 1)
	go func() {
		logger.Info("listening", "addr", srv.Addr)
		err := srv.ListenAndServe()
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		served <- err
	}()

	select {
	case err := <-served:
		return err
	case <-ctx.Done():
		logger.Info("shutting down", "grace", ShutdownGrace.String())
		// WithoutCancel: ctx is already cancelled, and the shutdown needs its
		// own deadline rather than inheriting an expired one.
		stopCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), ShutdownGrace)
		defer cancel()
		if err := srv.Shutdown(stopCtx); err != nil {
			return fmt.Errorf("graceful shutdown: %w", err)
		}
		return <-served
	}
}

// handleHealthz answers whether the process is alive. It must stay trivial: a
// liveness probe that depends on anything will restart a healthy container.
func handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writePlain(w, http.StatusOK, "ok")
}

// handleReadyz answers whether the server can serve traffic.
//
// The reason for a failure is logged, never sent: this endpoint is reachable
// by anyone who can reach the instance, and the threat model assumes the
// instance is published on the internet.
func handleReadyz(logger *slog.Logger, ready Readiness) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if ready == nil {
			writePlain(w, http.StatusOK, "ready")
			return
		}
		if err := ready(r.Context()); err != nil {
			logger.Error("not ready", "reason", err)
			writePlain(w, http.StatusServiceUnavailable, "not ready")
			return
		}
		writePlain(w, http.StatusOK, "ready")
	}
}

func writePlain(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, _ = fmt.Fprintln(w, body)
}

// logRequests records one line per request.
//
// It logs the matched route pattern, never the raw path: a search URL carries
// book titles, and what a person reads is exactly the data this project
// promises not to spread around.
func logRequests(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(recorder, r)

		pattern := r.Pattern
		if pattern == "" {
			pattern = "(unmatched)"
		}
		logger.Info("request",
			"method", r.Method,
			"route", pattern,
			"status", recorder.status,
			"duration_ms", time.Since(started).Milliseconds(),
		)
	})
}

// statusRecorder remembers the status code so it can be logged.
type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (r *statusRecorder) WriteHeader(status int) {
	if r.wroteHeader {
		return
	}
	r.status = status
	r.wroteHeader = true
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}
	return r.ResponseWriter.Write(b)
}
