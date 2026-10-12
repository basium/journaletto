package journal

import (
	"testing"
	"time"
)

func validInput() Input {
	return Input{
		Title:       "Completed payment integration",
		Description: "Fixed the retry logic.",
		Tags:        []string{"work", "payments"},
		Timezone:    "Europe/Malta",
	}
}

// Requirement: JE-001
func TestValidateAndNormalize_HappyPath(t *testing.T) {
	now := time.Date(2026, 8, 12, 13, 42, 18, 0, time.UTC)
	norm, err := ValidateAndNormalize(validInput(), 100, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if norm.Title != "Completed payment integration" {
		t.Errorf("title = %q", norm.Title)
	}
	if norm.Description != "Fixed the retry logic." {
		t.Errorf("description = %q", norm.Description)
	}
	if got, want := norm.Tags, []string{"work", "payments"}; !equalStrings(got, want) {
		t.Errorf("tags = %v, want %v", got, want)
	}
	// Europe/Malta is UTC+2 in August (CEST), so 13:42 UTC is still the
	// same calendar day locally; this just confirms the default resolves
	// via the client timezone rather than failing outright.
	if norm.JournalDate == "" {
		t.Errorf("journal_date should have defaulted, got empty string")
	}
}

// Requirement: JE-001
func TestValidateAndNormalize_MissingTitle(t *testing.T) {
	in := validInput()
	in.Title = "   "
	assertFieldError(t, in, "title")
}

// Requirement: JE-001
func TestValidateAndNormalize_MissingDescription(t *testing.T) {
	in := validInput()
	in.Description = ""
	assertFieldError(t, in, "description")
}

// Requirement: JE-005
func TestValidateAndNormalize_RejectsEmbeddedImage(t *testing.T) {
	in := validInput()
	in.Description = "Look at this ![alt text](https://example.com/photo.png) from today."
	assertFieldError(t, in, "description")
}

// Requirement: JE-004
func TestValidateAndNormalize_RequiresTimezone(t *testing.T) {
	in := validInput()
	in.Timezone = ""
	assertFieldError(t, in, "timezone")
}

// Requirement: JE-004
func TestValidateAndNormalize_RejectsLocalTimezone(t *testing.T) {
	in := validInput()
	in.Timezone = "Local"
	assertFieldError(t, in, "timezone")
}

// Requirement: JE-004
func TestValidateAndNormalize_RejectsGarbageTimezone(t *testing.T) {
	in := validInput()
	in.Timezone = "Not/A_Real_Zone"
	assertFieldError(t, in, "timezone")
}

// Requirement: JE-001
func TestValidateAndNormalize_JournalDateDefaultsUsingClientTimezone(t *testing.T) {
	// 2026-08-13 00:30 UTC is still 2026-08-12 in Pacific/Honolulu (UTC-10).
	now := time.Date(2026, 8, 13, 0, 30, 0, 0, time.UTC)
	in := validInput()
	in.Timezone = "Pacific/Honolulu"
	norm, err := ValidateAndNormalize(in, 100, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if norm.JournalDate != "2026-08-12" {
		t.Errorf("journal_date = %q, want 2026-08-12 (local-today, not UTC-today)", norm.JournalDate)
	}
}

// Requirement: JE-001
func TestValidateAndNormalize_ExplicitJournalDate(t *testing.T) {
	now := time.Date(2026, 8, 13, 0, 30, 0, 0, time.UTC)
	in := validInput()
	in.JournalDate = "2026-07-01"
	norm, err := ValidateAndNormalize(in, 100, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if norm.JournalDate != "2026-07-01" {
		t.Errorf("journal_date = %q, want 2026-07-01", norm.JournalDate)
	}
}

// Requirement: JE-001
func TestValidateAndNormalize_RejectsInvalidCalendarDate(t *testing.T) {
	in := validInput()
	in.JournalDate = "2026-02-30" // no such day
	assertFieldError(t, in, "journal_date")
}

// Requirement: JE-001
func TestValidateAndNormalize_RejectsMalformedJournalDate(t *testing.T) {
	in := validInput()
	in.JournalDate = "08/12/2026"
	assertFieldError(t, in, "journal_date")
}

// Requirement: JE-001 (tag trimming/dedup)
func TestValidateAndNormalize_TagsTrimmedDedupedEmptyDropped(t *testing.T) {
	now := time.Date(2026, 8, 12, 13, 42, 18, 0, time.UTC)
	in := validInput()
	in.Tags = []string{" work ", "work", "", "  ", "payments", "Payments"}
	norm, err := ValidateAndNormalize(in, 100, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"work", "payments", "Payments"}
	if !equalStrings(norm.Tags, want) {
		t.Errorf("tags = %v, want %v", norm.Tags, want)
	}
}

// Requirement: JE-001 (100-tag cap, Performance section)
func TestValidateAndNormalize_RejectsTooManyTags(t *testing.T) {
	in := validInput()
	tags := make([]string, 0, 101)
	for i := 0; i < 101; i++ {
		tags = append(tags, "tag"+string(rune('a'+i%26))+string(rune('0'+i/26)))
	}
	in.Tags = tags
	assertFieldError(t, in, "tags")
}

// Requirement: JE-001 (100-tag cap boundary)
func TestValidateAndNormalize_AllowsExactlyMaxTags(t *testing.T) {
	now := time.Date(2026, 8, 12, 13, 42, 18, 0, time.UTC)
	in := validInput()
	tags := make([]string, 0, 100)
	for i := 0; i < 100; i++ {
		tags = append(tags, "tag"+string(rune('a'+i%26))+string(rune('0'+i/26)))
	}
	in.Tags = tags
	norm, err := ValidateAndNormalize(in, 100, now)
	if err != nil {
		t.Fatalf("unexpected error at exactly max tags: %v", err)
	}
	if len(norm.Tags) != 100 {
		t.Errorf("got %d tags, want 100", len(norm.Tags))
	}
}

func assertFieldError(t *testing.T, in Input, wantField string) {
	t.Helper()
	now := time.Date(2026, 8, 12, 13, 42, 18, 0, time.UTC)
	_, err := ValidateAndNormalize(in, 100, now)
	if err == nil {
		t.Fatalf("expected a validation error, got nil")
	}
	var ve *ValidationError
	if !asValidationError(err, &ve) {
		t.Fatalf("expected *ValidationError, got %T: %v", err, err)
	}
	if ve.Field != wantField {
		t.Errorf("error field = %q, want %q (message: %s)", ve.Field, wantField, ve.Message)
	}
}

func asValidationError(err error, target **ValidationError) bool {
	ve, ok := err.(*ValidationError)
	if ok {
		*target = ve
	}
	return ok
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
