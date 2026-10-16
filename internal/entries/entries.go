// Package entries validates and saves journal entries as raw Markdown plus canonical SQLite rows.
package entries

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
)

const (
	MaxTitle   = 200
	MaxBody    = 100 * 1024
	MaxTags    = 100
	MaxTagLen  = 50
	dateLayout = "2006-01-02"
)

// Input is an unvalidated entry submission.
type Input struct {
	Title       string   `json:"title"`
	JournalDate string   `json:"journal_date"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	Timezone    string   `json:"timezone"`
}

// ValidationError identifies the offending field.
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (e *ValidationError) Error() string { return e.Field + ": " + e.Message }

// Entry is a saved entry.
type Entry struct {
	ID     string `json:"id"`
	Path   string `json:"path"`
	Status string `json:"status"`
}

// Service saves entries.
type Service struct {
	DB       *sql.DB
	DataRoot string
	UserID   string
	Now      func() time.Time
}

// frontMatter holds only user-authored data; generated data must never be written here.
type frontMatter struct {
	SchemaVersion int      `yaml:"schema_version"`
	ID            string   `yaml:"id"`
	Title         string   `yaml:"title"`
	JournalDate   string   `yaml:"journal_date"`
	CreatedAt     string   `yaml:"created_at"`
	UpdatedAt     string   `yaml:"updated_at"`
	Timezone      string   `yaml:"timezone"`
	Tags          []string `yaml:"tags"`
	Status        string   `yaml:"status"`
}

func normalize(in Input, now time.Time) (Input, error) {
	in.Title = strings.TrimSpace(in.Title)
	if n := utf8.RuneCountInString(in.Title); n < 1 || n > MaxTitle {
		return in, &ValidationError{"title", fmt.Sprintf("must be 1-%d characters", MaxTitle)}
	}
	if len(in.Description) > MaxBody {
		return in, &ValidationError{"description", "must be at most 100 KB"}
	}
	in.Timezone = strings.TrimSpace(in.Timezone)
	loc := time.UTC
	if in.Timezone == "" {
		in.Timezone = "UTC"
	} else if l, err := time.LoadLocation(in.Timezone); err != nil || in.Timezone == "Local" {
		return in, &ValidationError{"timezone", "unknown timezone"}
	} else {
		loc = l
	}
	in.JournalDate = strings.TrimSpace(in.JournalDate)
	if in.JournalDate == "" {
		in.JournalDate = now.In(loc).Format(dateLayout)
	} else if d, err := time.Parse(dateLayout, in.JournalDate); err != nil || d.Format(dateLayout) != in.JournalDate {
		return in, &ValidationError{"journal_date", "must be YYYY-MM-DD"}
	}
	seen := map[string]bool{}
	tags := []string{}
	for _, t := range in.Tags {
		t = strings.ToLower(strings.TrimSpace(t))
		if t == "" || seen[t] {
			continue
		}
		if utf8.RuneCountInString(t) > MaxTagLen {
			return in, &ValidationError{"tags", fmt.Sprintf("each tag must be 1-%d characters", MaxTagLen)}
		}
		seen[t] = true
		tags = append(tags, t)
	}
	if len(tags) > MaxTags {
		return in, &ValidationError{"tags", fmt.Sprintf("at most %d tags", MaxTags)}
	}
	in.Tags = tags
	return in, nil
}

// Save validates in, then stores the entry canonically and as an atomically written .md file.
// It never depends on AI processing; the entry is left with processing status "pending".
func (s *Service) Save(ctx context.Context, in Input) (Entry, error) {
	now := time.Now
	if s.Now != nil {
		now = s.Now
	}
	t := now().UTC()
	in, err := normalize(in, t)
	if err != nil {
		return Entry{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return Entry{}, err
	}
	ts := t.Format(time.RFC3339)
	dir := filepath.Join(s.DataRoot, "journals", s.UserID, t.Format(dateLayout))
	path := filepath.Join(dir, t.Format("20060102T150405Z")+"_"+id.String()+".md")

	fm, err := yaml.Marshal(frontMatter{1, id.String(), in.Title, in.JournalDate, ts, ts, in.Timezone, in.Tags, "active"})
	if err != nil {
		return Entry{}, err
	}
	content := "---\n" + string(fm) + "---\n\n# " + in.Title + "\n\n" + in.Description
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return Entry{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO entries(id,user_id,title,body,journal_date,created_at,updated_at,timezone,status,processing_status,file_path)
		VALUES (?,?,?,?,?,?,?,?,'active','pending',?)`,
		id.String(), s.UserID, in.Title, in.Description, in.JournalDate, ts, ts, in.Timezone, path); err != nil {
		return Entry{}, err
	}
	for _, tag := range in.Tags {
		if _, err := tx.ExecContext(ctx, `INSERT INTO entry_tags(entry_id,user_id,tag) VALUES (?,?,?)`, id.String(), s.UserID, tag); err != nil {
			return Entry{}, err
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO entries_fts(entry_id,title,body,tags) VALUES (?,?,?,?)`,
		id.String(), in.Title, in.Description, strings.Join(in.Tags, " ")); err != nil {
		return Entry{}, err
	}
	if err := writeAtomic(dir, path, []byte(content)); err != nil {
		return Entry{}, err
	}
	if err := tx.Commit(); err != nil {
		os.Remove(path)
		return Entry{}, err
	}
	return Entry{ID: id.String(), Path: path, Status: "pending"}, nil
}

func writeAtomic(dir, path string, data []byte) error {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp, 0o640); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// SuggestTags returns existing tags of the user starting with prefix.
func (s *Service) SuggestTags(ctx context.Context, prefix string) ([]string, error) {
	prefix = strings.ToLower(strings.TrimSpace(prefix))
	esc := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(prefix)
	rows, err := s.DB.QueryContext(ctx, `SELECT DISTINCT tag FROM entry_tags WHERE user_id=? AND tag LIKE ? ESCAPE '\' ORDER BY tag LIMIT 10`, s.UserID, esc+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
