// Package journal implements the core domain logic for journal entries:
// validation, Markdown rendering and atomic filesystem persistence. It has
// no knowledge of HTTP.
package journal

import "time"

// Entry is a validated journal entry ready to be rendered and saved.
type Entry struct {
	ID          string
	Title       string
	JournalDate string // YYYY-MM-DD, the day the entry represents
	Description string
	Tags        []string
	Timezone    string // IANA timezone name, captured from the client
	CreatedAt   time.Time
}

// FrontMatter is the YAML front matter written at the top of every raw
// Markdown journal file, per requirement JE-004. Field order matches the
// spec's example exactly, since yaml.v3 marshals struct fields in
// declaration order.
type FrontMatter struct {
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

const schemaVersion = 1

// StatusActive and StatusDeleted are the only valid front-matter status
// values (JE-004). Only StatusActive is produced by this slice; soft
// deletion (JE-011) is a later slice.
const (
	StatusActive  = "active"
	StatusDeleted = "deleted"
)

// ToFrontMatter builds the front matter for a freshly created entry, where
// created_at and updated_at are identical.
func (e Entry) ToFrontMatter() FrontMatter {
	createdAt := e.CreatedAt.UTC().Format(time.RFC3339)
	return FrontMatter{
		SchemaVersion: schemaVersion,
		ID:            e.ID,
		Title:         e.Title,
		JournalDate:   e.JournalDate,
		CreatedAt:     createdAt,
		UpdatedAt:     createdAt,
		Timezone:      e.Timezone,
		Tags:          e.Tags,
		Status:        StatusActive,
	}
}
