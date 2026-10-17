package worker_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"journaletto/internal/entries"
	"journaletto/internal/provider"
	"journaletto/internal/store"
	"journaletto/internal/worker"
)

func setup(t *testing.T) (*sql.DB, *entries.Service) {
	t.Helper()
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db, &entries.Service{DB: db, DataRoot: dir, UserID: "u"}
}

func save(t *testing.T, svc *entries.Service) string {
	t.Helper()
	e, err := svc.Save(context.Background(), entries.Input{Title: "t", Description: "body"})
	if err != nil {
		t.Fatal(err)
	}
	return e.ID
}

func status(t *testing.T, db *sql.DB, id string) string {
	t.Helper()
	s, err := worker.Status(context.Background(), db, "u", id)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// start runs w in the background and stops it when the test ends.
func start(t *testing.T, w *worker.Worker) {
	t.Helper()
	w.Poll = time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); w.Run(ctx) }()
	t.Cleanup(func() { cancel(); <-done })
}

func waitStatus(t *testing.T, db *sql.DB, id, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if status(t, db, id) == want {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("status %q, want %q", status(t, db, id), want)
}

func TestFakeProviderEntryReachesReady(t *testing.T) {
	db, svc := setup(t)
	id := save(t, svc)
	if got := status(t, db, id); got != "pending" {
		t.Fatalf("after save: %s", got)
	}
	start(t, &worker.Worker{DB: db, Steps: worker.DefaultSteps(db, provider.Fake{}), MaxRetries: 3})
	waitStatus(t, db, id, "ready")
}

func TestFailuresRetryThreeTimesWithBackoffThenFail(t *testing.T) {
	db, svc := setup(t)
	id := save(t, svc)
	var calls atomic.Int32
	var mu sync.Mutex
	var stamps []time.Time
	boom := worker.Step{Name: "boom", Run: func(context.Context, worker.Entry) error {
		calls.Add(1)
		mu.Lock()
		stamps = append(stamps, time.Now())
		mu.Unlock()
		return errors.New("provider down")
	}}
	start(t, &worker.Worker{DB: db, Steps: []worker.Step{boom}, MaxRetries: 3, Backoff: 10 * time.Millisecond})
	waitStatus(t, db, id, "failed")
	time.Sleep(60 * time.Millisecond) // no further attempts
	if n := calls.Load(); n != 4 {
		t.Fatalf("attempts = %d, want 1 + 3 retries", n)
	}
	mu.Lock()
	defer mu.Unlock()
	// delays double: >=10ms, >=20ms, >=40ms
	for i, min := range []time.Duration{10, 20, 40} {
		if d := stamps[i+1].Sub(stamps[i]); d < min*time.Millisecond {
			t.Errorf("gap %d = %v, want >= %vms", i, d, min)
		}
	}
}

func TestSweepAndResyncRequeueFailedAndPendingWhenProviderWasDown(t *testing.T) {
	db, svc := setup(t)
	failed, pending, stuck := save(t, svc), save(t, svc), save(t, svc)
	// Simulate: one entry failed earlier, one never got a job, one was mid-processing at a crash.
	for _, q := range []string{
		`UPDATE entries SET processing_status='failed' WHERE id='` + failed + `'`,
		`UPDATE jobs SET state='failed' WHERE entry_id='` + failed + `'`,
		`DELETE FROM jobs WHERE entry_id='` + pending + `'`,
		`UPDATE entries SET processing_status='processing' WHERE id='` + stuck + `'`,
		`UPDATE jobs SET state='running' WHERE entry_id='` + stuck + `'`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	start(t, &worker.Worker{DB: db, Steps: worker.DefaultSteps(db, provider.Fake{}), MaxRetries: 3})
	for _, id := range []string{failed, pending, stuck} {
		waitStatus(t, db, id, "ready")
	}

	// Resync a ready entry: it goes back through the pipeline; unknown ids are not found.
	if err := worker.Resync(context.Background(), db, "u", failed); err != nil {
		t.Fatal(err)
	}
	waitStatus(t, db, failed, "ready")
	if err := worker.Resync(context.Background(), db, "u", "nope"); !errors.Is(err, worker.ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestProcessingStoresSummaryAndInternalTagsWithProvenanceNotInRawFile(t *testing.T) {
	db, svc := setup(t)
	id := save(t, svc)
	var path string
	if err := db.QueryRow(`SELECT file_path FROM entries WHERE id=?`, id).Scan(&path); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	start(t, &worker.Worker{DB: db, Steps: worker.DefaultSteps(db, provider.Fake{}), MaxRetries: 3})
	waitStatus(t, db, id, "ready")

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Error("raw file changed by processing")
	}
	for kind, version := range map[string]string{worker.KindSummary: worker.SummaryPromptVersion, worker.KindInternalTags: worker.TagsPromptVersion} {
		var content, prov, model, pv, at string
		err := db.QueryRow(`SELECT content,provider,model,prompt_version,generated_at FROM entry_artifacts WHERE entry_id=? AND kind=?`, id, kind).
			Scan(&content, &prov, &model, &pv, &at)
		if err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		if content == "" || prov != "fake" || model == "" || pv != version || at == "" {
			t.Errorf("%s: content=%q provider=%q model=%q version=%q at=%q", kind, content, prov, model, pv, at)
		}
		if strings.Contains(string(after), content) {
			t.Errorf("%s leaked into raw file", kind)
		}
	}
}

// stubLLM wraps the fake provider, overriding Complete.
type stubLLM struct {
	provider.Fake
	complete func(prompt string) string
}

func (s stubLLM) Complete(ctx context.Context, prompt string) (string, error) {
	if out := s.complete(prompt); out != "\x00" {
		return out, nil
	}
	return s.Fake.Complete(ctx, prompt)
}

func count(t *testing.T, db *sql.DB, q string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestEntitiesNormalisedDedupedRejectUnknownAndIdempotentOnReprocess(t *testing.T) {
	db, svc := setup(t)
	id := save(t, svc)
	llm := stubLLM{complete: func(p string) string {
		if !strings.HasPrefix(p, provider.EntityPromptPrefix) {
			return "\x00"
		}
		return "```json\n" + `[
			{"type":"people","value":"  Ada   Lovelace ","relations":[{"to":"journaletto","kind":"Works On"}]},
			{"type":"People","value":"ada lovelace","relations":[]},
			{"type":"Projects","value":"Journaletto","relations":[{"to":"Nowhere","kind":"x"}]},
			{"type":"Spaceships","value":"Enterprise","relations":[]},
			{"type":"Dates","value":"  ","relations":[]}]` + "\n```"
	}}
	start(t, &worker.Worker{DB: db, Steps: worker.DefaultSteps(db, llm), MaxRetries: 0})
	waitStatus(t, db, id, "ready")

	check := func() {
		t.Helper()
		if n := count(t, db, `SELECT COUNT(*) FROM entities WHERE entry_id=?`, id); n != 2 {
			t.Errorf("entities = %d, want 2", n)
		}
		var v, pv, prov, model, at string
		if err := db.QueryRow(`SELECT value,prompt_version,provider,model,generated_at FROM entities WHERE type='People'`).Scan(&v, &pv, &prov, &model, &at); err != nil {
			t.Fatal(err)
		}
		if v != "Ada Lovelace" || pv != worker.EntitiesPromptVersion || prov != "fake" || model == "" || at == "" {
			t.Errorf("people row: %q %q %q %q %q", v, pv, prov, model, at)
		}
		if n := count(t, db, `SELECT COUNT(*) FROM entity_relations WHERE kind='works on' AND prompt_version<>''`); n != 1 {
			t.Errorf("relations = %d, want 1 (dangling one dropped)", n)
		}
		if n := count(t, db, `SELECT COUNT(*) FROM entity_relations`); n != 1 {
			t.Errorf("total relations = %d, want 1", n)
		}
	}
	check()

	if err := worker.Resync(context.Background(), db, "u", id); err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(5 * time.Second); count(t, db, `SELECT COUNT(*) FROM jobs WHERE state='done'`) < 2 && time.Now().Before(deadline); {
		time.Sleep(2 * time.Millisecond)
	}
	waitStatus(t, db, id, "ready")
	check()
}

func TestBlankSummaryFailsTheStepInsteadOfReady(t *testing.T) {
	db, svc := setup(t)
	id := save(t, svc)
	llm := stubLLM{complete: func(p string) string {
		if strings.HasPrefix(p, "Summarise") {
			return " \n "
		}
		return "\x00"
	}}
	start(t, &worker.Worker{DB: db, Steps: worker.DefaultSteps(db, llm), MaxRetries: 1, Backoff: time.Millisecond})
	waitStatus(t, db, id, "failed")
	if n := count(t, db, `SELECT COUNT(*) FROM entry_artifacts WHERE entry_id=?`, id); n != 0 {
		t.Errorf("artifacts stored for blank summary: %d", n)
	}
}

func TestSweepFailureDoesNotStopWorkers(t *testing.T) {
	db, svc := setup(t)
	queued, orphan := save(t, svc), save(t, svc)
	// The orphan has no job, so the startup sweep tries to insert one; make that fail.
	for _, q := range []string{
		`DELETE FROM jobs WHERE entry_id='` + orphan + `'`,
		`CREATE TRIGGER sweep_boom BEFORE INSERT ON jobs BEGIN SELECT RAISE(ABORT,'boom'); END`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	start(t, &worker.Worker{DB: db, Steps: worker.DefaultSteps(db, provider.Fake{}), MaxRetries: 3})
	waitStatus(t, db, queued, "ready") // workers still ran despite the sweep error
}

func TestSaveSucceedsWhenProviderIsDown(t *testing.T) {
	db, svc := setup(t)
	p, err := provider.New("external") // placeholder adapter: always unavailable
	if err != nil {
		t.Fatal(err)
	}
	start(t, &worker.Worker{DB: db, Steps: worker.DefaultSteps(db, p), MaxRetries: 1, Backoff: time.Millisecond})
	id := save(t, svc) // must not error
	waitStatus(t, db, id, "failed")
}
