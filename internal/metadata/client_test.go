package metadata_test

import (
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aydam2728/bookplate/internal/metadata"
)

func TestGet_RefusesAHostThatIsNotAllowed(t *testing.T) {
	f := metadata.NewFetcher([]string{"openlibrary.org"}, "test")

	_, err := f.Get(t.Context(), "https://evil.example.org/anything")
	if !errors.Is(err, metadata.ErrBlocked) {
		t.Errorf("an unlisted host was contacted, err = %v", err)
	}
}

func TestGet_RefusesPlainHTTP(t *testing.T) {
	f := metadata.NewFetcher([]string{"openlibrary.org"}, "test")

	_, err := f.Get(t.Context(), "http://openlibrary.org/isbn/9780140328721.json")
	if !errors.Is(err, metadata.ErrBlocked) {
		t.Errorf("a plain HTTP request was allowed, err = %v", err)
	}
}

func TestGet_RefusesALoopbackAddressEvenWhenAllowed(t *testing.T) {
	// The host is on the allow-list and the scheme is right, so only the dial
	// check can stop this. That check is what defeats a provider name pointed
	// at the instance itself.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	_, port, err := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatalf("splitting the test server address: %v", err)
	}

	f := metadata.NewFetcher([]string{"127.0.0.1"}, "test")
	_, err = f.Get(t.Context(), "https://127.0.0.1:"+port+"/")
	if err == nil {
		t.Fatal("the fetcher reached a loopback address")
	}
	if !strings.Contains(err.Error(), "not a public address") {
		t.Errorf("the connection failed for an unrelated reason: %v", err)
	}
}

func TestGet_RefusesAnUnparseableURL(t *testing.T) {
	f := metadata.NewFetcher([]string{"openlibrary.org"}, "test")

	if _, err := f.Get(t.Context(), "://"); !errors.Is(err, metadata.ErrBlocked) {
		t.Errorf("an unparseable URL was accepted, err = %v", err)
	}
}
