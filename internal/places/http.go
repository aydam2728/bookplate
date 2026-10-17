package places

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/aydam2728/bookplate/internal/platform/httpserver"
	"github.com/aydam2728/bookplate/internal/platform/scope"
)

// Handlers serves the places endpoints.
type Handlers struct {
	store  *Store
	logger *slog.Logger
}

// NewHandlers returns handlers over store.
func NewHandlers(store *Store, logger *slog.Logger) *Handlers {
	return &Handlers{store: store, logger: logger}
}

// Register adds the places routes to mux, behind the given guard.
func (h *Handlers) Register(mux *http.ServeMux, protect func(http.HandlerFunc) http.HandlerFunc) {
	mux.HandleFunc("GET /api/places", protect(h.list))
	mux.HandleFunc("POST /api/places", protect(h.create))
	mux.HandleFunc("POST /api/places/{id}/move", protect(h.move))
	mux.HandleFunc("POST /api/places/{id}/rename", protect(h.rename))
	mux.HandleFunc("DELETE /api/places/{id}", protect(h.remove))
	mux.HandleFunc("POST /api/places/assign", protect(h.assign))
}

func (h *Handlers) caller(w http.ResponseWriter, r *http.Request) (scope.Caller, bool) {
	caller, err := scope.FromContext(r.Context())
	if err != nil {
		httpserver.WriteError(w, h.logger, http.StatusUnauthorized, "sign in first", err)
		return scope.Caller{}, false
	}
	return caller, true
}

// fail maps a domain error onto a status and a message a person can act on.
func (h *Handlers) fail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, scope.ErrForbidden):
		httpserver.WriteError(w, nil, http.StatusForbidden, "your role cannot change places", nil)
	case errors.Is(err, ErrNotFound):
		httpserver.WriteError(w, nil, http.StatusNotFound, "no such place", nil)
	case errors.Is(err, ErrWouldCycle):
		httpserver.WriteError(w, nil, http.StatusBadRequest,
			"a place cannot be moved inside itself", nil)
	case errors.Is(err, ErrInvalid):
		httpserver.WriteError(w, nil, http.StatusBadRequest, err.Error(), nil)
	default:
		httpserver.WriteError(w, h.logger, http.StatusInternalServerError,
			"something went wrong", err)
	}
}

func (h *Handlers) list(w http.ResponseWriter, r *http.Request) {
	caller, ok := h.caller(w, r)
	if !ok {
		return
	}
	tree, err := h.store.List(r.Context(), caller)
	if err != nil {
		h.fail(w, err)
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, tree)
}

type createRequest struct {
	ParentID string `json:"parentId"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
}

func (h *Handlers) create(w http.ResponseWriter, r *http.Request) {
	caller, ok := h.caller(w, r)
	if !ok {
		return
	}
	var req createRequest
	if err := httpserver.ReadJSON(w, r, &req); err != nil {
		httpserver.WriteError(w, h.logger, http.StatusBadRequest, "unreadable request", err)
		return
	}
	place, err := h.store.Create(r.Context(), caller, req.ParentID, req.Name, req.Kind)
	if err != nil {
		h.fail(w, err)
		return
	}
	httpserver.WriteJSON(w, http.StatusCreated, place)
}

type moveRequest struct {
	ParentID string `json:"parentId"`
}

func (h *Handlers) move(w http.ResponseWriter, r *http.Request) {
	caller, ok := h.caller(w, r)
	if !ok {
		return
	}
	var req moveRequest
	if err := httpserver.ReadJSON(w, r, &req); err != nil {
		httpserver.WriteError(w, h.logger, http.StatusBadRequest, "unreadable request", err)
		return
	}
	if err := h.store.Move(r.Context(), caller, r.PathValue("id"), req.ParentID); err != nil {
		h.fail(w, err)
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, map[string]bool{"moved": true})
}

type renameRequest struct {
	Name string `json:"name"`
}

func (h *Handlers) rename(w http.ResponseWriter, r *http.Request) {
	caller, ok := h.caller(w, r)
	if !ok {
		return
	}
	var req renameRequest
	if err := httpserver.ReadJSON(w, r, &req); err != nil {
		httpserver.WriteError(w, h.logger, http.StatusBadRequest, "unreadable request", err)
		return
	}
	if err := h.store.Rename(r.Context(), caller, r.PathValue("id"), req.Name); err != nil {
		h.fail(w, err)
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, map[string]bool{"renamed": true})
}

func (h *Handlers) remove(w http.ResponseWriter, r *http.Request) {
	caller, ok := h.caller(w, r)
	if !ok {
		return
	}
	if err := h.store.Delete(r.Context(), caller, r.PathValue("id")); err != nil {
		h.fail(w, err)
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

type assignRequest struct {
	CopyIDs []string `json:"copyIds"`
	PlaceID string   `json:"placeId"`
}

func (h *Handlers) assign(w http.ResponseWriter, r *http.Request) {
	caller, ok := h.caller(w, r)
	if !ok {
		return
	}
	var req assignRequest
	if err := httpserver.ReadJSON(w, r, &req); err != nil {
		httpserver.WriteError(w, h.logger, http.StatusBadRequest, "unreadable request", err)
		return
	}
	moved, err := h.store.AssignCopies(r.Context(), caller, req.CopyIDs, req.PlaceID)
	if err != nil {
		h.fail(w, err)
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, map[string]int{"moved": moved})
}
