package httpserver_test

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aydam2728/bookplate/internal/platform/httpserver"
)

func TestProbe_WhenInstanceIsAlive_Succeeds(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if err := httpserver.Probe(t.Context(), strings.TrimPrefix(srv.URL, "http://")); err != nil {
		t.Errorf("Probe failed against a healthy instance: %v", err)
	}
}

func TestProbe_WhenAddrOmitsTheHost_TargetsLoopback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	_, port, err := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatalf("splitting the test server address: %v", err)
	}

	// ":8080" is the default listen address, and the health check inside the
	// container has to turn it into something dialable.
	if err := httpserver.Probe(t.Context(), ":"+port); err != nil {
		t.Errorf("Probe could not reach a server listening on every interface: %v", err)
	}
}

func TestProbe_WhenInstanceIsUnhealthy_Fails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	err := httpserver.Probe(t.Context(), strings.TrimPrefix(srv.URL, "http://"))
	if err == nil {
		t.Fatal("Probe accepted a 503")
	}
	if !strings.Contains(err.Error(), "503") {
		t.Errorf("the error does not report the status: %v", err)
	}
}

func TestProbe_WhenNothingIsListening_Fails(t *testing.T) {
	// Port 1 is reserved and nothing listens there.
	if err := httpserver.Probe(t.Context(), "127.0.0.1:1"); err == nil {
		t.Error("Probe succeeded against a closed port")
	}
}

func TestProbe_WhenAddrIsMalformed_Fails(t *testing.T) {
	err := httpserver.Probe(t.Context(), "not-an-address")
	if err == nil {
		t.Fatal("Probe accepted a malformed address")
	}
	if !strings.Contains(err.Error(), "not-an-address") {
		t.Errorf("the error does not quote the rejected address: %v", err)
	}
}
