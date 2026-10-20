package entries_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"journaletto/internal/entries"
	"journaletto/internal/provider"
	"journaletto/internal/worker"
)

// derived returns every piece of generated data of the entry as text, so a test can assert what
// it contains and that nothing from an older text is left.
func derived(t *testing.T, db *sql.DB, id string) string {
	t.Helper()
	var b strings.Builder
	for _, q := range []string{
		`SELECT 'artifact:'||kind||'='||content FROM entry_artifacts WHERE entry_id=? ORDER BY kind`,
		`SELECT 'entity:'||type||'='||value FROM entities WHERE entry_id=? ORDER BY type,value`,
		`SELECT 'relation:'||f.value||' '||r.kind||' '||t.value FROM entity_relations r JOIN entities f ON f.id=r.from_id JOIN entities t ON t.id=r.to_id WHERE f.entry_id=? ORDER BY 1`,
		`SELECT 'chunk:'||granularity||'='||context||'|'||text FROM chunks WHERE entry_id=? ORDER BY ord`,
		`SELECT 'chunkfts:'||text FROM chunks_fts WHERE entry_id=? ORDER BY rowid`,
		`SELECT 'entryfts:'||title||'|'||body||'|'||tags FROM entries_fts WHERE entry_id=?`,
		`SELECT 'occurrence:'||rule_id||'='||event_key||'|'||excerpt FROM occurrences WHERE entry_id=? ORDER BY rule_id,event_key`,
	} {
		rows, err := db.Query(q, id)
		if err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		for rows.Next() {
			var s string
			if err := rows.Scan(&s); err != nil {
				t.Fatal(err)
			}
			b.WriteString(s + "\n")
		}
		rows.Close()
	}
	return b.String()
}

// counts reports the row counts that must stay stable when an entry is reprocessed.
func counts(t *testing.T, db *sql.DB) string {
	t.Helper()
	return scalar(t, db, `SELECT (SELECT COUNT(*) FROM entities)||','||(SELECT COUNT(*) FROM entity_relations)||','||(SELECT COUNT(*) FROM chunks)||','||(SELECT COUNT(*) FROM chunk_embeddings)||','||(SELECT COUNT(*) FROM chunks_fts)||','||(SELECT COUNT(*) FROM entries_fts)||','||(SELECT COUNT(*) FROM occurrences)||','||(SELECT COUNT(*) FROM entry_artifacts)`)
}

func previousFiles(path string) []string {
	m, _ := filepath.Glob(filepath.Join(filepath.Dir(path), "versions", "*.previous.md"))
	return m
}

