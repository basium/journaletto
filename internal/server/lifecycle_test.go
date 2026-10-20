package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"journaletto/internal/config"
	"journaletto/internal/provider"
	"journaletto/internal/server"
	"journaletto/internal/store"
	"journaletto/internal/worker"
)

// TestMilestone2EntryLifecycleEndToEnd is the M2 gate. One entry goes through the public HTTP
// routes with a real worker (fake provider): create, ready, two edits, delete, list (hidden, then
// shown as Deleted), restore and resync. After every step the raw file, its front matter, the
// database counts and the number of previous versions are asserted.
func TestMilestone2EntryLifecycleEndToEnd(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Config{Version: "t", Commit: "c", DevUserID: "dev-user", DataRoot: dir, DBPath: filepath.Join(dir, "t.db")}
	db, err := store.Open(cfg.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	h := server.New(cfg, db)

	ctx, cancel := context.WithCancel(context.Background())
	w := &worker.Worker{DB: db, Steps: worker.DefaultSteps(db, provider.Fake{}), Poll: 2 * time.Millisecond}
	done := make(chan struct{})
	go func() { _ = w.Run(ctx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })

	do := func(method, path, accept, ct, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Accept", accept)
		if ct != "" {
			req.Header.Set("Content-Type", ct)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	one := func(q string, args ...any) string {
		t.Helper()
		var s string
		if err := db.QueryRow(q, args...).Scan(&s); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		return s
	}
	waitReady := func(id string) {
		t.Helper()
		for i := 0; i < 1000; i++ {
			if one(`SELECT processing_status FROM entries WHERE id=?`, id) == "ready" &&
				one(`SELECT COUNT(*) FROM jobs WHERE state IN ('queued','running')`) == "0" {
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
		t.Fatal("entry never became ready")
	}
	// counts of every table the pipeline fills, for one entry and for the whole database.
	counts := func() string {
		return one(`SELECT (SELECT COUNT(*) FROM entries)||','||(SELECT COUNT(*) FROM entities)||','||(SELECT COUNT(*) FROM entity_relations)||','||(SELECT COUNT(*) FROM chunks)||','||(SELECT COUNT(*) FROM chunk_embeddings)||','||(SELECT COUNT(*) FROM chunks_fts)||','||(SELECT COUNT(*) FROM entries_fts)||','||(SELECT COUNT(*) FROM occurrences)||','||(SELECT COUNT(*) FROM entry_artifacts)`)
	}
	read := func(path string) string {
		t.Helper()
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	previous := func(path string) []string {
		m, _ := filepath.Glob(filepath.Join(filepath.Dir(path), "versions", "*.previous.md"))
		return m
	}
	has := func(step, text string, want ...string) {
		t.Helper()
		for _, s := range want {
			if !strings.Contains(text, s) {
				t.Errorf("%s: missing %q in:\n%s", step, s, text)
			}
		}
	}
	lacks := func(step, text string, bad ...string) {
		t.Helper()
		for _, s := range bad {
			if strings.Contains(text, s) {
				t.Errorf("%s: unexpected %q in:\n%s", step, s, text)
			}
		}
	}

	// 1. create
	rec := do("POST", "/entries", "application/json", "application/json",
		`{"title":"Round 0","timezone":"UTC","tags":["Work"],"description":"# Plan\n\nFake Project zebra start.\nI closed ticket A-0 today.\n"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", rec.Code, rec.Body)
	}
	var saved struct{ ID, Path string }
	_ = json.Unmarshal(rec.Body.Bytes(), &saved)
	id, path := saved.ID, saved.Path
	raw := read(path)
	has("create", raw, "---\nschema_version: 1\n", "id: "+id, "title: Round 0", "status: active", "zebra start")
	lacks("create", raw, "deleted_at")
	if n := len(previous(path)); n != 0 {
		t.Errorf("create: previous versions = %d", n)
	}
	if got := one(`SELECT COUNT(*) FROM entries`); got != "1" {
		t.Errorf("create: entries = %s", got)
	}

	// 2. ready
	waitReady(id)
	ready := counts()
	if !strings.HasPrefix(ready, "1,") || strings.Contains(","+ready+",", ",0,") {
		t.Errorf("ready: counts %s (every derived table should be filled)", ready)
	}
	if got := one(`SELECT COUNT(*) FROM entries_fts WHERE entries_fts MATCH 'zebra'`); got != "1" {
		t.Errorf("ready: entry FTS hits = %s", got)
	}
	if n := len(previous(path)); n != 0 {
		t.Errorf("ready: previous versions = %d", n)
	}
	if read(path) != raw {
		t.Error("ready: processing modified the raw file")
	}

	// 3. edit x2: identity kept, one previous version holding the immediately prior content.
	created := one(`SELECT created_at FROM entries WHERE id=?`, id)
	updated := one(`SELECT updated_at FROM entries WHERE id=?`, id)
	for i := 1; i <= 2; i++ {
		step := "edit " + string(rune('0'+i))
		before := read(path)
		round := string(rune('0' + i))
		rec = do("POST", "/entries/"+id, "text/html", "application/x-www-form-urlencoded", url.Values{
			"title": {"Round " + round}, "timezone": {"UTC"}, "tags": {"work"},
			"description": {"# Plan\n\nFake Project giraffe " + round + ".\nI closed ticket A-" + round + " today.\n"},
		}.Encode())
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("%s = %d %s", step, rec.Code, rec.Body)
		}
		waitReady(id)
		raw = read(path)
		has(step, raw, "id: "+id, "created_at: \""+created+"\"", "title: Round "+round, "status: active", "giraffe "+round)
		lacks(step, raw, "zebra")
		if got := one(`SELECT created_at FROM entries WHERE id=?`, id); got != created {
			t.Errorf("%s: created_at changed to %s", step, got)
		}
		if now := one(`SELECT updated_at FROM entries WHERE id=?`, id); now <= updated {
			t.Errorf("%s: updated_at %s did not advance past %s", step, now, updated)
		} else {
			updated = now
		}
		prev := previous(path)
		if len(prev) != 1 {
			t.Fatalf("%s: previous versions = %v", step, prev)
		}
		if read(prev[0]) != before {
			t.Errorf("%s: previous version is not the immediately prior content", step)
		}
		if got := counts(); got != ready {
			t.Errorf("%s: counts %s, want %s", step, got, ready)
		}
		if got := one(`SELECT COUNT(*) FROM chunks_fts WHERE chunks_fts MATCH 'zebra'`); got != "0" {
			t.Errorf("%s: stale zebra chunks = %s", step, got)
		}
	}
	editedRaw := read(path)

	// 4. delete: front matter flips, file and derived data stay, still one previous version.
	if rec = do("POST", "/entries/"+id+"/delete", "application/json", "", ""); rec.Code != http.StatusOK {
		t.Fatalf("delete = %d", rec.Code)
	}
	raw = read(path)
	has("delete", raw, "status: deleted", "deleted_at: ", "giraffe 2")
	if got := one(`SELECT status FROM entries WHERE id=?`, id); got != "deleted" {
		t.Errorf("delete: row status %s", got)
	}
	if n := len(previous(path)); n != 1 {
		t.Errorf("delete: previous versions = %d", n)
	}
	if got := counts(); got != ready {
		t.Errorf("delete: counts %s, want %s", got, ready)
	}

	// 5. list hides the deleted entry; include_deleted shows it labelled Deleted.
	page := do("GET", "/entries", "text/html", "", "").Body.String()
	lacks("list", page, "Round 2")
	page = do("GET", "/entries?include_deleted=true", "text/html", "", "").Body.String()
	has("list include_deleted", page, "Round 2", ">Deleted<")
	var js struct {
		Items []struct{ Title, Status string }
	}
	_ = json.Unmarshal(do("GET", "/entries", "application/json", "", "").Body.Bytes(), &js)
	if len(js.Items) != 0 {
		t.Errorf("list: %d items, want 0", len(js.Items))
	}
	_ = json.Unmarshal(do("GET", "/entries?include_deleted=true", "application/json", "", "").Body.Bytes(), &js)
	if len(js.Items) != 1 || js.Items[0].Status != "deleted" {
		t.Errorf("list include_deleted: %+v", js.Items)
	}

	// 6. restore: back to the edited content with status active and no deleted_at.
	if rec = do("POST", "/entries/"+id+"/restore", "application/json", "", ""); rec.Code != http.StatusOK {
		t.Fatalf("restore = %d", rec.Code)
	}
	raw = read(path)
	has("restore", raw, "status: active", "giraffe 2", "id: "+id)
	lacks("restore", raw, "deleted_at", "status: deleted")
	if got := one(`SELECT status||COALESCE(deleted_at,'') FROM entries WHERE id=?`, id); got != "active" {
		t.Errorf("restore: row %q", got)
	}
	if n := len(previous(path)); n != 1 {
		t.Errorf("restore: previous versions = %d", n)
	}
	if got := counts(); got != ready {
		t.Errorf("restore: counts %s, want %s", got, ready)
	}
	if page = do("GET", "/entries", "text/html", "", "").Body.String(); !strings.Contains(page, "Round 2") {
		t.Errorf("restore: entry missing from default list")
	}
	// updated_at is rewritten by delete/restore, so compare the Markdown body only.
	body := func(s string) string { return s[strings.LastIndex(s, "\n# Round 2"):] }
	if body(raw) != body(editedRaw) {
		t.Error("restore: Markdown body differs from before the delete")
	}

	// 7. resync rebuilds everything from the raw file without duplicates.
	if rec = do("POST", "/entries/"+id+"/resync", "application/json", "", ""); rec.Code != http.StatusAccepted {
		t.Fatalf("resync = %d %s", rec.Code, rec.Body)
	}
	waitReady(id)
	if got := counts(); got != ready {
		t.Errorf("resync: counts %s, want %s", got, ready)
	}
	if n := len(previous(path)); n != 1 {
		t.Errorf("resync: previous versions = %d", n)
	}
	has("resync", read(path), "status: active", "giraffe 2")
	if got := one(`SELECT COUNT(*) FROM chunks_fts WHERE chunks_fts MATCH 'giraffe'`); got == "0" {
		t.Error("resync: edited text not searchable")
	}
}
