// Package server wires the HTTP routes.
package server

import (
	"embed"
	"encoding/json"
	"html/template"
	"net/http"

	"journaletto/internal/config"
)

//go:embed templates/*.html
var templatesFS embed.FS

// New returns the application handler.
func New(cfg config.Config) http.Handler {
	tpl := template.Must(template.ParseFS(templatesFS, "templates/*.html"))
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "version": cfg.Version, "commit": cfg.Commit})
	})
	mux.HandleFunc("GET /status", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("Service is running (" + template.HTMLEscapeString(cfg.Version) + ")"))
	})
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = tpl.ExecuteTemplate(w, "home.html", nil)
	})
	return mux
}