func TestEditRegeneratesAllDerivedDataFromTheNewTextOnly(t *testing.T) {
	svc, _, _ := newSvc(t)
	clock := time.Date(2026, 3, 4, 10, 0, 0, 0, time.UTC)
	svc.Now = func() time.Time { return clock }
	saved, err := svc.Save(context.Background(), entries.Input{Title: "Zebra planning", Tags: []string{"old"},
		Description: "# Zebra section\n\nWe discussed Fake Project and the zebra roadmap.\nI closed ticket ZEB-1 today.\n"})
	if err != nil {
		t.Fatal(err)
	}
	process(t, svc.DB, saved.ID)
	before := derived(t, svc.DB, saved.ID)
	for _, want := range []string{"Zebra planning", "zebra roadmap", "closed ticket zeb-1", "relation:Zebra planning part_of Fake Project"} {
		if !strings.Contains(before, want) {
			t.Fatalf("first processing lacks %q:\n%s", want, before)
		}
	}
	v0, _ := svc.Get(context.Background(), saved.ID)

	clock = clock.Add(time.Hour)
	if _, err := svc.Update(context.Background(), saved.ID, entries.Input{Title: "Giraffe review", Tags: []string{"new"},
		Description: "# Giraffe section\n\nThe giraffe habitat was reviewed.\nI merged PR 12 today.\n"}); err != nil {
		t.Fatal(err)
	}
	process(t, svc.DB, saved.ID)
	after := derived(t, svc.DB, saved.ID)

	for _, want := range []string{"Giraffe review", "giraffe habitat", "merged pr 12", "internal_tags"} {
		if !strings.Contains(after, want) {
			t.Errorf("regenerated data lacks %q:\n%s", want, after)
		}
	}
	// Fake Project is not in the new text, so the grounded pipeline must drop it along with the old entity.
	for _, gone := range []string{"zebra", "Zebra", "zeb-1", "Fake Project"} {
		if strings.Contains(after, gone) {
			t.Errorf("stale data %q survived the edit:\n%s", gone, after)
		}
	}
	for _, q := range []string{
		`SELECT COUNT(*) FROM chunks_fts WHERE chunks_fts MATCH 'zebra'`,
		`SELECT COUNT(*) FROM entries_fts WHERE entries_fts MATCH 'zebra'`,
	} {
		if n := scalar(t, svc.DB, q); n != "0" {
			t.Errorf("%s = %s", q, n)
		}
	}
	if n := scalar(t, svc.DB, `SELECT COUNT(*) FROM chunks_fts WHERE chunks_fts MATCH 'giraffe'`); n == "0" {
		t.Error("new text not searchable in chunks_fts")
	}

	v1, _ := svc.Get(context.Background(), saved.ID)
	if v1.ID != v0.ID || v1.CreatedAt != v0.CreatedAt || v1.UpdatedAt <= v0.UpdatedAt {
		t.Errorf("identity/timestamps: before %+v after %+v", v0, v1)
	}
	if filepath.Base(saved.Path) != filepath.Base(scalar(t, svc.DB, `SELECT file_path FROM entries WHERE id=?`, saved.ID)) {
		t.Error("file name changed")
	}
	if !strings.Contains(v1.Summary, "Giraffe") || strings.Contains(v1.Summary, "Zebra") {
		t.Errorf("summary = %q", v1.Summary)
	}
}

func TestEditAdvancesUpdatedAtEvenWithinTheSameSecond(t *testing.T) {
	svc, _, _ := newSvc(t)
	frozen := time.Date(2026, 3, 4, 10, 0, 0, 0, time.UTC)
	svc.Now = func() time.Time { return frozen }
	saved, err := svc.Save(context.Background(), entries.Input{Title: "T", Description: "one"})
	if err != nil {
		t.Fatal(err)
	}
	last := ""
	for i := 0; i < 3; i++ {
		if _, err := svc.Update(context.Background(), saved.ID, entries.Input{Title: "T", Description: fmt.Sprint("edit ", i)}); err != nil {
			t.Fatal(err)
		}
		v, _ := svc.Get(context.Background(), saved.ID)
		if last != "" && v.UpdatedAt <= last {
			t.Fatalf("edit %d: updated_at %s did not advance past %s", i, v.UpdatedAt, last)
		}
		last = v.UpdatedAt
		raw, _ := os.ReadFile(saved.Path)
		if !strings.Contains(string(raw), "updated_at: \""+v.UpdatedAt+"\"") {
			t.Fatalf("file updated_at differs from row %s:\n%s", v.UpdatedAt, raw)
		}
	}
	if n := len(previousFiles(saved.Path)); n != 1 {
		t.Fatalf("previous files = %d", n)
	}
}

