// Package metadata fetches bibliographic data from external providers.
//
// Everything this package sends leaves the instance, which makes it the one
// place server-side request forgery can happen. The client below is therefore
// deliberately unhelpful: it talks only to hosts on an allow-list, refuses to
// reach private addresses, will not follow a redirect off the list, and caps
// both the time and the bytes it will accept.
package metadata

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"syscall"
	"time"
)

// Limits on anything fetched from outside. A provider that hangs or answers
// with a gigabyte must not take the instance down with it.
const (
	RequestTimeout  = 10 * time.Second
	MaxResponseSize = 4 << 20 // 4 MiB, enough for a cover image

	// Attempts bounds how many times one fetch is tried. Three covers the
	// transient failures without keeping a person waiting.
	Attempts = 3

	// RetryBackoff is multiplied by the attempt number, so the waits are
	// short and increasing rather than a thundering retry.
	RetryBackoff = 400 * time.Millisecond
)

// ErrBlocked reports a request the client refused to make.
var ErrBlocked = errors.New("blocked outbound request")

// Fetcher performs the outbound requests of one provider.
type Fetcher struct {
	allowed map[string]bool
	agent   string
	client  *http.Client
}

// NewFetcher returns a Fetcher that may only reach the given hosts.
//
// userAgent names the application and a contact address. Open Library raises
// the rate limit for identified callers, and an operator who can be reached
// is an operator who gets warned rather than blocked.
func NewFetcher(hosts []string, userAgent string) *Fetcher {
	allowed := make(map[string]bool, len(hosts))
	for _, h := range hosts {
		allowed[h] = true
	}

	f := &Fetcher{allowed: allowed, agent: userAgent}
	f.client = &http.Client{
		Timeout: RequestTimeout,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout: 5 * time.Second,
				// Control sees the address actually being dialled, after the
				// name has been resolved. Checking here rather than on the
				// hostname is what defeats DNS rebinding: a name that resolves
				// to a public address on the first lookup and to 127.0.0.1 on
				// the second is still refused.
				Control: blockPrivateAddresses,
			}).DialContext,
			ForceAttemptHTTP2:   true,
			MaxIdleConnsPerHost: 2,
			IdleConnTimeout:     30 * time.Second,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return fmt.Errorf("%w: too many redirects", ErrBlocked)
			}
			if !allowed[req.URL.Hostname()] {
				return fmt.Errorf("%w: redirect to %q, which is not on the allow-list",
					ErrBlocked, req.URL.Hostname())
			}
			return nil
		},
	}
	return f
}

// Get fetches target and returns at most MaxResponseSize bytes.
func (f *Fetcher) Get(ctx context.Context, target string) ([]byte, error) {
	parsed, err := url.Parse(target)
	if err != nil {
		return nil, fmt.Errorf("%w: unparseable URL", ErrBlocked)
	}
	if parsed.Scheme != "https" {
		return nil, fmt.Errorf("%w: %q is not https", ErrBlocked, parsed.Scheme)
	}
	if !f.allowed[parsed.Hostname()] {
		return nil, fmt.Errorf("%w: %q is not on the allow-list", ErrBlocked, parsed.Hostname())
	}

	ctx, cancel := context.WithTimeout(ctx, RequestTimeout)
	defer cancel()

	// A provider that answers 503 on one call and succeeds on the next is
	// the normal state of affairs, not an incident: Google Books in
	// particular fails this way several times an hour. Without a retry a
	// bulk scan turns routine flakiness into books typed in by hand.
	return withRetry(ctx, func() ([]byte, bool, error) {
		return f.attempt(ctx, target, parsed.Host)
	})
}

// withRetry runs one fetch until it succeeds, fails finally, or runs out of
// attempts. It takes a closure rather than doing the request itself so the
// policy can be tested without a network and without relaxing the dial check
// that refuses loopback addresses.
func withRetry(ctx context.Context, once func() ([]byte, bool, error)) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt < Attempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("giving up after %d attempts: %w", attempt, ctx.Err())
			case <-time.After(RetryBackoff * time.Duration(attempt)):
			}
		}

		body, retryable, err := once()
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retryable {
			return nil, err
		}
	}
	return nil, fmt.Errorf("after %d attempts: %w", Attempts, lastErr)
}

// attempt makes one request and says whether failing again is worth trying.
// A refusal of ours and anything the provider answers in the 4xx range are
// final: retrying a bad request only wastes someone else's quota.
func (f *Fetcher) attempt(ctx context.Context, target, host string) ([]byte, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, false, fmt.Errorf("building the request: %w", err)
	}
	req.Header.Set("User-Agent", f.agent)
	req.Header.Set("Accept", "application/json")

	resp, err := f.client.Do(req)
	if err != nil {
		// A refused connection or a reset is worth one more try; a blocked
		// address is not.
		return nil, !errors.Is(err, ErrBlocked), fmt.Errorf("fetching %s: %w", host, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		retryable := resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests
		return nil, retryable, fmt.Errorf("fetching %s: status %d", host, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxResponseSize+1))
	if err != nil {
		return nil, true, fmt.Errorf("reading from %s: %w", host, err)
	}
	if len(body) > MaxResponseSize {
		return nil, false, fmt.Errorf("%w: %s sent more than %d bytes",
			ErrBlocked, host, MaxResponseSize)
	}
	return body, false, nil
}

// blockPrivateAddresses refuses to connect to anything that is not a public
// address: loopback, link-local, private ranges and the cloud metadata
// address all live there.
func blockPrivateAddresses(network, address string, _ syscall.RawConn) error {
	if network != "tcp4" && network != "tcp6" {
		return fmt.Errorf("%w: refusing network %q", ErrBlocked, network)
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("%w: unreadable address %q", ErrBlocked, address)
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf("%w: unreadable address %q", ErrBlocked, address)
	}
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return fmt.Errorf("%w: %s is not a public address", ErrBlocked, ip)
	}
	return nil
}
