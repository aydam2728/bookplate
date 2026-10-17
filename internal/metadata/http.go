package metadata

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/aydam2728/bookplate/internal/platform/httpserver"
)

// Handlers serves the lookup endpoint.
type Handlers struct {
	chain     Chain
	normalise func(string) (string, error)
	logger    *slog.Logger
}

// NewHandlers returns handlers over chain. normalise is injected so this
// package does not depend on the catalogue, which owns ISBN parsing.
func NewHandlers(chain Chain, normalise func(string) (string, error), logger *slog.Logger) *Handlers {
	return &Handlers{chain: chain, normalise: normalise, logger: logger}
}

// Register adds the lookup route to mux, behind the given guard.
func (h *Handlers) Register(mux *http.ServeMux, protect func(http.HandlerFunc) http.HandlerFunc) {
	mux.HandleFunc("GET /api/lookup", protect(h.lookup))
}

func (h *Handlers) lookup(w http.ResponseWriter, r *http.Request) {
	// The identifier is validated locally before anything leaves the
	// instance: a mistyped ISBN should cost nobody a round trip.
	isbn13, err := h.normalise(r.URL.Query().Get("isbn"))
	if err != nil {
		httpserver.WriteError(w, nil, http.StatusBadRequest, err.Error(), nil)
		return
	}

	record, err := h.chain.ByISBN(r.Context(), isbn13)
	switch {
	case errors.Is(err, ErrNotFound):
		httpserver.WriteError(w, nil, http.StatusNotFound,
			"no catalogue knows this ISBN, type the details by hand", nil)
		return
	case err != nil:
		// Lookup is a convenience. When it fails the person types the book in
		// themselves, so this must never be fatal.
		httpserver.WriteError(w, h.logger, http.StatusBadGateway,
			"the catalogues did not answer, type the details by hand", err)
		return
	}

	httpserver.WriteJSON(w, http.StatusOK, record)
}
