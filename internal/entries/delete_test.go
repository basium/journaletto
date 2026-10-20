package entries_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"journaletto/internal/entries"
)

func TestDeleteRestoreRewritesFileIdempotentlyKeepingDerivedDataAndVersions(t *testing.T) {
	svc, _, _ := newSvc(t)
	clock := time.Date(2026, 3, 4, 10, 0, 0, 0, time.UTC)
	svc.Now = func() time.Time { return clock }
	ctx := context.Background()
	saved, err := svc.Save(ctx, entries.Input{Title: "T", Description: "body text", Tags: []string{"a"}})
	if err != nil {
		t.Fatal(err)
	}
	process(t, svc.DB, saved.ID)
	clock = clock.Add(time.Hour)
	if _, err := svc.Update(ctx, saved.ID, entries.Input{Title: "T2", Description: "body v2"}); err != nil {
		t.Fatal(err)
	}
	process(t, svc.DB, saved.ID)
	versions, _ := filepath.Glob(filepath.Join(filepath.Dir(saved.Path), "versions", "*"))
	prevBefore, _ := os.ReadFile(versions[0])
	derived := func() string {
		return scalar(t, svc.DB, `SELECT (SELECT COUNT(*) FROM entities)||','||(SELECT COUNT(*) FROM chunks)||','||(SELECT COUNT(*) FROM entry_artifacts)||','||(SELECT COUNT(*) FROM chunk_embeddings)`)
	}
	before := derived()

	clock = clock.Add(time.Hour)
	if err := svc.Delete(ctx, saved.ID); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(saved.Path)
	for _, want := range []string{"status: deleted\n", "deleted_at: \"2026-03-04T12:00:00Z\"\n", "updated_at: \"2026-03-04T12:00:00Z\"", "\n---\n\n# T2\n\nbody v2\n"} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("deleted file missing %q:\n%s", want, raw)
		}
	}
	if strings.Index(string(raw), "status:") > strings.Index(string(raw), "deleted_at:") {
		t.Errorf("deleted_at must follow status:\n%s", raw)
	}
	if got := scalar(t, svc.DB, `SELECT status||'|'||deleted_at||'|'||updated_at FROM entries`); got != "deleted|2026-03-04T12:00:00Z|2026-03-04T12:00:00Z" {
		t.Errorf("row = %s", got)
	}
	// Idempotent: a second delete changes nothing.
	clock = clock.Add(time.Hour)
	if err := svc.Delete(ctx, saved.ID); err != nil {
		t.Fatal(err)
	}
	if again, _ := os.ReadFile(saved.Path); string(again) != string(raw) {
		t.Errorf("second delete rewrote file:\n%s", again)
	}
	if _, err := svc.Update(ctx, saved.ID, entries.Input{Title: "X"}); !errors.Is(err, entries.ErrDeleted) {
		t.Errorf("update of deleted entry: %v", err)
	}

	clock = clock.Add(time.Hour)
	if err := svc.Restore(ctx, saved.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.Restore(ctx, saved.ID); err != nil {
		t.Fatal(err)
	}
	raw, _ = os.ReadFile(saved.Path)
	if !strings.Contains(string(raw), "status: active\n") || strings.Contains(string(raw), "deleted_at") || !strings.Contains(string(raw), "updated_at: \"2026-03-04T14:00:00Z\"") {
		t.Errorf("restored file:\n%s", raw)
	}
	if got := scalar(t, svc.DB, `SELECT status||'|'||deleted_at FROM entries`); got != "active|" {
		t.Errorf("row = %s", got)
	}
	if derived() != before {
		t.Errorf("derived data changed: %s -> %s", before, derived())
	}
	if prevAfter, _ := os.ReadFile(versions[0]); string(prevAfter) != string(prevBefore) {
		t.Error("previous version touched")
	}
	if err := svc.Delete(ctx, "missing"); !errors.Is(err, entries.ErrNotFound) {
		t.Errorf("unknown id: %v", err)
	}
}

func TestDeleteCommitFailureLeavesFileAndRowConsistent(t *testing.T) {
	svc, _, _ := newSvc(t)
	ctx := context.Background()
	saved, err := svc.Save(ctx, entries.Input{Title: "Keep", Description: "orig"})
	if err != nil {
		t.Fatal(err)
	}
	orig, _ := os.ReadFile(saved.Path)
	svc.Commit = func(*sql.Tx) error { return errors.New("commit failed") }
	if err := svc.Delete(ctx, saved.ID); err == nil || !strings.Contains(err.Error(), "commit failed") {
		t.Fatalf("err = %v", err)
	}
	if now, _ := os.ReadFile(saved.Path); string(now) != string(orig) {
		t.Errorf("file not restored:\n%s", now)
	}
	if got := scalar(t, svc.DB, `SELECT status FROM entries`); got != "active" {
		t.Errorf("row status = %s", got)
	}
}

func TestConcurrentDeleteAndEditNeverResurrectAFileMarkedDeleted(t *testing.T) {
	svc, _, _ := newSvc(t)
	ctx := context.Background()
	saved, err := svc.Save(ctx, entries.Input{Title: "Race", Description: "orig"})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); _ = svc.Delete(ctx, saved.ID) }()
	go func() { defer wg.Done(); _, _ = svc.Update(ctx, saved.ID, entries.Input{Title: "Edited", Description: "new"}) }()
	wg.Wait()
	raw, _ := os.ReadFile(saved.Path)
	row := scalar(t, svc.DB, `SELECT status FROM entries`)
	if row != "deleted" && row != "active" || strings.Contains(string(raw), "status: "+row+"\n") == false {
		t.Errorf("file and row disagree (row %s):\n%s", row, raw)
	}
}
