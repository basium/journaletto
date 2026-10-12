package journal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func testEntry() Entry {
	return Entry{
		ID:          "019c45a6-3e22-7d31-a794-1d12dde6c123",
		Title:       "Completed payment integration",
		JournalDate: "2026-08-12",
		Description: "Fixed the retry logic.",
		Tags:        []string{"work", "payments"},
		Timezone:    "Europe/Malta",
		CreatedAt:   time.Date(2026, 8, 12, 13, 42, 18, 0, time.UTC),
	}
}

// Requirement: JE-002, JE-003, JE-004, JE-012
func TestSave_HappyPath(t *testing.T) {
	root := t.TempDir()
	entry := testEntry()

	relPath, err := Save(root, "dev-user-001", entry, 100*1024)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantRel := filepath.Join("dev-user-001", "2026-08-12", "20260812T134218Z_019c45a6-3e22-7d31-a794-1d12dde6c123.md")
	if relPath != wantRel {
		t.Errorf("relPath = %q, want %q", relPath, wantRel)
	}

	fullPath := filepath.Join(root, relPath)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		t.Fatalf("expected file at %s: %v", fullPath, err)
	}
	content := string(data)

	if !strings.HasPrefix(content, "---\n") {
		t.Fatalf("file does not start with front matter delimiter")
	}
	parts := strings.SplitN(content, "---\n", 3)
	if len(parts) != 3 {
		t.Fatalf("expected two '---' sections, got %d", len(parts))
	}

	var fm FrontMatter
	if err := yaml.Unmarshal([]byte(parts[1]), &fm); err != nil {
		t.Fatalf("front matter did not parse as YAML: %v", err)
	}
	if fm.SchemaVersion != 1 {
		t.Errorf("schema_version = %d, want 1", fm.SchemaVersion)
	}
	if fm.ID != entry.ID {
		t.Errorf("id = %q, want %q", fm.ID, entry.ID)
	}
	if fm.Status != StatusActive {
		t.Errorf("status = %q, want %q", fm.Status, StatusActive)
	}
	if fm.CreatedAt != fm.UpdatedAt {
		t.Errorf("created_at (%q) != updated_at (%q) on fresh entry", fm.CreatedAt, fm.UpdatedAt)
	}
	if fm.Timezone != entry.Timezone {
		t.Errorf("timezone = %q, want %q", fm.Timezone, entry.Timezone)
	}
	if !equalStrings(fm.Tags, entry.Tags) {
		t.Errorf("tags = %v, want %v", fm.Tags, entry.Tags)
	}

	if !strings.Contains(parts[2], "# Completed payment integration") {
		t.Errorf("body missing title heading")
	}
	if !strings.Contains(parts[2], "Fixed the retry logic.") {
		t.Errorf("body missing description")
	}

	// No stray temp files should remain in the daily directory.
	dailyDir := filepath.Dir(fullPath)
	entries, err := os.ReadDir(dailyDir)
	if err != nil {
		t.Fatalf("reading daily dir: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected exactly 1 file in daily dir, found %d", len(entries))
	}
}

// Requirement: JE-012 (100 KB limit, Performance section)
func TestSave_RejectsOversizedEntry(t *testing.T) {
	root := t.TempDir()
	entry := testEntry()
	entry.Description = strings.Repeat("a", 200*1024)

	_, err := Save(root, "dev-user-001", entry, 100*1024)
	if err == nil {
		t.Fatalf("expected an error for an oversized entry")
	}

	assertNoFilesWritten(t, root)
}

// Requirement: JE-012 (100 KB boundary)
func TestSave_AllowsExactlyMaxSize(t *testing.T) {
	root := t.TempDir()
	entry := testEntry()

	// Render once to measure the fixed overhead, then pad the description
	// so the final content is exactly maxEntryBytes.
	baseline, err := Render(entry.ToFrontMatter(), entry.Title, entry.Description)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	const maxBytes = 100 * 1024
	pad := maxBytes - len(baseline)
	if pad < 0 {
		t.Fatalf("baseline already exceeds maxBytes; adjust test fixture")
	}
	entry.Description = entry.Description + strings.Repeat("a", pad)

	content, err := Render(entry.ToFrontMatter(), entry.Title, entry.Description)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(content) != maxBytes {
		t.Fatalf("test setup error: content is %d bytes, want exactly %d", len(content), maxBytes)
	}

	if _, err := Save(root, "dev-user-001", entry, maxBytes); err != nil {
		t.Fatalf("expected exactly-max-size entry to be accepted, got error: %v", err)
	}
}

func assertNoFilesWritten(t *testing.T, root string) {
	t.Helper()
	var count int
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			count++
			t.Logf("unexpected file left behind: %s", path)
		}
		return nil
	})
	if count != 0 {
		t.Errorf("expected no files written, found %d", count)
	}
}
