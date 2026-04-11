package web

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:static
var staticFiles embed.FS

// HasEmbeddedUI returns true if the static directory contains an index.html.
func HasEmbeddedUI() bool {
	f, err := staticFiles.Open("static/index.html")
	if err != nil {
		return false
	}
	f.Close()
	return true
}

// ServeUI returns an http.Handler that serves the embedded frontend with SPA fallback.
func ServeUI() http.Handler {
	subFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		return http.NotFoundHandler()
	}
	fileServer := http.FileServer(http.FS(subFS))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Don't serve UI for API routes
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}

		// Try to serve the actual file
		path := r.URL.Path
		if path == "/" {
			path = "/index.html"
		}

		// Check if file exists
		f, err := subFS.Open(strings.TrimPrefix(path, "/"))
		if err == nil {
			f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}

		// SPA fallback: serve index.html for all non-file paths
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})
}
