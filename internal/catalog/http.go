package catalog

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/aydam2728/bookplate/internal/platform/httpserver"
	"github.com/aydam2728/bookplate/internal/platform/scope"
)

// Handlers serves the catalogue endpoints.
type Handlers struct {
	store  *Store
	logger *slog.Logger
}

// NewHandlers returns handlers over store.
func NewHandlers(store *Store, logger *slog.Logger) *Handlers {
	return &Handlers{store: store, logger: logger}
}

// Register adds the catalogue routes to mux.
//
// protect is passed in rather than imported so that this package does not
// depend on how authentication works, only that every route has it.
func (h *Handlers) Register(mux *http.ServeMux, protect func(http.HandlerFunc) http.HandlerFunc) {
	mux.HandleFunc("GET /api/books", protect(h.list))
	mux.HandleFunc("POST /api/books", protect(h.add))
}

func (h *Handlers) list(w http.ResponseWriter, r *http.Request) {
	caller, err := scope.FromContext(r.Context())
	if err != nil {
		httpserver.WriteError(w, h.logger, http.StatusUnauthorized, "sign in first", err)
		return
	}

	books, err := h.store.List(r.Context(), caller)
	if err != nil {
		httpserver.WriteError(w, h.logger, http.StatusInternalServerError, "could not read the shelf", err)
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, books)
}

func (h *Handlers) add(w http.ResponseWriter, r *http.Request) {
	caller, err := scope.FromContext(r.Context())
	if err != nil {
		httpserver.WriteError(w, h.logger, http.StatusUnauthorized, "sign in first", err)
		return
	}

	var in AddInput
	if err := httpserver.ReadJSON(w, r, &in); err != nil {
		httpserver.WriteError(w, h.logger, http.StatusBadRequest, "unreadable request", err)
		return
	}

	book, err := h.store.Add(r.Context(), caller, in)
	switch {
	case errors.Is(err, scope.ErrForbidden):
		httpserver.WriteError(w, nil, http.StatusForbidden, "your role cannot add books", nil)
		return
	case errors.Is(err, ErrNotABook):
		httpserver.WriteError(w, nil, http.StatusBadRequest, err.Error(), nil)
		return
	case errors.Is(err, ErrBadISBN):
		httpserver.WriteError(w, nil, http.StatusBadRequest,
			"that ISBN does not look right, check the digits", nil)
		return
	case errors.Is(err, ErrInvalid):
		httpserver.WriteError(w, nil, http.StatusBadRequest, "a title is required", nil)
		return
	case err != nil:
		httpserver.WriteError(w, h.logger, http.StatusInternalServerError, "could not add the book", err)
		return
	}

	httpserver.WriteJSON(w, http.StatusCreated, book)
}
