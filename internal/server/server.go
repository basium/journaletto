// Package server wires the HTTP routes.
package server

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"html/template"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"journaletto/internal/config"
	"journaletto/internal/entries"
	"journaletto/internal/worker"
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
	ID       string // set on the edit page
	Full     string // full-page template for non-HTMX results; default home.html
	Updated  bool
}

// readInput parses a create/update submission (JSON or form). On failure it writes the
// response itself and returns ok=false.
func readInput(w http.ResponseWriter, r *http.Request, render func(http.ResponseWriter, string, page), full string) (in entries.Input, asJSON, ok bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
	if mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mt == "application/json" {
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeJSON(w, http.StatusBadRequest, entries.ValidationError{Field: "body", Message: "invalid JSON or too large"})
			return in, true, false
		}
		return in, true, true
	}
	if err := r.ParseForm(); err != nil {
		p := page{Full: full, ID: r.PathValue("id"), Error: &entries.ValidationError{Field: "body", Message: "invalid or too large form"}}
		renderResult(w, r, render, p, http.StatusBadRequest)
		return in, false, false
	}
	return entries.Input{
		Title: r.PostFormValue("title"), JournalDate: r.PostFormValue("journal_date"),
		Description: r.PostFormValue("description"), Timezone: r.PostFormValue("timezone"),
		Tags: strings.Split(r.PostFormValue("tags"), ","),
	}, false, true
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
		in, asJSON, ok := readInput(w, r, render, "home.html")
		if !ok {
			return
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
	// status renders the processing-status fragment, which polls itself until a terminal state.
	status := func(w http.ResponseWriter, r *http.Request, code int) {
		id := r.PathValue("id")
		st, err := worker.Status(r.Context(), db, cfg.DevUserID, id)
		if errors.Is(err, worker.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			log.Printf("status: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if strings.Contains(r.Header.Get("Accept"), "application/json") {
			writeJSON(w, code, map[string]string{"id": id, "status": st})
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(code)
		if err := tpl.ExecuteTemplate(w, "status", map[string]any{"ID": id, "Status": st, "Polling": st == "pending" || st == "processing"}); err != nil {
			log.Printf("render: %v", err)
		}
	}
	mux.HandleFunc("GET /entries", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		inc, _ := strconv.ParseBool(q.Get("include_deleted"))
		pg, _ := strconv.Atoi(q.Get("page"))
		lp, err := svc.List(r.Context(), inc, pg)
		if err != nil {
			log.Printf("list: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if strings.Contains(r.Header.Get("Accept"), "application/json") {
			writeJSON(w, http.StatusOK, lp)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := tpl.ExecuteTemplate(w, "list.html", lp); err != nil {
			log.Printf("render: %v", err)
		}
	})
	mux.HandleFunc("GET /entries/{id}", func(w http.ResponseWriter, r *http.Request) {
		v, err := svc.Get(r.Context(), r.PathValue("id"))
		if errors.Is(err, entries.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			log.Printf("view: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if strings.Contains(r.Header.Get("Accept"), "application/json") {
			writeJSON(w, http.StatusOK, v)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := tpl.ExecuteTemplate(w, "entry.html", v); err != nil {
			log.Printf("render: %v", err)
		}
	})
	mux.HandleFunc("GET /entries/{id}/edit", func(w http.ResponseWriter, r *http.Request) {
		v, err := svc.Get(r.Context(), r.PathValue("id"))
		if errors.Is(err, entries.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			log.Printf("edit: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		render(w, "edit.html", page{ID: v.ID, Full: "edit.html", TagsText: strings.Join(v.Tags, ", "),
			Form: entries.Input{Title: v.Title, JournalDate: v.JournalDate, Description: v.Body, Timezone: v.Timezone}})
	})
	mux.HandleFunc("POST /entries/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		in, asJSON, ok := readInput(w, r, render, "edit.html")
		if !ok {
			return
		}
		saved, err := svc.Update(r.Context(), id, in)
		var ve *entries.ValidationError
		hx := r.Header.Get("HX-Request") == "true"
		switch {
		case errors.Is(err, entries.ErrNotFound):
			http.NotFound(w, r)
		case errors.Is(err, entries.ErrDeleted):
			if asJSON {
				writeJSON(w, http.StatusConflict, entries.ValidationError{Message: "entry is deleted; restore it first"})
				return
			}
			http.Error(w, "entry is deleted; restore it first", http.StatusConflict)
		case errors.As(err, &ve):
			if asJSON {
				writeJSON(w, http.StatusBadRequest, ve)
				return
			}
			renderResult(w, r, render, page{ID: id, Full: "edit.html", Form: in, TagsText: strings.Join(in.Tags, ", "), Error: ve}, http.StatusBadRequest)
		case err != nil:
			log.Printf("update: %v", err)
			if asJSON {
				writeJSON(w, http.StatusInternalServerError, entries.ValidationError{Message: "could not save entry"})
				return
			}
			renderResult(w, r, render, page{ID: id, Full: "edit.html", Form: in, TagsText: strings.Join(in.Tags, ", "), Error: &entries.ValidationError{Field: "server", Message: "could not save entry"}}, http.StatusInternalServerError)
		case asJSON:
			writeJSON(w, http.StatusOK, saved)
		case hx:
			renderResult(w, r, render, page{ID: id, Saved: &saved, Updated: true}, http.StatusOK)
		default:
			http.Redirect(w, r, "/entries/"+id, http.StatusSeeOther)
		}
	})
	toggle := func(op func(context.Context, string) error) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			id := r.PathValue("id")
			err := op(r.Context(), id)
			if errors.Is(err, entries.ErrNotFound) {
				http.NotFound(w, r)
				return
			}
			if err != nil {
				log.Printf("delete/restore: %v", err)
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			if strings.Contains(r.Header.Get("Accept"), "application/json") {
				v, err := svc.Get(r.Context(), id)
				if err != nil {
					http.Error(w, "internal error", http.StatusInternalServerError)
					return
				}
				writeJSON(w, http.StatusOK, v)
				return
			}
			http.Redirect(w, r, "/entries/"+id, http.StatusSeeOther)
		}
	}
	mux.HandleFunc("POST /entries/{id}/delete", toggle(svc.Delete))
	mux.HandleFunc("POST /entries/{id}/restore", toggle(svc.Restore))
	mux.HandleFunc("GET /entries/{id}/status", func(w http.ResponseWriter, r *http.Request) { status(w, r, http.StatusOK) })
	mux.HandleFunc("POST /entries/{id}/resync", func(w http.ResponseWriter, r *http.Request) {
		err := worker.Resync(r.Context(), db, cfg.DevUserID, r.PathValue("id"))
		if errors.Is(err, worker.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			log.Printf("resync: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		status(w, r, http.StatusAccepted)
	})
	return mux
}

// renderResult sends the fragment to HTMX requests and the full page otherwise.
func renderResult(w http.ResponseWriter, r *http.Request, render func(http.ResponseWriter, string, page), p page, code int) {
	name := "home.html"
	if p.Full != "" {
		name = p.Full
	}
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
