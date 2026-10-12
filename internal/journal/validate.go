package journal

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Input is the raw, unvalidated data submitted for a new journal entry.
type Input struct {
	Title       string
	JournalDate string // optional; YYYY-MM-DD if provided
	Description string
	Tags        []string
	Timezone    string
}

// Normalized is Input after validation and normalization: trimmed strings,
// deduplicated tags and a resolved journal_date.
type Normalized struct {
	Title       string
	JournalDate string
	Description string
	Tags        []string
	Timezone    string
}

// ValidationError indicates a client-supplied value failed validation. It
// carries the offending field name so callers can build a structured error
// response.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

func validationErr(field, format string, args ...any) error {
	return &ValidationError{Field: field, Message: fmt.Sprintf(format, args...)}
}

// markdownImageRE matches standard Markdown image syntax, e.g. ![alt](url).
// Requirement JE-005 forbids embedded media; this slice's validation scope
// is limited to this syntax (see docs/decisions/0001-slice1-entry-save.md).
var markdownImageRE = regexp.MustCompile(`!\[[^\]]*\]\([^)]*\)`)

// ValidateAndNormalize validates raw input and returns its normalized form.
// now is the instant to treat as "the current time" when journal_date is
// omitted and must default to "today" in the client's timezone; callers
// should pass the same instant used for the entry's created_at so the two
// stay consistent.
func ValidateAndNormalize(in Input, maxTags int, now time.Time) (Normalized, error) {
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return Normalized{}, validationErr("title", "must not be empty")
	}

	description := strings.TrimSpace(in.Description)
	if description == "" {
		return Normalized{}, validationErr("description", "must not be empty")
	}
	if markdownImageRE.MatchString(description) {
		return Normalized{}, validationErr("description", "must not contain embedded media (Markdown image syntax)")
	}

	loc, err := resolveTimezone(in.Timezone)
	if err != nil {
		return Normalized{}, err
	}

	journalDate, err := resolveJournalDate(in.JournalDate, loc, now)
	if err != nil {
		return Normalized{}, err
	}

	tags, err := normalizeTags(in.Tags, maxTags)
	if err != nil {
		return Normalized{}, err
	}

	return Normalized{
		Title:       title,
		JournalDate: journalDate,
		Description: description,
		Tags:        tags,
		Timezone:    in.Timezone,
	}, nil
}

func resolveTimezone(tz string) (*time.Location, error) {
	tz = strings.TrimSpace(tz)
	if tz == "" {
		return nil, validationErr("timezone", "must not be empty")
	}
	if tz == "Local" {
		return nil, validationErr("timezone", `"Local" is not a meaningful cross-machine timezone; use an IANA name`)
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return nil, validationErr("timezone", "not a valid IANA timezone name: %s", tz)
	}
	return loc, nil
}

const dateLayout = "2006-01-02"

func resolveJournalDate(journalDate string, loc *time.Location, now time.Time) (string, error) {
	journalDate = strings.TrimSpace(journalDate)
	if journalDate == "" {
		return now.In(loc).Format(dateLayout), nil
	}

	parsed, err := time.Parse(dateLayout, journalDate)
	if err != nil || parsed.Format(dateLayout) != journalDate {
		return "", validationErr("journal_date", "must be a valid calendar date in YYYY-MM-DD format: %s", journalDate)
	}
	return journalDate, nil
}

func normalizeTags(tags []string, maxTags int) ([]string, error) {
	seen := make(map[string]struct{}, len(tags))
	normalized := make([]string, 0, len(tags))
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		normalized = append(normalized, t)
	}
	if len(normalized) > maxTags {
		return nil, validationErr("tags", "must not contain more than %d tags", maxTags)
	}
	return normalized, nil
}
