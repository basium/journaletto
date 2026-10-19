package worker_test

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"journaletto/internal/entries"
	"journaletto/internal/provider"
	"journaletto/internal/worker"
)

func saveBody(t *testing.T, svc *entries.Service, title, body string) string {
	t.Helper()
	e, err := svc.Save(context.Background(), entries.Input{Title: title, Description: body})
	if err != nil {
		t.Fatal(err)
	}
	return e.ID
}

func waitDone(t *testing.T, db *sql.DB, id string, jobs int) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(2 * time.Millisecond) {
		if count(t, db, `SELECT COUNT(*) FROM jobs WHERE state='done'`) >= jobs {
			break
		}
	}
	waitStatus(t, db, id, "ready")
}

const sectionedBody = "Opening words before any heading.\n\n# Plan\nShip the zebra feature.\n\n## Risks\nThe zebra might bolt.\n\n# Notes\n" +
	"Ada Lovelace reviewed it."

func TestSectionChunksFollowHeadingsWithParentContextAndExactTextIndex(t *testing.T) {
	db, svc := setup(t)
	long := strings.Repeat("word ", 5000) // far beyond any fixed token window
	id := saveBody(t, svc, "Weekly review", sectionedBody+"\n\n# Long\n"+long)
	start(t, &worker.Worker{DB: db, Steps: worker.DefaultSteps(db, provider.Fake{}), MaxRetries: 0})
	waitStatus(t, db, id, "ready")

	rows, err := db.Query(`SELECT heading_path, context, text FROM chunks WHERE entry_id=? AND granularity='section' ORDER BY ord`, id)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var paths []string
	for rows.Next() {
		var path, ctxt, text string
		if err := rows.Scan(&path, &ctxt, &text); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(ctxt, "Weekly review") || !strings.Contains(ctxt, path) {
			t.Errorf("context %q lacks title/path %q", ctxt, path)
		}
		if path == "Long" && len(text) < len(long)-1 {
			t.Errorf("long section was split: %d bytes", len(text))
		}
		paths = append(paths, path)
	}
	if got := strings.Join(paths, "|"); got != "|Plan|Plan > Risks|Notes|Long" {
		t.Errorf("heading paths = %q", got)
	}
	if n := count(t, db, `SELECT COUNT(*) FROM chunks_fts WHERE chunks_fts MATCH '"zebra might bolt"'`); n != 1 {
		t.Errorf("exact phrase hits = %d, want 1", n)
	}
}

func TestFactChunksFromProviderWithProvenanceAndReplacedOnReprocess(t *testing.T) {
	db, svc := setup(t)
	id := saveBody(t, svc, "Weekly review", "# Notes\nAda Lovelace reviewed it.")
	var calls int
	llm := stubLLM{complete: func(p string) string {
		if !strings.HasPrefix(p, provider.FactPromptPrefix) {
			return "\x00"
		}
		calls++
		if calls == 1 {
			return `["Ada Lovelace reviewed the weekly review.","Weekly review notes mention Ada Lovelace."]`
		}
		return "```json\n" + `[{"fact":"Ada Lovelace approved the weekly review.","section":"notes"}]` + "\n```"
	}}
	start(t, &worker.Worker{DB: db, Steps: worker.DefaultSteps(db, llm), MaxRetries: 0})
	waitStatus(t, db, id, "ready")

	if n := count(t, db, `SELECT COUNT(*) FROM chunks WHERE entry_id=? AND granularity='fact'`, id); n != 2 {
		t.Fatalf("facts = %d, want 2", n)
	}
	var prov, model, pv, ctxt string
	if err := db.QueryRow(`SELECT provider,model,prompt_version,context FROM chunks WHERE text LIKE 'Ada Lovelace reviewed the%' AND granularity='fact'`).
		Scan(&prov, &model, &pv, &ctxt); err != nil {
		t.Fatal(err)
	}
	if prov != "fake" || model == "" || pv != worker.FactsPromptVersion || !strings.Contains(ctxt, "Weekly review") {
		t.Errorf("fact provenance: %q %q %q ctx=%q", prov, model, pv, ctxt)
	}

	if err := worker.Resync(context.Background(), db, "u", id); err != nil {
		t.Fatal(err)
	}
	waitDone(t, db, id, 2)
	if n := count(t, db, `SELECT COUNT(*) FROM chunks WHERE entry_id=? AND granularity='fact'`, id); n != 1 {
		t.Errorf("facts after reprocess = %d, want 1 (replaced, not duplicated)", n)
	}
	var path string
	if err := db.QueryRow(`SELECT heading_path FROM chunks WHERE granularity='fact'`).Scan(&path); err != nil || path != "Notes" {
		t.Errorf("fact heading path = %q, %v", path, err)
	}
	if n := count(t, db, `SELECT COUNT(*) FROM chunks WHERE entry_id=? AND granularity='section'`, id); n != 1 {
		t.Errorf("sections after reprocess = %d, want 1", n)
	}
	if n := count(t, db, `SELECT COUNT(*) FROM chunks_fts`); n != 2 {
		t.Errorf("fts rows = %d, want 2", n)
	}
}

func TestChunksLinkOnlyEntitiesTheyMention(t *testing.T) {
	db, svc := setup(t)
	id := saveBody(t, svc, "Weekly review", "# Notes\nFake Project is going well.\n\n# Other\nNothing relevant.")
	start(t, &worker.Worker{DB: db, Steps: worker.DefaultSteps(db, provider.Fake{}), MaxRetries: 0})
	waitStatus(t, db, id, "ready")
	var path string
	if err := db.QueryRow(`SELECT c.heading_path FROM chunk_entities ce JOIN chunks c ON c.id=ce.chunk_id JOIN entities e ON e.id=ce.entity_id
		WHERE c.granularity='section' AND e.value='Fake Project'`).Scan(&path); err != nil || path != "Notes" {
		t.Errorf("linked section = %q, %v (want only Notes)", path, err)
	}
}
