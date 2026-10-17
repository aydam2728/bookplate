package httpserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
)

// MaxRequestBody bounds what a request may send. Nothing this API accepts is
// large, and an unbounded reader is a way to exhaust memory on a Pi.
const MaxRequestBody = 64 << 10

// WriteJSON sends v with the given status.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		// The status is already written; there is nothing useful left to say
		// to the client.
		return
	}
}

// WriteError sends a message the client may show to a person.
//
// It never carries an internal error: the threat model assumes the instance
// is on the internet, and a database error quoted back is free reconnaissance.
func WriteError(w http.ResponseWriter, logger *slog.Logger, status int, public string, err error) {
	if err != nil && logger != nil {
		logger.Error("request failed", "status", status, "reason", err)
	}
	WriteJSON(w, status, map[string]string{"error": public})
}

// ReadJSON decodes a request body into v.
//
// It requires a JSON content type, which also gives a measure of cross-site
// request forgery protection: a cross-origin HTML form cannot set it, so a
// hostile page cannot post one at a logged-in browser.
func ReadJSON(w http.ResponseWriter, r *http.Request, v any) error {
	contentType := r.Header.Get("Content-Type")
	if mediaType, _, _ := strings.Cut(contentType, ";"); strings.TrimSpace(mediaType) != "application/json" {
		return fmt.Errorf("expected a JSON body, got %q", contentType)
	}

	r.Body = http.MaxBytesReader(w, r.Body, MaxRequestBody)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(v); err != nil {
		return fmt.Errorf("unreadable body: %w", err)
	}
	if err := decoder.Decode(new(struct{})); !errors.Is(err, io.EOF) {
		return errors.New("the body has more than one JSON value")
	}
	return nil
}
