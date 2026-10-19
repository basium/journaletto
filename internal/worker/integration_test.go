package worker_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"journaletto/internal/entries"
	"journaletto/internal/provider"
	"journaletto/internal/worker"
)

// TestMilestone1EndToEndEntryToReady is the milestone gate: one entry saved through the service
// is processed by the worker (fake provider) and every artefact exists in the database or on
// disk, produced by the pipeline and not by fixtures.
func TestMilestone1EndToEndEntryToReady(t *testing.T) {
	db, svc := setup(t)
	ctx := context.Background()
	e, err := svc.Save(ctx, entries.Input{
		Title: "Release day", Tags: []string{"Work"}, Timezone: "Europe/Rome",
		Description: "# Plan\nShip the zebra feature.\n\n# Done\nClosed ticket ABC-1 today.\nMerged PR 42 after review.",
	})
	if err != nil {
		t.Fatal(err)
	}
	id := e.ID
	start(t, &worker.Worker{DB: db, Steps: worker.DefaultSteps(db, provider.Fake{}), MaxRetries: 0})
	waitStatus(t, db, id, "ready")

	// Raw file with front matter and untouched description; no generated data in it.
	raw, err := os.ReadFile(e.Path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"---\nschema_version: 1\n", "id: " + id, "title: Release day", "timezone: Europe/Rome",
		"- work", "status: active", "\n# Release day\n\n# Plan\nShip the zebra feature."} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("raw file lacks %q", want)
		}
	}
	if strings.Contains(string(raw), "fake") {
		t.Error("generated data leaked into raw file")
	}

	// Canonical record and exact-text index.
	var title, body, tz string
	if err := db.QueryRow(`SELECT title,body,timezone FROM entries WHERE id=?`, id).Scan(&title, &body, &tz); err != nil {
		t.Fatal(err)
	}
	if title != "Release day" || !strings.Contains(body, "zebra") || tz != "Europe/Rome" {
		t.Errorf("canonical row: %q %q %q", title, body, tz)
	}
	if n := count(t, db, `SELECT COUNT(*) FROM entries_fts WHERE entries_fts MATCH 'zebra' AND entry_id=?`, id); n != 1 {
		t.Errorf("entry FTS hits = %d", n)
	}
	if n := count(t, db, `SELECT COUNT(*) FROM chunks_fts WHERE chunks_fts MATCH 'zebra' AND entry_id=?`, id); n < 1 {
		t.Errorf("chunk FTS hits = %d", n)
	}

	// Chunks (both granularities), entities, embeddings, occurrences.
	for _, g := range []string{"section", "fact"} {
		if n := count(t, db, `SELECT COUNT(*) FROM chunks WHERE entry_id=? AND granularity=?`, id, g); n < 1 {
			t.Errorf("no %s chunks", g)
		}
	}
	if n := count(t, db, `SELECT COUNT(*) FROM entities WHERE entry_id=?`, id); n < 1 {
		t.Error("no entities")
	}
	chunks := count(t, db, `SELECT COUNT(*) FROM chunks WHERE entry_id=?`, id)
	if n := count(t, db, `SELECT COUNT(*) FROM chunk_embeddings WHERE entry_id=? AND dims=?`, id, provider.FakeDims); n != chunks {
		t.Errorf("embeddings = %d, chunks = %d", n, chunks)
	}
	if n := count(t, db, `SELECT COUNT(*) FROM occurrences WHERE entry_id=? AND rule_id='closed-ticket'`, id); n != 1 {
		t.Errorf("closed-ticket occurrences = %d", n)
	}
	if n := count(t, db, `SELECT COUNT(*) FROM occurrences WHERE entry_id=? AND rule_id='merged-pr'`, id); n != 1 {
		t.Errorf("merged-pr occurrences = %d", n)
	}

	// Provenance on every generated table.
	// (Section chunks are produced deterministically, so they carry their own non-empty label.)
	for _, table := range []string{"entry_artifacts", "entities", "chunks", "chunk_embeddings", "occurrences"} {
		q := `SELECT COUNT(*) FROM ` + table + ` WHERE entry_id=? AND (provider='' OR model='' OR generated_at='')`
		if n := count(t, db, q, id); n != 0 {
			t.Errorf("%s: %d rows without provenance", table, n)
		}
		if table != "chunks" {
			if n := count(t, db, `SELECT COUNT(*) FROM `+table+` WHERE entry_id=? AND provider<>'fake'`, id); n != 0 {
				t.Errorf("%s: %d rows not attributed to the fake provider", table, n)
			}
		}
		if n := count(t, db, `SELECT COUNT(*) FROM `+table+` WHERE entry_id=?`, id); n == 0 {
			t.Errorf("%s: empty", table)
		}
	}
	for _, table := range []string{"entities", "chunks", "occurrences"} {
		if n := count(t, db, `SELECT COUNT(*) FROM `+table+` WHERE entry_id=? AND prompt_version=''`, id); n != 0 {
			t.Errorf("%s: %d rows without prompt version", table, n)
		}
	}
}
