package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"journaletto/internal/config"
	"journaletto/internal/server"
	"journaletto/internal/store"
)

func setup(t *testing.T) (http.Handler, config.Config, func(q string) string) {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Config{Version: "1.2.3", Commit: "abc", DevUserID: "dev-user", DataRoot: dir, DBPath: filepath.Join(dir, "t.db")}
	db, err := store.Open(cfg.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	scalar := func(q string) string {
		var s string
		if err := db.QueryRow(q).Scan(&s); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		return s
	}
	return server.New(cfg, db), cfg, scalar
}

func post(h http.Handler, ct, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/entries", strings.NewReader(body))
	req.Header.Set("Content-Type", ct)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestHealthz(t *testing.T) {
	h, _, _ := setup(t)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || rec.Code != 200 || body["version"] != "1.2.3" {
		t.Fatalf("status %d body %q", rec.Code, rec.Body.String())
	}
}

func TestHomeServesFormWithVendoredHTMX(t *testing.T) {
	h, _, _ := setup(t)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	b := rec.Body.String()
	if rec.Code != 200 || !strings.Contains(b, `name="timezone"`) || strings.Contains(b, "unpkg") {
		t.Fatalf("status %d body %q", rec.Code, b)
	}
	js := httptest.NewRecorder()
	h.ServeHTTP(js, httptest.NewRequest(http.MethodGet, "/static/htmx.min.js", nil))
	if js.Code != 200 || js.Body.Len() < 10000 {
		t.Fatalf("htmx asset status %d", js.Code)
	}
}

func TestSaveEntryWritesMarkdownAndCanonicalRows(t *testing.T) {
	h, cfg, scalar := setup(t)
	rec := post(h, "application/json", `{"title":"Day one","journal_date":"2026-03-04","description":"Hello **world**","tags":["Work","work"," Home "],"timezone":"Europe/Rome"}`)
	if rec.Code != 201 {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	var out struct{ ID, Path, Status string }
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	rel, _ := filepath.Rel(cfg.DataRoot, out.Path)
	re := regexp.MustCompile(`^journals/dev-user/\d{4}-\d{2}-\d{2}/\d{8}T\d{6}Z_` + out.ID + `\.md$`)
	if !re.MatchString(filepath.ToSlash(rel)) {
		t.Fatalf("unexpected path %s", rel)
	}
	raw, err := os.ReadFile(out.Path)
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	for _, want := range []string{"schema_version: 1", "id: " + out.ID, "title: Day one", "journal_date: \"2026-03-04\"", "timezone: Europe/Rome", "- work", "- home", "status: active", "\n# Day one\n", "Hello **world**"} {
		if !strings.Contains(s, want) {
			t.Errorf("file missing %q:\n%s", want, s)
		}
	}
	if strings.Count(s, "- work") != 1 {
		t.Errorf("tags not deduped:\n%s", s)
	}
	if got := scalar(`SELECT processing_status FROM entries`); got != "pending" {
		t.Errorf("processing_status %s", got)
	}
	if got := scalar(`SELECT entry_id FROM entries_fts WHERE entries_fts MATCH 'world'`); got != out.ID {
		t.Errorf("fts row %q", got)
	}
}

func TestInvalidInputRejected(t *testing.T) {
	h, cfg, scalar := setup(t)
	rec := post(h, "application/json", `{"title":"  ","description":"x"}`)
	var ve struct{ Field, Message string }
	if rec.Code != 400 || json.Unmarshal(rec.Body.Bytes(), &ve) != nil || ve.Field != "title" || ve.Message == "" {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	form := url.Values{"title": {"ok"}, "journal_date": {"nope"}}
	hx := httptest.NewRequest(http.MethodPost, "/entries", strings.NewReader(form.Encode()))
	hx.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	hx.Header.Set("HX-Request", "true")
	hrec := httptest.NewRecorder()
	h.ServeHTTP(hrec, hx)
	if hrec.Code != 400 || !strings.Contains(hrec.Body.String(), "journal_date") {
		t.Fatalf("html status %d body %s", hrec.Code, hrec.Body.String())
	}
	if n := scalar(`SELECT COUNT(*) FROM entries`); n != "0" {
		t.Errorf("rows persisted for invalid input: %s", n)
	}
	if _, err := os.Stat(filepath.Join(cfg.DataRoot, "journals")); err == nil {
		t.Errorf("files written for invalid input")
	}
}
