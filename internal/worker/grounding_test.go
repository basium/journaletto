package worker_test

import (
	"bytes"
	"database/sql"
	"log"
	"os"
	"strings"
	"testing"

	"journaletto/internal/worker"
)

const groundedBody = "# Standup\nAda **Lovelace** closed ticket JRN-12 after the fix.\n\nMerged PR 45 into `main` on Friday."

func runGrounded(t *testing.T, llm stubLLM) (db *sql.DB, id string, logs string) {
	t.Helper()
	db, svc := setup(t)
	id = saveBody(t, svc, "Standup notes", groundedBody)
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	start(t, &worker.Worker{DB: db, Steps: worker.DefaultSteps(db, llm), MaxRetries: 0})
	waitStatus(t, db, id, "ready")
	return db, id, buf.String()
}

func TestEntitiesNotInEntryAreDroppedAndLoggedByReasonOnly(t *testing.T) {
	llm := stubLLM{complete: func(p string) string {
		if !strings.HasPrefix(p, "Extract entities") {
			return "\x00"
		}
		return `[{"type":"People","value":"ada  lovelace","relations":[]},
			{"type":"Tickets","value":"JRN-12","relations":[]},
			{"type":"People","value":"Zaphod Beeblebrox","relations":[]}]`
	}}
	db, id, logs := runGrounded(t, llm)
	if n := count(t, db, `SELECT COUNT(*) FROM entities WHERE entry_id=?`, id); n != 2 {
		t.Errorf("entities = %d, want 2 (Zaphod dropped)", n)
	}
	if n := count(t, db, `SELECT COUNT(*) FROM entities WHERE value LIKE 'Zaphod%'`); n != 0 {
		t.Error("ungrounded entity kept")
	}
	if !strings.Contains(logs, "entry "+id) || !strings.Contains(logs, "entity type=People") || strings.Contains(logs, "Zaphod") {
		t.Errorf("drop log must name entry/type/reason and never content: %q", logs)
	}
}

func TestFactChunksNeedSupportFromEntryText(t *testing.T) {
	llm := stubLLM{complete: func(p string) string {
		if !strings.HasPrefix(p, "Extract facts") {
			return "\x00"
		}
		return `["Ada Lovelace closed ticket JRN-12 after the fix.",
			"Ada Lovelace closed ticket JRN-99 after the fix.",
			"The quarterly budget was approved by the board.",
			"Merged PR 45 on Friday."]`
	}}
	db, id, logs := runGrounded(t, llm)
	if n := count(t, db, `SELECT COUNT(*) FROM chunks WHERE entry_id=? AND granularity='fact'`, id); n != 2 {
		t.Errorf("facts = %d, want 2 (wrong number and invented fact dropped)", n)
	}
	if n := count(t, db, `SELECT COUNT(*) FROM chunks WHERE entry_id=? AND granularity='section'`, id); n != 1 {
		t.Errorf("sections = %d, section chunks are exempt from grounding", n)
	}
	if !strings.Contains(logs, "fact chunk") || strings.Contains(logs, "budget") || strings.Contains(logs, "JRN-99") {
		t.Errorf("drop log: %q", logs)
	}
}

func TestRuleExcerptsMustAppearInEntryIgnoringMarkdownAndCase(t *testing.T) {
	llm := stubLLM{complete: func(p string) string {
		if !strings.HasPrefix(p, "Detect occurrences") || !strings.Contains(p, "rule closed-ticket.") {
			return "\x00"
		}
		return `[{"event":"a","excerpt":"ada lovelace closed TICKET jrn-12","confidence":0.9},
			{"event":"b","excerpt":"Closed ticket JRN-77 yesterday","confidence":0.9}]`
	}}
	db, id, logs := runGrounded(t, llm)
	if n := count(t, db, `SELECT COUNT(*) FROM occurrences WHERE entry_id=? AND rule_id='closed-ticket'`, id); n != 1 {
		t.Errorf("occurrences = %d, want 1", n)
	}
	if !strings.Contains(logs, "rule excerpt") || strings.Contains(logs, "JRN-77") {
		t.Errorf("drop log: %q", logs)
	}
}
