package httpserver_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aydam2728/bookplate/internal/platform/httpserver"
)

// newServer builds a server whose logs are captured, so a test can assert on
// what was written without polluting the test output.
func newServer(t *testing.T, ready httpserver.Readiness) (http.Handler, *bytes.Buffer) {
	t.Helper()
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return httpserver.New(":0", logger, ready).Handler, &logs
}

func get(t *testing.T, h http.Handler, path string) *http.Response {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec.Result()
}

func body(t *testing.T, resp *http.Response) string {
	t.Helper()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading the body: %v", err)
	}
	return strings.TrimSpace(string(b))
}

func TestHealthz_WhenDependenciesAreBroken_StillReportsAlive(t *testing.T) {
	broken := func(context.Context) error { return errors.New("database is gone") }
	handler, _ := newServer(t, broken)

	resp := get(t, handler, "/healthz")

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d: a liveness probe that depends on anything "+
			"restarts a healthy container", resp.StatusCode, http.StatusOK)
	}
	if got := body(t, resp); got != "ok" {
		t.Errorf("body = %q, want %q", got, "ok")
	}
}

func TestReadyz_WhenDependenciesAreUsable_ReportsReady(t *testing.T) {
	handler, _ := newServer(t, func(context.Context) error { return nil })

	resp := get(t, handler, "/readyz")

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestReadyz_WhenNoProbeIsConfigured_ReportsReady(t *testing.T) {
	handler, _ := newServer(t, nil)

	if resp := get(t, handler, "/readyz"); resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestReadyz_WhenADependencyFails_RefusesTraffic(t *testing.T) {
	handler, _ := newServer(t, func(context.Context) error {
		return errors.New("database is gone")
	})

	resp := get(t, handler, "/readyz")

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusServiceUnavailable)
	}
}

func TestReadyz_WhenADependencyFails_DoesNotLeakTheReason(t *testing.T) {
	const secret = "dial tcp 10.0.0.5:5432: connection refused"
	handler, logs := newServer(t, func(context.Context) error { return errors.New(secret) })

	resp := get(t, handler, "/readyz")

	if got := body(t, resp); strings.Contains(got, secret) {
		t.Errorf("the response leaks the failure reason to anyone who can reach "+
			"the instance: %q", got)
	}
	if !strings.Contains(logs.String(), secret) {
		t.Error("the reason was hidden from the response but not written to the " +
			"logs, so the operator has no way to diagnose it")
	}
}

func TestUnknownRoute_IsNotFound(t *testing.T) {
	handler, _ := newServer(t, nil)

	if resp := get(t, handler, "/does-not-exist"); resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

func TestRequestLog_RecordsTheRouteNotTheRawPath(t *testing.T) {
	handler, logs := newServer(t, nil)

	get(t, handler, "/readyz?q=the+anarchist+cookbook")

	written := logs.String()
	if strings.Contains(written, "anarchist") {
		t.Errorf("the query string reached the logs; what a person reads must not "+
			"be recorded there: %s", written)
	}
	if !strings.Contains(written, "/readyz") {
		t.Errorf("the matched route is missing from the log line: %s", written)
	}
}

func TestNew_SetsAReadHeaderTimeout(t *testing.T) {
	srv := httpserver.New(":0", slog.New(slog.DiscardHandler), nil)

	if srv.ReadHeaderTimeout == 0 {
		t.Error("ReadHeaderTimeout is unset: one slow client can hold a connection " +
			"open indefinitely")
	}
}

func TestRun_WhenContextIsCancelled_ShutsDownCleanly(t *testing.T) {
	srv := httpserver.New("127.0.0.1:0", slog.New(slog.DiscardHandler), nil)
	ctx, cancel := context.WithCancel(t.Context())

	done := make(chan error, 1)
	go func() { done <- httpserver.Run(ctx, srv, slog.New(slog.DiscardHandler)) }()

	cancel()

	if err := <-done; err != nil {
		t.Errorf("Run returned %v, want a clean shutdown", err)
	}
}