func TestRepeatedEditResyncDeleteRestoreKeepsOneCoherentSetAndOnePrevious(t *testing.T) {
	svc, _, _ := newSvc(t)
	ctx := context.Background()
	clock := time.Date(2026, 3, 4, 10, 0, 0, 0, time.UTC)
	svc.Now = func() time.Time { return clock }
	saved, err := svc.Save(ctx, entries.Input{Title: "Start", Description: "# S\n\nFake Project start.\nI closed ticket A-1.\n"})
	if err != nil {
		t.Fatal(err)
	}
	process(t, svc.DB, saved.ID)

	edit := func(i int) {
		clock = clock.Add(time.Hour)
		prev, _ := os.ReadFile(saved.Path)
		body := fmt.Sprintf("# S\n\nFake Project round %d.\nI closed ticket A-%d.\n", i, i)
		if _, err := svc.Update(ctx, saved.ID, entries.Input{Title: fmt.Sprint("Round ", i), Description: body}); err != nil {
			t.Fatal(err)
		}
		process(t, svc.DB, saved.ID)
		files := previousFiles(saved.Path)
		if len(files) != 1 {
			t.Fatalf("round %d: previous files = %v", i, files)
		}
		got, _ := os.ReadFile(files[0])
		if string(got) != string(prev) {
			t.Fatalf("round %d: previous is not the immediately prior content:\n%s", i, got)
		}
	}
	edit(1)
	stable := counts(t, svc.DB)
	for i := 2; i <= 5; i++ {
		edit(i)
		if err := worker.Resync(ctx, svc.DB, "u", saved.ID); err != nil {
			t.Fatal(err)
		}
		process(t, svc.DB, saved.ID)
		if err := svc.Delete(ctx, saved.ID); err != nil {
			t.Fatal(err)
		}
		if err := svc.Delete(ctx, saved.ID); err != nil {
			t.Fatal(err)
		}
		if err := svc.Restore(ctx, saved.ID); err != nil {
			t.Fatal(err)
		}
		if got := counts(t, svc.DB); got != stable {
			t.Fatalf("round %d: counts %s, want %s", i, got, stable)
		}
		d := derived(t, svc.DB, saved.ID)
		if !strings.Contains(d, fmt.Sprintf("closed ticket a-%d", i)) || strings.Contains(d, fmt.Sprintf("a-%d", i-1)) {
			t.Fatalf("round %d: derived data not from the latest text:\n%s", i, d)
		}
	}
	if n := scalar(t, svc.DB, `SELECT COUNT(*) FROM jobs WHERE state IN ('queued','running')`); n != "0" {
		t.Errorf("active jobs left: %s", n)
	}
}

func TestConcurrentEditsAndResyncsConvergeOnTheLatestText(t *testing.T) {
	svc, _, _ := newSvc(t)
	ctx, cancel := context.WithCancel(context.Background())
	saved, err := svc.Save(ctx, entries.Input{Title: "Start", Description: "# S\n\nFake Project start.\nI closed ticket A-0.\n"})
	if err != nil {
		t.Fatal(err)
	}
	w := &worker.Worker{DB: svc.DB, Steps: worker.DefaultSteps(svc.DB, provider.Fake{}), Poll: 2 * time.Millisecond}
	done := make(chan struct{})
	go func() { _ = w.Run(ctx); close(done) }()
	defer func() { cancel(); <-done }()

	var wg sync.WaitGroup
	for i := 1; i <= 6; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			body := fmt.Sprintf("# S\n\nFake Project round %d.\nI closed ticket A-%d.\n", i, i)
			if _, err := svc.Update(ctx, saved.ID, entries.Input{Title: fmt.Sprint("Round ", i), Description: body}); err != nil {
				t.Error(err)
			}
		}()
		go func() {
			defer wg.Done()
			if err := worker.Resync(ctx, svc.DB, "u", saved.ID); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()

	settled := false
	for i := 0; i < 600 && !settled; i++ {
		settled = scalar(t, svc.DB, `SELECT processing_status FROM entries WHERE id=?`, saved.ID) == "ready" &&
			scalar(t, svc.DB, `SELECT COUNT(*) FROM jobs WHERE state IN ('queued','running')`) == "0"
		if !settled {
			time.Sleep(10 * time.Millisecond)
		}
	}
	if !settled {
		t.Fatal("entry never settled")
	}
	v, _ := svc.Get(ctx, saved.ID)
	d := derived(t, svc.DB, saved.ID)
	title := strings.TrimPrefix(v.Title, "Round ")
	if !strings.Contains(d, "closed ticket a-"+title) {
		t.Errorf("derived data does not match the stored text (title %q):\n%s", v.Title, d)
	}
	for i := 0; i <= 6; i++ {
		if fmt.Sprint(i) != title && strings.Contains(d, fmt.Sprintf("a-%d", i)) {
			t.Errorf("stale data from round %d:\n%s", i, d)
		}
	}
	if got := counts(t, svc.DB); got != "2,1,2,2,2,1,1,2" {
		t.Errorf("counts = %s", got)
	}
	if n := len(previousFiles(saved.Path)); n != 1 {
		t.Errorf("previous files = %d", n)
	}
}
