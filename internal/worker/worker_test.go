package worker_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
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
	start(t, &worker.Worker{DB: db, Steps: worker.DefaultSteps(provider.Fake{}), MaxRetries: 3})
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
	start(t, &worker.Worker{DB: db, Steps: worker.DefaultSteps(provider.Fake{}), MaxRetries: 3})
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

func TestSaveSucceedsWhenProviderIsDown(t *testing.T) {
	db, svc := setup(t)
	p, err := provider.New("external") // placeholder adapter: always unavailable
	if err != nil {
		t.Fatal(err)
	}
	start(t, &worker.Worker{DB: db, Steps: worker.DefaultSteps(p), MaxRetries: 1, Backoff: time.Millisecond})
	id := save(t, svc) // must not error
	waitStatus(t, db, id, "failed")
}
