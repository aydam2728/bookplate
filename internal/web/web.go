// Package web serves the interface: a single page, its script and its
// stylesheet, embedded in the binary so the image stays one artefact.
//
// This is the P1 walking skeleton. The React client of
// docs/adr/0002-go-server-typescript-client.md lands at P5, when the
// interface grows past one page.
package web

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed assets
var assets embed.FS

// Register serves the page and its assets from mux.
func Register(mux *http.ServeMux) {
	sub, err := fs.Sub(assets, "assets")
	if err != nil {
		// Unreachable: the directory is embedded at build time.
		panic(err)
	}
	files := http.FileServerFS(sub)

	mux.Handle("GET /app.js", files)
	mux.Handle("GET /app.css", files)
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFileFS(w, r, sub, "index.html")
	})
}

// Harden sets the response headers that keep a page honest.
//
// The policy allows nothing by default and no inline script at all, which is
// why the script and the stylesheet are separate files rather than embedded
// in the page.
func Harden(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy",
			"default-src 'none'; script-src 'self'; style-src 'self'; "+
				"img-src 'self' data:; connect-src 'self'; form-action 'none'; "+
				"base-uri 'none'; frame-ancestors 'none'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}
