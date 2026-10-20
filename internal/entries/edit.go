package entries

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// ErrNotFound is returned for an unknown entry (or one owned by another user).
var ErrNotFound = errors.New("entry not found")

// ErrDeleted is returned when editing a soft-deleted entry; restore it first.
var ErrDeleted = errors.New("entry is deleted")

// View is an entry as shown on its page: the raw user data plus generated metadata around it.
type View struct {
	ID               string
	Title            string
	Body             string
	JournalDate      string
	Timezone         string
	CreatedAt        string // RFC3339 UTC
	UpdatedAt        string // RFC3339 UTC
	Tags             []string
	ProcessingStatus string
	Summary          string
	InternalTags     []string
	Status           string // active | deleted
	DeletedAt        string // RFC3339 UTC; empty unless deleted
	Deleted          bool
}

// Get loads one of the user's entries with its generated metadata.
func (s *Service) Get(ctx context.Context, id string) (View, error) {
	v := View{ID: id, Tags: []string{}, InternalTags: []string{}}
	err := s.DB.QueryRowContext(ctx, `SELECT title,body,journal_date,timezone,created_at,updated_at,processing_status,status,deleted_at
		FROM entries WHERE id=? AND user_id=?`, id, s.UserID).
		Scan(&v.Title, &v.Body, &v.JournalDate, &v.Timezone, &v.CreatedAt, &v.UpdatedAt, &v.ProcessingStatus, &v.Status, &v.DeletedAt)
	v.Deleted = v.Status == "deleted"
	if errors.Is(err, sql.ErrNoRows) {
		return View{}, ErrNotFound
	}
	if err != nil {
		return View{}, err
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT tag FROM entry_tags WHERE entry_id=? ORDER BY tag`, id)
	if err != nil {
		return View{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return View{}, err
		}
		v.Tags = append(v.Tags, t)
	}
	if err := rows.Err(); err != nil {
		return View{}, err
	}
	arts, err := s.DB.QueryContext(ctx, `SELECT kind,content FROM entry_artifacts WHERE entry_id=?`, id)
	if err != nil {
		return View{}, err
	}
	defer arts.Close()
	for arts.Next() {
		var kind, content string
		if err := arts.Scan(&kind, &content); err != nil {
			return View{}, err
		}
		switch kind {
		case "summary":
			v.Summary = content
		case "internal_tags":
			_ = json.Unmarshal([]byte(content), &v.InternalTags)
		}
	}
	return v, arts.Err()
}

// Update validates in like Save, then rewrites the entry in place: id, file name and created_at
// are kept, updated_at advances, exactly one previous version is kept under versions/, and
// processing is re-queued (derived data is replaced when the worker's stages rerun).
func (s *Service) Update(ctx context.Context, id string, in Input) (Entry, error) {
	now := time.Now
	if s.Now != nil {
		now = s.Now
	}
	t := now().UTC()
	maxBody := s.MaxBody
	if maxBody <= 0 {
		maxBody = DefaultMaxBody
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var path, createdAt, oldDate, oldTZ, status string
	err := s.DB.QueryRowContext(ctx, `SELECT file_path,created_at,journal_date,timezone,status FROM entries WHERE id=? AND user_id=?`, id, s.UserID).
		Scan(&path, &createdAt, &oldDate, &oldTZ, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return Entry{}, ErrNotFound
	}
	if err != nil {
		return Entry{}, err
	}
	if status == "deleted" {
		return Entry{}, ErrDeleted
	}
	// Omitted date/timezone keep their stored values rather than resetting to defaults.
	if strings.TrimSpace(in.JournalDate) == "" {
		in.JournalDate = oldDate
	}
	if strings.TrimSpace(in.Timezone) == "" {
		in.Timezone = oldTZ
	}
	in, err = normalize(in, t, maxBody)
	if err != nil {
		return Entry{}, err
	}
	ts := t.Format(time.RFC3339)
	fm, err := yaml.Marshal(frontMatter{1, id, in.Title, in.JournalDate, createdAt, ts, in.Timezone, in.Tags, "active", ""})
	if err != nil {
		return Entry{}, err
	}
	content := "---\n" + string(fm) + "---\n\n# " + in.Title + "\n\n" + in.Description
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	prev, err := os.ReadFile(path)
	if err != nil {
		return Entry{}, err
	}
	dir := filepath.Dir(path)
	vdir := filepath.Join(dir, "versions")
	vpath := filepath.Join(vdir, t.Format("20060102T150405Z")+"_"+id+".previous.md")

	tmp, err := stage(dir, []byte(content))
	if err != nil {
		return Entry{}, err
	}
	defer os.Remove(tmp)
	vtmp, err := stage(vdir, prev)
	if err != nil {
		return Entry{}, err
	}
	defer os.Remove(vtmp)

	rename := os.Rename
	if s.Rename != nil {
		rename = s.Rename
	}
	if err := rename(vtmp, vpath); err != nil {
		return Entry{}, err
	}
	old, _ := filepath.Glob(filepath.Join(vdir, "*_"+id+".previous.md"))
	for _, o := range old {
		if o != vpath {
			os.Remove(o)
		}
	}
	if err := rename(tmp, path); err != nil {
		return Entry{}, err
	}
	if err := s.replace(ctx, id, in, ts); err != nil {
		// Commit failed: put the previous content back so file and row agree.
		if rerr := restore(dir, path, prev, rename); rerr != nil {
			return Entry{}, errors.Join(err, fmt.Errorf("restoring previous content of %s (file and database now disagree): %w", path, rerr))
		}
		return Entry{}, err
	}
	return Entry{ID: id, Path: path, Status: "pending"}, nil
}

// restore atomically puts prev back at path, cleaning up its temp file if the rename fails.
func restore(dir, path string, prev []byte, rename func(string, string) error) error {
	rtmp, err := stage(dir, prev)
	if err != nil {
		return err
	}
	if err := rename(rtmp, path); err != nil {
		return errors.Join(err, removeIfExists(rtmp))
	}
	return nil
}

func removeIfExists(p string) error {
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (s *Service) replace(ctx context.Context, id string, in Input, ts string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE entries SET title=?,body=?,journal_date=?,timezone=?,updated_at=?,processing_status='pending' WHERE id=?`,
		in.Title, in.Description, in.JournalDate, in.Timezone, ts, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM entry_tags WHERE entry_id=?`, id); err != nil {
		return err
	}
	for _, tag := range in.Tags {
		if _, err := tx.ExecContext(ctx, `INSERT INTO entry_tags(entry_id,user_id,tag) VALUES (?,?,?)`, id, s.UserID, tag); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM entries_fts WHERE entry_id=?`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO entries_fts(entry_id,title,body,tags) VALUES (?,?,?,?)`,
		id, in.Title, in.Description, strings.Join(in.Tags, " ")); err != nil {
		return err
	}
	// One active job per entry. A running job saw the old text, so it is reset to queued; the
	// worker ignores the outcome of a job it no longer owns and the rerun replaces derived data.
	if _, err := tx.ExecContext(ctx, `UPDATE jobs SET state='queued', run_after=0, attempts=0 WHERE entry_id=? AND state IN ('queued','running')`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO jobs(entry_id,state,run_after) SELECT ?,'queued',0
		WHERE NOT EXISTS (SELECT 1 FROM jobs WHERE entry_id=? AND state IN ('queued','running'))`, id, id); err != nil {
		return err
	}
	if s.Commit != nil {
		return s.Commit(tx)
	}
	return tx.Commit()
}
