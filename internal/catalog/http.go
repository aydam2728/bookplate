package catalog

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/aydam2728/bookplate/internal/platform/httpserver"
	"github.com/aydam2728/bookplate/internal/platform/scope"
)

// CoverAttacher downloads a cover and points an edition at it. It is an
// interface so the catalogue does not depend on how images are fetched or
// stored, only that something can do it.
type CoverAttacher interface {
	AttachFromURL(ctx context.Context, caller scope.Caller, editionID, url string) error
}

// Handlers serves the catalogue endpoints.
type Handlers struct {
	store  *Store
	covers CoverAttacher
	logger *slog.Logger
}

// NewHandlers returns handlers over store. covers may be nil, in which case
// books are catalogued without pictures.
func NewHandlers(store *Store, covers CoverAttacher, logger *slog.Logger) *Handlers {
	return &Handlers{store: store, covers: covers, logger: logger}
}

// Register adds the catalogue routes to mux.
//
// protect is passed in rather than imported so that this package does not
// depend on how authentication works, only that every route has it.
func (h *Handlers) Register(mux *http.ServeMux, protect func(http.HandlerFunc) http.HandlerFunc) {
	mux.HandleFunc("GET /api/books", protect(h.list))
	mux.HandleFunc("POST /api/books", protect(h.add))
	mux.HandleFunc("GET /api/books/{id}", protect(h.one))
	mux.HandleFunc("PATCH /api/books/{id}", protect(h.update))
}

// unfiledFilter is the value of placeId that asks for the copies sitting
// nowhere. An identifier is 26 characters of Crockford base32, so a real
// place can never collide with it.
const unfiledFilter = "none"

func (h *Handlers) list(w http.ResponseWriter, r *http.Request) {
	caller, err := scope.FromContext(r.Context())
	if err != nil {
		httpserver.WriteError(w, h.logger, http.StatusUnauthorized, "sign in first", err)
		return
	}

	// An unfiled listing and a place listing are different questions, so
	// "unfiled" is a value of the place filter rather than a flag beside it:
	// one control in the interface, one parameter here.
	place := r.URL.Query().Get("placeId")
	q := Query{Text: r.URL.Query().Get("q"), Unfiled: place == unfiledFilter}
	if !q.Unfiled {
		q.PlaceID = place
	}

	books, err := h.store.List(r.Context(), caller, q)
	if errors.Is(err, ErrInvalid) {
		httpserver.WriteError(w, nil, http.StatusBadRequest, err.Error(), nil)
		return
	}
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

	// A cover is a nicety. Fetching it must never cost the person the book
	// they just catalogued, so a failure is logged and the entry stands.
	if h.covers != nil && in.CoverURL != "" {
		if err := h.covers.AttachFromURL(r.Context(), caller, book.EditionID, in.CoverURL); err != nil {
			h.logger.Warn("cover not attached", "edition", book.EditionID, "reason", err)
		} else if updated, err := h.store.ByCopyID(r.Context(), caller, book.CopyID); err == nil {
			book = updated
		}
	}

	httpserver.WriteJSON(w, http.StatusCreated, book)
}

func (h *Handlers) one(w http.ResponseWriter, r *http.Request) {
	caller, err := scope.FromContext(r.Context())
	if err != nil {
		httpserver.WriteError(w, h.logger, http.StatusUnauthorized, "sign in first", err)
		return
	}

	book, err := h.store.ByCopyID(r.Context(), caller, r.PathValue("id"))
	if err != nil {
		// One answer whether the copy does not exist or belongs to another
		// household: telling them apart would confirm it exists.
		httpserver.WriteError(w, nil, http.StatusNotFound, "no such book", nil)
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, book)
}

func (h *Handlers) update(w http.ResponseWriter, r *http.Request) {
	caller, err := scope.FromContext(r.Context())
	if err != nil {
		httpserver.WriteError(w, h.logger, http.StatusUnauthorized, "sign in first", err)
		return
	}

	var in Edit
	if err := httpserver.ReadJSON(w, r, &in); err != nil {
		httpserver.WriteError(w, h.logger, http.StatusBadRequest, "unreadable request", err)
		return
	}

	book, err := h.store.Update(r.Context(), caller, r.PathValue("id"), in)
	switch {
	case errors.Is(err, scope.ErrForbidden):
		httpserver.WriteError(w, nil, http.StatusForbidden, "your role cannot change books", nil)
		return
	case errors.Is(err, ErrInvalid):
		httpserver.WriteError(w, nil, http.StatusBadRequest, "no such place", nil)
		return
	case err != nil:
		httpserver.WriteError(w, h.logger, http.StatusNotFound, "no such book", err)
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, book)
}
