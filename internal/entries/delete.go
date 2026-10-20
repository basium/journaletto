package entries

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Delete soft-deletes an entry: the raw file is rewritten with status deleted and a UTC
// deleted_at, and the row follows. Derived data, versions and the file itself stay. Idempotent.
func (s *Service) Delete(ctx context.Context, id string) error { return s.setDeleted(ctx, id, true) }

// Restore reverses Delete (status active, deleted_at removed). Idempotent.
func (s *Service) Restore(ctx context.Context, id string) error { return s.setDeleted(ctx, id, false) }

func (s *Service) setDeleted(ctx context.Context, id string, del bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now
	if s.Now != nil {
		now = s.Now
	}
	t := now().UTC()
	var path, status, updatedAt string
	err := s.DB.QueryRowContext(ctx, `SELECT file_path,status,updated_at FROM entries WHERE id=? AND user_id=?`, id, s.UserID).
		Scan(&path, &status, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if (status == "deleted") == del {
		return nil
	}
	// updated_at must strictly advance even within the same second.
	if prev, perr := time.Parse(time.RFC3339, updatedAt); perr == nil && !t.Truncate(time.Second).After(prev) {
		t = prev.Add(time.Second)
	}
	ts := t.Format(time.RFC3339)
	newStatus, deletedAt := "active", ""
	if del {
		newStatus, deletedAt = "deleted", ts
	}

	prev, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	content, err := rewriteFrontMatter(prev, ts, newStatus, deletedAt)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	tmp, err := stage(dir, content)
	if err != nil {
		return err
	}
	defer os.Remove(tmp)
	rename := os.Rename
	if s.Rename != nil {
		rename = s.Rename
	}
	if err := rename(tmp, path); err != nil {
		return err
	}
	if err := s.setStatusRow(ctx, id, ts, newStatus, deletedAt); err != nil {
		if rerr := restore(dir, path, prev, rename); rerr != nil {
			return errors.Join(err, fmt.Errorf("restoring previous content of %s (file and database now disagree): %w", path, rerr))
		}
		return err
	}
	return nil
}

func (s *Service) setStatusRow(ctx context.Context, id, ts, status, deletedAt string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE entries SET status=?,deleted_at=?,updated_at=? WHERE id=?`, status, deletedAt, ts, id); err != nil {
		return err
	}
	if s.Commit != nil {
		return s.Commit(tx)
	}
	return tx.Commit()
}

// rewriteFrontMatter changes only status, deleted_at and updated_at; the Markdown body is kept verbatim.
func rewriteFrontMatter(raw []byte, updatedAt, status, deletedAt string) ([]byte, error) {
	text := string(raw)
	const open, closing = "---\n", "\n---\n"
	if !strings.HasPrefix(text, open) {
		return nil, errors.New("entry file has no front matter")
	}
	end := strings.Index(text[len(open)-1:], closing)
	if end < 0 {
		return nil, errors.New("entry file has unterminated front matter")
	}
	fmEnd := len(open) - 1 + end + 1 // index just after the newline ending the YAML
	var fm frontMatter
	if err := yaml.Unmarshal([]byte(text[len(open):fmEnd]), &fm); err != nil {
		return nil, err
	}
	fm.UpdatedAt, fm.Status, fm.DeletedAt = updatedAt, status, deletedAt
	out, err := yaml.Marshal(fm)
	if err != nil {
		return nil, err
	}
	return []byte(open + string(out) + text[fmEnd:]), nil
}
