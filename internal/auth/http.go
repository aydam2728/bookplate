package auth

import (
	"errors"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/aydam2728/bookplate/internal/platform/httpserver"
	"github.com/aydam2728/bookplate/internal/platform/scope"
)

// SessionCookie is the cookie that carries the session token.
const SessionCookie = "bookplate_session"

// Handlers serves the account endpoints.
type Handlers struct {
	store   *Store
	limiter *Limiter
	logger  *slog.Logger
}

// NewHandlers returns handlers over store.
func NewHandlers(store *Store, logger *slog.Logger) *Handlers {
	return &Handlers{store: store, limiter: NewLimiter(), logger: logger}
}

// Register adds the account routes to mux.
func (h *Handlers) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/session", h.session)
	mux.HandleFunc("POST /api/setup", h.setup)
	mux.HandleFunc("POST /api/login", h.login)
	mux.HandleFunc("POST /api/logout", h.logout)
}

// Protect runs next only for an authenticated caller, which it places on the
// request context for the handler to take back out.
func (h *Handlers) Protect(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, err := h.caller(r)
		if err != nil {
			httpserver.WriteError(w, nil, http.StatusUnauthorized, "sign in first", nil)
			return
		}
		next(w, r.WithContext(scope.NewContext(r.Context(), caller)))
	}
}

func (h *Handlers) caller(r *http.Request) (scope.Caller, error) {
	cookie, err := r.Cookie(SessionCookie)
	if err != nil || cookie.Value == "" {
		return scope.Caller{}, ErrNotFound
	}
	return h.store.Caller(r.Context(), cookie.Value)
}

type sessionView struct {
	NeedsSetup bool   `json:"needsSetup"`
	SignedIn   bool   `json:"signedIn"`
	UserID     string `json:"userId,omitempty"`
	Role       string `json:"role,omitempty"`
}

// session tells the interface what to show: the setup form, the login form,
// or the shelf.
func (h *Handlers) session(w http.ResponseWriter, r *http.Request) {
	needsSetup, err := h.store.NeedsSetup(r.Context())
	if err != nil {
		httpserver.WriteError(w, h.logger, http.StatusInternalServerError, "something went wrong", err)
		return
	}
	view := sessionView{NeedsSetup: needsSetup}
	if caller, err := h.caller(r); err == nil {
		view.SignedIn = true
		view.UserID = caller.UserID
		view.Role = string(caller.Role)
	}
	httpserver.WriteJSON(w, http.StatusOK, view)
}

type setupRequest struct {
	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
	Password    string `json:"password"`
	LibraryName string `json:"libraryName"`
}

func (h *Handlers) setup(w http.ResponseWriter, r *http.Request) {
	var req setupRequest
	if err := httpserver.ReadJSON(w, r, &req); err != nil {
		httpserver.WriteError(w, h.logger, http.StatusBadRequest, "unreadable request", err)
		return
	}
	if req.LibraryName == "" {
		req.LibraryName = "Home"
	}

	err := h.store.Setup(r.Context(), req.Email, req.DisplayName, req.Password, req.LibraryName)
	switch {
	case errors.Is(err, ErrAlreadySetUp):
		httpserver.WriteError(w, nil, http.StatusConflict, "this instance is already set up", nil)
		return
	case errors.Is(err, ErrPasswordTooShort):
		httpserver.WriteError(w, nil, http.StatusBadRequest,
			"the password needs at least 10 characters", nil)
		return
	case err != nil:
		httpserver.WriteError(w, h.logger, http.StatusInternalServerError, "setup failed", err)
		return
	}

	h.signIn(w, r, req.Email, req.Password)
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *Handlers) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := httpserver.ReadJSON(w, r, &req); err != nil {
		httpserver.WriteError(w, h.logger, http.StatusBadRequest, "unreadable request", err)
		return
	}

	key := clientKey(r)
	if !h.limiter.Allow(key) {
		httpserver.WriteError(w, nil, http.StatusTooManyRequests,
			"too many attempts, wait a few minutes", nil)
		return
	}

	h.signIn(w, r, req.Email, req.Password)
}

func (h *Handlers) signIn(w http.ResponseWriter, r *http.Request, email, password string) {
	key := clientKey(r)

	token, err := h.store.Login(r.Context(), email, password, r.UserAgent())
	if errors.Is(err, ErrNotFound) {
		h.limiter.Record(key)
		// One message for a wrong password and for an unknown address.
		httpserver.WriteError(w, nil, http.StatusUnauthorized,
			"that address and password do not match", nil)
		return
	}
	if err != nil {
		httpserver.WriteError(w, h.logger, http.StatusInternalServerError, "sign in failed", err)
		return
	}
	h.limiter.Reset(key)

	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   isSecure(r),
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(SessionTTL),
	})
	httpserver.WriteJSON(w, http.StatusOK, map[string]bool{"signedIn": true})
}

func (h *Handlers) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(SessionCookie); err == nil && cookie.Value != "" {
		if err := h.store.Logout(r.Context(), cookie.Value); err != nil {
			h.logger.Error("logout", "reason", err)
		}
	}
	http.SetCookie(w, &http.Cookie{
		Name: SessionCookie, Value: "", Path: "/",
		HttpOnly: true, Secure: isSecure(r), SameSite: http.SameSiteLaxMode, MaxAge: -1,
	})
	httpserver.WriteJSON(w, http.StatusOK, map[string]bool{"signedIn": false})
}

// isSecure decides whether to mark the cookie Secure. Marking it
// unconditionally would break a plain-HTTP instance on a local network; not
// marking it at all would leak the session on a proxied connection.
func isSecure(r *http.Request) bool {
	return r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
}

// clientKey throttles by source address. It is not a strong identity, but it
// is what a login form has before anyone is authenticated.
func clientKey(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
