// Package worker runs the in-process background processing of saved entries.
//
// Entries move pending -> processing -> ready|failed. Jobs live in the SQLite jobs table,
// so they survive restarts. Processing is a list of pluggable Steps run in order.
package worker

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"
)

// Entry is what a Step gets to work on.
type Entry struct {
	ID, UserID, Title, Body, Path string
}

// Step is one processing stage (e.g. embedding, tagging). Later tasks append their own.
type Step struct {
	Name string
	Run  func(ctx context.Context, e Entry) error
}

// ErrNotFound is returned by Resync for an unknown entry.
var ErrNotFound = errors.New("entry not found")

// Worker claims jobs and runs Steps. Zero values get sensible defaults.
type Worker struct {
	DB          *sql.DB
	Steps       []Step
	Concurrency int           // default 1
	MaxRetries  int           // retries after the first attempt before failing
	Backoff     time.Duration // base delay; doubles per retry (default 1s)
	Poll        time.Duration // idle poll interval (default 250ms)
	Now         func() time.Time
}

func (w *Worker) now() time.Time {
	if w.Now != nil {
		return w.Now()
	}
	return time.Now()
}

// Run sweeps unfinished entries into the queue, then processes jobs until ctx is done.
func (w *Worker) Run(ctx context.Context) error {
	if err := Sweep(ctx, w.DB); err != nil {
		return fmt.Errorf("startup sweep: %w", err)
	}
	n := w.Concurrency
	if n < 1 {
		n = 1
	}
	poll := w.Poll
	if poll <= 0 {
		poll = 250 * time.Millisecond
	}
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ctx.Err() == nil {
				ok, err := w.runOne(ctx)
				if err != nil && ctx.Err() == nil {
					log.Printf("worker: %v", err)
				}
				if ok && err == nil {
					continue
				}
				select {
				case <-ctx.Done():
				case <-time.After(poll):
				}
			}
		}()
	}
	wg.Wait()
	return nil
}

// runOne claims and processes one due job; ok is false when nothing was due.
func (w *Worker) runOne(ctx context.Context) (ok bool, err error) {
	var jobID int64
	var e Entry
	var attempts int
	err = w.DB.QueryRowContext(ctx, `UPDATE jobs SET state='running', attempts=attempts+1
		WHERE id=(SELECT id FROM jobs WHERE state='queued' AND run_after<=? ORDER BY id LIMIT 1)
		RETURNING id, entry_id, attempts`, w.now().UnixMilli()).Scan(&jobID, &e.ID, &attempts)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if _, err := w.DB.ExecContext(ctx, `UPDATE entries SET processing_status='processing' WHERE id=?`, e.ID); err != nil {
		return true, err
	}
	if err := w.DB.QueryRowContext(ctx, `SELECT user_id,title,body,file_path FROM entries WHERE id=?`, e.ID).
		Scan(&e.UserID, &e.Title, &e.Body, &e.Path); err != nil {
		return true, err
	}

	runErr := w.process(ctx, e)
	if runErr != nil && ctx.Err() != nil {
		return true, nil // shutting down: leave the job running; the next startup sweep requeues it
	}
	// Record the outcome even if ctx is cancelled meanwhile.
	bg := context.WithoutCancel(ctx)
	switch {
	case runErr == nil:
		return true, w.finish(bg, jobID, e.ID, "done", "ready", "", 0)
	case attempts > w.MaxRetries:
		log.Printf("worker: entry %s failed after %d attempts: %v", e.ID, attempts, runErr)
		return true, w.finish(bg, jobID, e.ID, "failed", "failed", runErr.Error(), 0)
	default:
		base := w.Backoff
		if base <= 0 {
			base = time.Second
		}
		delay := base << (attempts - 1) // base, 2*base, 4*base ...
		return true, w.finish(bg, jobID, e.ID, "queued", "pending", runErr.Error(), w.now().Add(delay).UnixMilli())
	}
}

func (w *Worker) process(ctx context.Context, e Entry) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	for _, s := range w.Steps {
		if err := s.Run(ctx, e); err != nil {
			return fmt.Errorf("step %s: %w", s.Name, err)
		}
	}
	return nil
}

func (w *Worker) finish(ctx context.Context, jobID int64, entryID, jobState, entryStatus, lastErr string, runAfter int64) error {
	if _, err := w.DB.ExecContext(ctx, `UPDATE jobs SET state=?, last_error=?, run_after=? WHERE id=?`, jobState, lastErr, runAfter, jobID); err != nil {
		return err
	}
	_, err := w.DB.ExecContext(ctx, `UPDATE entries SET processing_status=? WHERE id=?`, entryStatus, entryID)
	return err
}

// Sweep requeues work that was interrupted or never finished: running jobs from a crashed
// process, and pending/processing/failed entries that have no active job. Statements are
// idempotent, so a crash mid-sweep is repaired by the next one.
func Sweep(ctx context.Context, db *sql.DB) error {
	for _, q := range []string{
		`UPDATE jobs SET state='queued', run_after=0 WHERE state='running'`,
		`INSERT INTO jobs(entry_id,state,run_after) SELECT id,'queued',0 FROM entries
			WHERE processing_status IN ('pending','processing','failed')
			AND NOT EXISTS (SELECT 1 FROM jobs j WHERE j.entry_id=entries.id AND j.state IN ('queued','running'))`,
		`UPDATE entries SET processing_status='pending' WHERE processing_status IN ('processing','failed')`,
	} {
		if _, err := db.ExecContext(ctx, q); err != nil {
			return err
		}
	}
	return nil
}

// Resync requeues processing of one of the user's entries. A queued job is made due now with
// fresh retries; a running job is left alone.
func Resync(ctx context.Context, db *sql.DB, userID, entryID string) error {
	var one int
	err := db.QueryRowContext(ctx, `SELECT 1 FROM entries WHERE id=? AND user_id=?`, entryID, userID).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	// Order matters: set the status before the job can be claimed, otherwise a fast worker
	// could finish first and this would overwrite its final status with "pending".
	if _, err := db.ExecContext(ctx, `UPDATE entries SET processing_status='pending' WHERE id=? AND processing_status<>'processing'`, entryID); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `UPDATE jobs SET run_after=0, attempts=0 WHERE entry_id=? AND state='queued'`, entryID); err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `INSERT INTO jobs(entry_id,state,run_after) SELECT ?,'queued',0
		WHERE NOT EXISTS (SELECT 1 FROM jobs WHERE entry_id=? AND state IN ('queued','running'))`, entryID, entryID)
	return err
}

// Status returns the processing status of one of the user's entries.
func Status(ctx context.Context, db *sql.DB, userID, entryID string) (string, error) {
	var s string
	err := db.QueryRowContext(ctx, `SELECT processing_status FROM entries WHERE id=? AND user_id=?`, entryID, userID).Scan(&s)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return s, err
}
