package covers

import (
	"context"
	"errors"
	"fmt"
	"io"
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

// UploadCredit is the attribution of a cover the household supplied itself.
// It is a sentence rather than a host, so it cannot collide with one.
const UploadCredit = "uploaded by the household"

// MaxUpload is the largest cover the interface accepts. A re-encoded cover
// is tens of kilobytes; this leaves room for a photograph taken with a
// telephone without letting one request fill a volume.
const MaxUpload = 8 << 20

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
	mux.HandleFunc("PUT /api/covers/editions/{id}", protect(h.upload))
}

// upload stores a cover the household sent itself and points an edition at
// it.
//
// Nothing is fetched here. The bytes arrive from the browser, so there is no
// outbound request, no host to allow and no SSRF to guard against. That is
// why this exists at all: it is the only way to give a picture to a book no
// catalogue has one for, and in France that is a great many of them, because
// the BnF publishes no covers.
//
// The bytes are still decoded, bounded and re-encoded like everything else.
// A household member is not an attacker, but a browser is not a guarantee
// either, and the rule that nothing is stored as it arrived has no
// exceptions.
func (h *Handlers) upload(w http.ResponseWriter, r *http.Request) {
	caller, err := scope.FromContext(r.Context())
	if err != nil {
		httpserver.WriteError(w, nil, http.StatusUnauthorized, "sign in first", nil)
		return
	}

	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, MaxUpload))
	if err != nil {
		httpserver.WriteError(w, nil, http.StatusRequestEntityTooLarge,
			fmt.Sprintf("a cover has to be smaller than %d MB", MaxUpload>>20), nil)
		return
	}

	assetID, err := h.store.Save(r.Context(), caller, raw, "", UploadCredit)
	switch {
	case errors.Is(err, ErrNotAnImage):
		httpserver.WriteError(w, nil, http.StatusBadRequest,
			"that file is not a JPEG, a PNG or a GIF", nil)
		return
	case errors.Is(err, ErrTooLarge):
		httpserver.WriteError(w, nil, http.StatusBadRequest,
			"that image is too large to decode safely", nil)
		return
	case errors.Is(err, scope.ErrForbidden):
		httpserver.WriteError(w, nil, http.StatusForbidden,
			"your role cannot change books", nil)
		return
	case err != nil:
		httpserver.WriteError(w, h.logger, http.StatusInternalServerError,
			"could not store the cover", err)
		return
	}

	if err := h.store.Attach(r.Context(), caller, r.PathValue("id"), assetID); err != nil {
		// Same answer for an edition that does not exist and one belonging
		// to another household. The stored image is left where it is: it is
		// addressed by its own content, so nothing points at it.
		httpserver.WriteError(w, nil, http.StatusNotFound, "no such book", nil)
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, map[string]string{"coverId": assetID})
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
