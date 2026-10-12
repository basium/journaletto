package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func testDeps(root string) EntryDeps {
	return EntryDeps{
		JournalRoot:   root,
		DevUserID:     "dev-user-001",
		MaxTags:       100,
		MaxEntryBytes: 100 * 1024,
	}
}

func doRequest(t *testing.T, deps EntryDeps, body any) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshaling request body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/entries", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	NewRouter(deps).ServeHTTP(rec, req)
	return rec
}

// Requirement: JE-001, JE-002, JE-003, JE-004, JE-012
func TestCreateEntry_HappyPath(t *testing.T) {
	root := t.TempDir()
	deps := testDeps(root)

	rec := doRequest(t, deps, createEntryRequest{
		Title:       "Completed payment integration",
		Description: "Fixed the retry logic.",
		Tags:        []string{"work", "payments"},
		Timezone:    "Europe/Malta",
	})

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var resp createEntryResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if resp.ID == "" {
		t.Errorf("expected non-empty id")
	}
	if resp.Status != "pending" {
		t.Errorf("status = %q, want pending", resp.Status)
	}
	if resp.Path == "" {
		t.Errorf("expected non-empty path")
	}
	if filepath.IsAbs(resp.Path) {
		t.Errorf("path must be relative, got %q", resp.Path)
	}

	fullPath := filepath.Join(root, resp.Path)
	if _, err := os.Stat(fullPath); err != nil {
		t.Errorf("expected file at %s: %v", fullPath, err)
	}
}

// Requirement: JE-001, JE-012 (fail fast, no side effects)
func TestCreateEntry_ValidationFailure_NoFileCreated(t *testing.T) {
	cases := []struct {
		name string
		req  createEntryRequest
	}{
		{
			name: "missing title",
			req: createEntryRequest{
				Description: "x", Timezone: "UTC",
			},
		},
		{
			name: "missing description",
			req: createEntryRequest{
				Title: "x", Timezone: "UTC",
			},
		},
		{
			name: "bad timezone",
			req: createEntryRequest{
				Title: "x", Description: "y", Timezone: "Not/Real",
			},
		},
		{
			name: "too many tags",
			req: createEntryRequest{
				Title: "x", Description: "y", Timezone: "UTC",
				Tags: manyTags(101),
			},
		},
		{
			name: "embedded image",
			req: createEntryRequest{
				Title: "x", Description: "see ![a](b.png)", Timezone: "UTC",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			deps := testDeps(root)

			rec := doRequest(t, deps, tc.req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusBadRequest, rec.Body.String())
			}

			var count int
			filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
				if err == nil && !info.IsDir() {
					count++
				}
				return nil
			})
			if count != 0 {
				t.Errorf("expected no files created on validation failure, found %d", count)
			}
		})
	}
}

// Requirement: JE-012 (oversized entry, 100 KB limit)
func TestCreateEntry_OversizedEntry_NoFileCreated(t *testing.T) {
	root := t.TempDir()
	deps := testDeps(root)
	deps.MaxEntryBytes = 200 // tiny limit to trigger easily

	rec := doRequest(t, deps, createEntryRequest{
		Title: "x", Description: "this description is definitely going to exceed a 200 byte cap once front matter is included too",
		Timezone: "UTC",
	})

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}

	var count int
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			count++
		}
		return nil
	})
	if count != 0 {
		t.Errorf("expected no files created for oversized entry, found %d", count)
	}
}

func manyTags(n int) []string {
	tags := make([]string, n)
	for i := range tags {
		tags[i] = "tag" + string(rune('a'+i%26)) + string(rune('0'+i/26))
	}
	return tags
}
