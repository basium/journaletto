package journal

import (
	"strings"
	"testing"
	"time"
)

// Requirement: JE-002, JE-004
func TestRender_FormatAndFields(t *testing.T) {
	e := Entry{
		ID:          "019c45a6-3e22-7d31-a794-1d12dde6c123",
		Title:       "Completed payment integration",
		JournalDate: "2026-08-12",
		Description: "Fixed the retry logic.\n\n## Notes\nAll good now.",
		Tags:        []string{"work", "payments"},
		Timezone:    "Europe/Malta",
		CreatedAt:   time.Date(2026, 8, 12, 13, 42, 18, 0, time.UTC),
	}

	content, err := Render(e.ToFrontMatter(), e.Title, e.Description)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.HasPrefix(content, "---\n") {
		t.Fatalf("content does not start with front matter delimiter:\n%s", content)
	}

	parts := strings.SplitN(content, "---\n", 3)
	if len(parts) != 3 {
		t.Fatalf("expected exactly two '---' delimited sections, got %d parts:\n%s", len(parts), content)
	}
	yamlBlock := parts[1]
	body := parts[2]

	for _, want := range []string{
		"schema_version: 1",
		`id: 019c45a6-3e22-7d31-a794-1d12dde6c123`,
		"title: Completed payment integration",
		"journal_date: \"2026-08-12\"",
		"created_at: \"2026-08-12T13:42:18Z\"",
		"updated_at: \"2026-08-12T13:42:18Z\"",
		"timezone: Europe/Malta",
		"status: active",
	} {
		if !strings.Contains(yamlBlock, want) {
			t.Errorf("front matter missing %q; got:\n%s", want, yamlBlock)
		}
	}

	if !strings.HasPrefix(body, "\n# Completed payment integration\n\n") {
		t.Errorf("body does not start with expected heading block, got:\n%q", body)
	}
	if !strings.Contains(body, "Fixed the retry logic.\n\n## Notes\nAll good now.") {
		t.Errorf("body does not preserve description verbatim, got:\n%q", body)
	}
}

// Requirement: JE-002
func TestRender_DescriptionPreservedByteForByte(t *testing.T) {
	desc := "Line one.\n\n- bullet a\n- bullet b\n\n> a quote with **bold** and `code`"
	e := Entry{
		ID:          "019c45a6-3e22-7d31-a794-1d12dde6c123",
		Title:       "T",
		JournalDate: "2026-08-12",
		Description: desc,
		Timezone:    "UTC",
		CreatedAt:   time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC),
	}
	content, err := Render(e.ToFrontMatter(), e.Title, e.Description)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasSuffix(content, desc+"\n") {
		t.Errorf("description not preserved verbatim at end of file; got:\n%q", content)
	}
}
