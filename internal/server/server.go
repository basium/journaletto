// Package server wires the HTTP routes.
package server

import (
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"html/template"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"strings"

	"journaletto/internal/config"
	"journaletto/internal/entries"
)

//go:embed templates/*.html
var templatesFS embed.FS

//go:embed static
var staticFS embed.FS

const maxRequestBytes = 512 * 1024

type page struct {
	Form     entries.Input
	TagsText string
	Error    *entries.ValidationError
	Saved    *entries.Entry
}

// New returns the application handler.
func New(cfg config.Config, db *sql.DB) http.Handler {
	tpl := template.Must(template.ParseFS(templatesFS, "templates/*.html"))
	svc := &entries.Service{DB: db, DataRoot: cfg.DataRoot, UserID: cfg.DevUserID, MaxBody: cfg.MaxEntryBytes}
	mux := http.NewServeMux()
	static, _ := fs.Sub(staticFS, "static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(static)))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "version": cfg.Version, "commit": cfg.Commit})
	})
	mux.HandleFunc("GET /status", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("Service is running (" + template.HTMLEscapeString(cfg.Version) + ")"))
	})
	render := func(w http.ResponseWriter, name string, p page) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := tpl.ExecuteTemplate(w, name, p); err != nil {
			log.Printf("render: %v", err)
		}
	}
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) {
		render(w, "home.html", page{})
	})
	mux.HandleFunc("GET /tags", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("tags")
		head, last := "", q
		if i := strings.LastIndex(q, ","); i >= 0 {
			head, last = q[:i+1]+" ", q[i+1:]
		}
		tags, err := svc.SuggestTags(r.Context(), last)
		if err != nil {
			log.Printf("tags: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		for _, t := range tags {
			_, _ = w.Write([]byte(`<option value="` + template.HTMLEscapeString(head+t) + `">`))
		}
	})
	mux.HandleFunc("POST /entries", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
		var in entries.Input
		asJSON := false
		if mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mt == "application/json" {
			asJSON = true
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
				writeJSON(w, http.StatusBadRequest, entries.ValidationError{Field: "body", Message: "invalid JSON or too large"})
				return
			}
		} else {
			if err := r.ParseForm(); err != nil {
				renderResult(w, r, render, page{Error: &entries.ValidationError{Field: "body", Message: "invalid or too large form"}}, http.StatusBadRequest)
				return
			}
			in = entries.Input{
				Title: r.PostFormValue("title"), JournalDate: r.PostFormValue("journal_date"),
				Description: r.PostFormValue("description"), Timezone: r.PostFormValue("timezone"),
				Tags: strings.Split(r.PostFormValue("tags"), ","),
			}
		}
		saved, err := svc.Save(r.Context(), in)
		var ve *entries.ValidationError
		switch {
		case errors.As(err, &ve):
			if asJSON {
				writeJSON(w, http.StatusBadRequest, ve)
				return
			}
			renderResult(w, r, render, page{Form: in, TagsText: strings.Join(in.Tags, ", "), Error: ve}, http.StatusBadRequest)
		case err != nil:
			log.Printf("save: %v", err)
			if asJSON {
				writeJSON(w, http.StatusInternalServerError, entries.ValidationError{Field: "", Message: "could not save entry"})
				return
			}
			renderResult(w, r, render, page{Form: in, TagsText: strings.Join(in.Tags, ", "), Error: &entries.ValidationError{Field: "server", Message: "could not save entry"}}, http.StatusInternalServerError)
		case asJSON:
			writeJSON(w, http.StatusCreated, saved)
		default:
			renderResult(w, r, render, page{Saved: &saved}, http.StatusCreated)
		}
	})
	return mux
}

// renderResult sends the fragment to HTMX requests and the full page otherwise.
func renderResult(w http.ResponseWriter, r *http.Request, render func(http.ResponseWriter, string, page), p page, code int) {
	name := "home.html"
	if r.Header.Get("HX-Request") == "true" {
		name = "result"
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(code)
	render(w, name, p)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
