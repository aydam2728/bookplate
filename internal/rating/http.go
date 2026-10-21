package rating

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/aydam2728/bookplate/internal/platform/httpserver"
	"github.com/aydam2728/bookplate/internal/platform/scope"
)

// Handlers serves the personal layer.
type Handlers struct {
	store  *Store
	logger *slog.Logger
}

// NewHandlers returns handlers over store.
func NewHandlers(store *Store, logger *slog.Logger) *Handlers {
	return &Handlers{store: store, logger: logger}
}

// Register adds the rating routes to mux, behind the given guard.
func (h *Handlers) Register(mux *http.ServeMux, protect func(http.HandlerFunc) http.HandlerFunc) {
	mux.HandleFunc("GET /api/works/{id}/rating", protect(h.read))
	mux.HandleFunc("PUT /api/works/{id}/rating", protect(h.write))
}

// view is what a detail page needs: what I said, and what the household says.
type view struct {
	Mine      Entry     `json:"mine"`
	Household Household `json:"household"`
}

func (h *Handlers) read(w http.ResponseWriter, r *http.Request) {
	caller, err := scope.FromContext(r.Context())
	if err != nil {
		httpserver.WriteError(w, h.logger, http.StatusUnauthorized, "sign in first", err)
		return
	}
	workID := r.PathValue("id")

	// Only the caller's own entry is ever returned. Another member's review
	// is theirs, and the household figure is an aggregate that names nobody.
	mine, err := h.store.Mine(r.Context(), caller, workID)
	if err != nil {
		httpserver.WriteError(w, h.logger, http.StatusInternalServerError, "something went wrong", err)
		return
	}
	everyone, err := h.store.OfHousehold(r.Context(), caller, workID)
	if errors.Is(err, ErrNotFound) {
		httpserver.WriteError(w, nil, http.StatusNotFound, "no such book", nil)
		return
	}
	if err != nil {
		httpserver.WriteError(w, h.logger, http.StatusInternalServerError, "something went wrong", err)
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, view{Mine: mine, Household: everyone})
}

func (h *Handlers) write(w http.ResponseWriter, r *http.Request) {
	caller, err := scope.FromContext(r.Context())
	if err != nil {
		httpserver.WriteError(w, h.logger, http.StatusUnauthorized, "sign in first", err)
		return
	}
	workID := r.PathValue("id")

	var in Entry
	if err := httpserver.ReadJSON(w, r, &in); err != nil {
		httpserver.WriteError(w, h.logger, http.StatusBadRequest, "unreadable request", err)
		return
	}

	switch err := h.store.Set(r.Context(), caller, workID, in); {
	case errors.Is(err, ErrInvalid):
		httpserver.WriteError(w, nil, http.StatusBadRequest, err.Error(), nil)
		return
	case errors.Is(err, ErrNotFound):
		httpserver.WriteError(w, nil, http.StatusNotFound, "no such book", nil)
		return
	case errors.Is(err, scope.ErrForbidden):
		httpserver.WriteError(w, nil, http.StatusForbidden, "sign in first", nil)
		return
	case err != nil:
		httpserver.WriteError(w, h.logger, http.StatusInternalServerError, "something went wrong", err)
		return
	}
	h.read(w, r)
}
