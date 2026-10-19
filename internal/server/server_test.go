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

func TestDateDefaultsClientSideOnlyWhenEmpty(t *testing.T) {
	h, _, _ := setup(t)
	get := func(path string) string {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		return rec.Body.String()
	}
	home := get("/")
	if !strings.Contains(home, `name="journal_date" value=""`) || !strings.Contains(home, `if (!dateEl.value)`) || !strings.Contains(home, "getFullYear()") || strings.Contains(home, "toISOString") {
		t.Errorf("new page should default date client-side from local parts when empty:\n%s", home)
	}
	var out struct{ ID string }
	if err := json.Unmarshal(post(h, "application/json", `{"title":"T","journal_date":"2026-03-04","timezone":"UTC"}`).Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if edit := get("/entries/" + out.ID + "/edit"); !strings.Contains(edit, `name="journal_date" value="2026-03-04"`) {
		t.Errorf("edit page should show stored date:\n%s", edit)
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

func TestStatusFragmentPollsUntilTerminalAndResyncRequeues(t *testing.T) {
	h, _, scalar := setup(t)
	var out struct{ ID string }
	if err := json.Unmarshal(post(h, "application/json", `{"title":"x"}`).Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	get := func(method, path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
		return rec
	}
	rec := get("GET", "/entries/"+out.ID+"/status")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "every 2s") || !strings.Contains(rec.Body.String(), "pending") {
		t.Fatalf("pending fragment: %d %s", rec.Code, rec.Body.String())
	}
	if page := get("GET", "/entries/"+out.ID); page.Code != 200 || !strings.Contains(page.Body.String(), "/status") {
		t.Fatalf("entry page: %d", page.Code)
	}

	scalar(`UPDATE entries SET processing_status='failed' RETURNING id`)
	rec = get("POST", "/entries/"+out.ID+"/resync")
	if rec.Code != 202 || !strings.Contains(rec.Body.String(), "pending") {
		t.Fatalf("resync: %d %s", rec.Code, rec.Body.String())
	}
	if n := scalar(`SELECT COUNT(*) FROM jobs WHERE state='queued'`); n != "1" {
		t.Fatalf("queued jobs = %s", n)
	}
	scalar(`UPDATE entries SET processing_status='ready' RETURNING id`)
	if rec = get("GET", "/entries/"+out.ID+"/status"); strings.Contains(rec.Body.String(), "every 2s") {
		t.Fatalf("ready fragment still polls: %s", rec.Body.String())
	}
	if get("POST", "/entries/nope/resync").Code != 404 {
		t.Fatal("unknown entry should 404")
	}
}

func TestViewShowsSectionsAndEditRedirectsToUpdatedEntry(t *testing.T) {
	h, _, scalar := setup(t)
	var out struct{ ID string }
	if err := json.Unmarshal(post(h, "application/json", `{"title":"Day <one>","description":"raw **body**","tags":["Mine"],"timezone":"UTC"}`).Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	scalar(`INSERT INTO entry_artifacts(entry_id,kind,content,provider,model,prompt_version,generated_at) VALUES ('` + out.ID + `','summary','SUMMARYTEXT','f','m','v','t') RETURNING kind`)
	scalar(`INSERT INTO entry_artifacts(entry_id,kind,content,provider,model,prompt_version,generated_at) VALUES ('` + out.ID + `','internal_tags','["hidden1"]','f','m','v','t') RETURNING kind`)
	get := func(path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		return rec
	}
	rec := get("/entries/" + out.ID)
	b := rec.Body.String()
	created := scalar(`SELECT created_at FROM entries`)
	for _, want := range []string{"Day &lt;one&gt;", "raw **body**", `<li>mine</li>`, `class="summary">SUMMARYTEXT`, `<li>hidden1</li>`, `<time class="local-time" datetime="` + created + `"`, "/entries/" + out.ID + "/edit"} {
		if !strings.Contains(b, want) {
			t.Errorf("view missing %q:\n%s", want, b)
		}
	}
	if form := get("/entries/" + out.ID + "/edit"); form.Code != 200 || !strings.Contains(form.Body.String(), `value="Day &lt;one&gt;"`) {
		t.Fatalf("edit form: %d %s", form.Code, form.Body.String())
	}

	edit := func(ct, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/entries/"+out.ID, strings.NewReader(body))
		req.Header.Set("Content-Type", ct)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	rec = edit("application/x-www-form-urlencoded", url.Values{"title": {"Renamed"}, "description": {"new body"}, "tags": {"x, y"}}.Encode())
	if rec.Code != 303 || rec.Header().Get("Location") != "/entries/"+out.ID {
		t.Fatalf("edit: %d %s", rec.Code, rec.Body.String())
	}
	if got := scalar(`SELECT title||'|'||body||'|'||processing_status FROM entries`); got != "Renamed|new body|pending" {
		t.Errorf("row after edit: %s", got)
	}
	if rec = edit("application/json", `{"title":""}`); rec.Code != 400 || !strings.Contains(rec.Body.String(), `"title"`) {
		t.Errorf("invalid edit: %d %s", rec.Code, rec.Body.String())
	}
	if rec = edit("application/json", `{"title":"x"}`); rec.Code != 200 {
		t.Errorf("json edit: %d", rec.Code)
	}
	if get("/entries/nope").Code != 404 {
		t.Error("unknown entry should 404")
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
