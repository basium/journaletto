package entries_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"journaletto/internal/entries"
	"journaletto/internal/provider"
	"journaletto/internal/worker"
)

func scalar(t *testing.T, db *sql.DB, q string, args ...any) string {
	t.Helper()
	var s string
	if err := db.QueryRow(q, args...).Scan(&s); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	return s
}

// process runs the real pipeline (fake provider) until the entry is ready.
func process(t *testing.T, db *sql.DB, id string) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	w := &worker.Worker{DB: db, Steps: worker.DefaultSteps(db, provider.Fake{}), Poll: 5 * time.Millisecond}
	go func() { _ = w.Run(ctx); close(done) }()
	defer func() { cancel(); <-done }()
	for i := 0; i < 400; i++ {
		if scalar(t, db, `SELECT processing_status FROM entries WHERE id=?`, id) == "ready" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("entry never became ready")
}

func TestEditKeepsIdentityKeepsOneVersionAndReprocessesWithoutDuplicates(t *testing.T) {
	svc, _, _ := newSvc(t)
	clock := time.Date(2026, 3, 4, 10, 0, 0, 0, time.UTC)
	svc.Now = func() time.Time { return clock }
	saved, err := svc.Save(context.Background(), entries.Input{Title: "First", Description: "v1 body", Tags: []string{"a"}, Timezone: "Europe/Rome", JournalDate: "2026-03-04"})
	if err != nil {
		t.Fatal(err)
	}
	process(t, svc.DB, saved.ID)
	count := func() string {
		return scalar(t, svc.DB, `SELECT (SELECT COUNT(*) FROM entities)||','||(SELECT COUNT(*) FROM chunks)||','||(SELECT COUNT(*) FROM chunk_embeddings)||','||(SELECT COUNT(*) FROM entity_relations)||','||(SELECT COUNT(*) FROM entry_artifacts)||','||(SELECT COUNT(*) FROM chunks_fts)`)
	}
	before := count()

	v1, _ := os.ReadFile(saved.Path)
	for i, body := range []string{"v2 body", "v3 body"} { // two edits: only the latest previous version remains
		clock = clock.Add(time.Hour)
		if _, err := svc.Update(context.Background(), saved.ID, entries.Input{Title: "Second", Description: body, Tags: []string{"b", "B"}}); err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			if n := scalar(t, svc.DB, `SELECT COUNT(*) FROM jobs WHERE state IN ('queued','running')`); n != "1" {
				t.Fatalf("active jobs after edit = %s", n)
			}
		}
	}
	// A second edit while already queued must not duplicate the job.
	if n := scalar(t, svc.DB, `SELECT COUNT(*) FROM jobs WHERE state IN ('queued','running')`); n != "1" {
		t.Fatalf("active jobs = %s", n)
	}

	v, err := svc.Get(context.Background(), saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	if v.Title != "Second" || v.Body != "v3 body" || strings.Join(v.Tags, ",") != "b" || v.ProcessingStatus != "pending" {
		t.Fatalf("view after edit: %+v", v)
	}
	if v.CreatedAt != "2026-03-04T10:00:00Z" || v.UpdatedAt != "2026-03-04T12:00:00Z" {
		t.Fatalf("timestamps created=%s updated=%s", v.CreatedAt, v.UpdatedAt)
	}
	if v.Timezone != "Europe/Rome" || v.JournalDate != "2026-03-04" {
		t.Fatalf("omitted timezone/date not kept: %+v", v)
	}
	cur, _ := os.ReadFile(saved.Path)
	for _, want := range []string{"id: " + saved.ID, "created_at: \"2026-03-04T10:00:00Z\"", "updated_at: \"2026-03-04T12:00:00Z\"", "v3 body"} {
		if !strings.Contains(string(cur), want) {
			t.Errorf("file missing %q:\n%s", want, cur)
		}
	}
	versions, _ := filepath.Glob(filepath.Join(filepath.Dir(saved.Path), "versions", "*"))
	if len(versions) != 1 || filepath.Base(versions[0]) != "20260304T120000Z_"+saved.ID+".previous.md" {
		t.Fatalf("versions = %v", versions)
	}
	prev, _ := os.ReadFile(versions[0])
	if !strings.Contains(string(prev), "v2 body") || strings.Contains(string(v1), "v2 body") {
		t.Errorf("previous version is not the one before the last edit:\n%s", prev)
	}

	process(t, svc.DB, saved.ID)
	if after := count(); after != before {
		t.Errorf("derived data duplicated or lost: before %s after %s", before, after)
	}
	if got := scalar(t, svc.DB, `SELECT content FROM entry_artifacts WHERE kind='summary'`); !strings.Contains(got, "v3 body") {
		t.Errorf("summary not regenerated: %s", got)
	}
}

func TestEditValidatesLikeCreateAndLeavesEntryUntouched(t *testing.T) {
	svc, _, _ := newSvc(t)
	saved, err := svc.Save(context.Background(), entries.Input{Title: "Keep", Description: "orig"})
	if err != nil {
		t.Fatal(err)
	}
	var ve *entries.ValidationError
	if _, err := svc.Update(context.Background(), saved.ID, entries.Input{Title: " "}); !errors.As(err, &ve) || ve.Field != "title" {
		t.Fatalf("err = %v", err)
	}
	if _, err := svc.Update(context.Background(), "nope", entries.Input{Title: "x"}); !errors.Is(err, entries.ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
	raw, _ := os.ReadFile(saved.Path)
	if !strings.Contains(string(raw), "orig") {
		t.Errorf("file changed by rejected edit")
	}
	if m, _ := filepath.Glob(filepath.Join(filepath.Dir(saved.Path), "versions", "*")); len(m) != 0 {
		t.Errorf("version written for rejected edit: %v", m)
	}
}
