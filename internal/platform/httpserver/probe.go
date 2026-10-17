package httpserver

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"
)

// ProbeTimeout bounds a health probe. A container health check that hangs is
// worse than one that fails.
const ProbeTimeout = 3 * time.Second

// Probe asks a running instance whether it is alive.
//
// It exists so the container image can declare a HEALTHCHECK without shipping
// a shell, curl or wget: the binary checks itself, and the image keeps no
// tooling an attacker could reuse.
func Probe(ctx context.Context, addr string) error {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("probe address %q: %w", addr, err)
	}
	// The server listens on every interface; reach it over the loopback.
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}

	ctx, cancel := context.WithTimeout(ctx, ProbeTimeout)
	defer cancel()

	url := "http://" + net.JoinHostPort(host, port) + "/healthz"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("probe request: %w", err)
	}

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return fmt.Errorf("probe %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("probe %s: status %d", url, resp.StatusCode)
	}
	return nil
}
