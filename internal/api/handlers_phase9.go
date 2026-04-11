package api

import (
	"encoding/json"
	"net/http"

	"github.com/conduit-obs/conduit/internal/version"
)

// Version returns the server version information.
func (h *Handlers) Version(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(version.Info())
}
