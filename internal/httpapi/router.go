package httpapi

import "net/http"

// NewRouter builds the HTTP router for the journal server.
func NewRouter(deps EntryDeps) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/entries", deps.CreateEntry)
	return mux
}
