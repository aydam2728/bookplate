package covers

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/aydam2728/bookplate/internal/platform/httpserver"
	"github.com/aydam2728/bookplate/internal/platform/scope"
)

// CoversHost is the only host cover images may be fetched from. It is
// separate from the catalogue host and has to be allowed explicitly.
const CoversHost = "covers.openlibrary.org"

// credits names the source of a cover for the attribution stored beside it.
//
// The host is the evidence. It is what the outbound allow-list let through,
// so it cannot be whatever a form claimed, and the picture no longer always
// comes from the catalogue that described the book: a BnF record is
// completed with somebody else's cover.
//
// The metadata package names the same hosts as constants. Two strings are
// repeated here rather than making storage depend on the readers above it.
var credits = map[string]string{
	CoversHost:         "Open Library",
	"books.google.com": "Google Books",
	"inventaire.io":    "Inventaire",
}

// Credit names where a cover came from, for the attribution a stored cover
// carries. A host nobody has named is credited by its own name rather than
// guessed at: a plain credit is better than a wrong one.
func Credit(target string) string {
	parsed, err := url.Parse(target)
	if err != nil || parsed.Hostname() == "" {
		return "unknown"
	}
	if credit, ok := credits[parsed.Hostname()]; ok {
		return credit
	}
	return parsed.Hostname()
}

// fetcher is the one thing this package needs from the network, which lets a
// test drive it without any network at all.
type fetcher interface {
	Get(ctx context.Context, target string) ([]byte, error)
}

// Service fetches a cover and attaches it to an edition.
type Service struct {
	fetch  fetcher
	store  *Store
	logger *slog.Logger
}

// NewService returns a Service.
func NewService(f fetcher, store *Store, logger *slog.Logger) *Service {
	return &Service{fetch: f, store: store, logger: logger}
}

// AttachFromURL downloads a cover and points the edition at it.
//
// A failure here is never fatal to the book being added: a catalogue entry
// without a picture is still a catalogue entry, and the person is not asked
// to do it again.
func (s *Service) AttachFromURL(ctx context.Context, caller scope.Caller, editionID, url string) error {
	if url == "" {
		return nil
	}
	raw, err := s.fetch.Get(ctx, url)
	if err != nil {
		return fmt.Errorf("fetching the cover: %w", err)
	}

	assetID, err := s.store.Save(ctx, caller, raw, url, Credit(url))
	if err != nil {
		return err
	}
	return s.store.Attach(ctx, caller, editionID, assetID)
}

// Handlers serves stored covers.
type Handlers struct {
	store  *Store
	logger *slog.Logger
}

// NewHandlers returns handlers over store.
func NewHandlers(store *Store, logger *slog.Logger) *Handlers {
	return &Handlers{store: store, logger: logger}
}

// Register adds the cover route to mux, behind the given guard.
func (h *Handlers) Register(mux *http.ServeMux, protect func(http.HandlerFunc) http.HandlerFunc) {
	mux.HandleFunc("GET /api/covers/{id}", protect(h.serve))
}

func (h *Handlers) serve(w http.ResponseWriter, r *http.Request) {
	caller, err := scope.FromContext(r.Context())
	if err != nil {
		httpserver.WriteError(w, nil, http.StatusUnauthorized, "sign in first", nil)
		return
	}

	data, mime, err := h.store.Read(r.Context(), caller, r.PathValue("id"))
	if err != nil {
		// One answer for a cover that does not exist and one that belongs to
		// another household: the difference would confirm it exists.
		httpserver.WriteError(w, nil, http.StatusNotFound, "no such cover", nil)
		return
	}

	w.Header().Set("Content-Type", mime)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// The bytes are addressed by a content hash, so they never change.
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	_, _ = w.Write(data)
}
