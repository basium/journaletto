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
	return runGroundedBody(t, groundedBody, llm)
}

func runGroundedBody(t *testing.T, body string, llm stubLLM) (db *sql.DB, id string, logs string) {
	t.Helper()
	db, svc := setup(t)
	id = saveBody(t, svc, "Standup notes", body)
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

const tokenBody = "The Canada office shipped C++ code. Plan C was dropped.\n" +
	"José Müller reviewed **Sofia** work on node.js, ticket JE-42, issue #118, on 2026-10-19."

// Entities use whole-token matching: no substring hits, symbols stay significant, accents and
// markdown do not matter. A stub provider returns grounded and ungrounded entities together and
// only the grounded ones may be stored.
func TestEntityMatchingIsWholeTokenAndSymbolAware(t *testing.T) {
	cases := []struct {
		typ, value string
		kept       bool
	}{
		{"People", "Ada", false},        // only inside "Canada"
		{"Companies", "Canada", true},   // whole word
		{"Concepts", "C++", true},       // present as C++
		{"Concepts", "C#", false},       // only "c" and "c++" present
		{"Concepts", ".NET", false},     // absent
		{"Concepts", "node.js", true},   // dotted token
		{"Tickets", "JE-42", true},      // hyphenated id
		{"Tickets", "JE-4", false},      // prefix of a token
		{"Tickets", "#118", true},       // hash id
		{"People", "José Müller", true}, // NFC query, NFD text
		{"People", "**Sofia**", true},   // markdown-wrapped value
		{"Dates", "2026-10-19", true},
	}
	llm := stubLLM{complete: func(p string) string {
		if !strings.HasPrefix(p, "Extract entities") {
			return "\x00"
		}
		var parts []string
		for _, c := range cases {
			parts = append(parts, `{"type":"`+c.typ+`","value":"`+c.value+`","relations":[]}`)
		}
		return "[" + strings.Join(parts, ",") + "]"
	}}
	db, id, _ := runGroundedBody(t, tokenBody, llm)
	kept := 0
	for _, c := range cases {
		n := count(t, db, `SELECT COUNT(*) FROM entities WHERE entry_id=? AND value=?`, id, c.value)
		if c.kept {
			kept++
		}
		if (n == 1) != c.kept || n > 1 {
			t.Errorf("entity %q: stored %d times, want kept=%v", c.value, n, c.kept)
		}
	}
	if n := count(t, db, `SELECT COUNT(*) FROM entities WHERE entry_id=?`, id); n != kept {
		t.Errorf("entities = %d, want %d", n, kept)
	}
}

func TestOnlyGroundedEntityIsStoredEndToEnd(t *testing.T) {
	llm := stubLLM{complete: func(p string) string {
		if !strings.HasPrefix(p, "Extract entities") {
			return "\x00"
		}
		return `[{"type":"People","value":"Zaphod Beeblebrox","relations":[]},
			{"type":"People","value":"Ada Lovelace","relations":[]}]`
	}}
	db, id, _ := runGrounded(t, llm)
	if n := count(t, db, `SELECT COUNT(*) FROM entities WHERE entry_id=?`, id); n != 1 {
		t.Fatalf("entities = %d, want exactly the grounded one", n)
	}
	if n := count(t, db, `SELECT COUNT(*) FROM entities WHERE entry_id=? AND value='Ada Lovelace'`, id); n != 1 {
		t.Error("grounded entity not stored")
	}
}

func TestPronounsAreNotStoredAsPeopleAndLoggedByReasonOnly(t *testing.T) {
	var prompt string
	llm := stubLLM{complete: func(p string) string {
		if !strings.HasPrefix(p, "Extract entities") {
			return "\x00"
		}
		prompt = p
		return `[{"type":"People","value":"I","relations":[]},
			{"type":"People","value":"We","relations":[]},
			{"type":"People","value":"Ada","relations":[{"to":"I","to_type":"People","kind":"told"}]},
			{"type":"Tickets","value":"JRN-12","relations":[]}]`
	}}
	db, id, logs := runGroundedBody(t, "I told Ada that we closed ticket JRN-12.", llm)
	if n := count(t, db, `SELECT COUNT(*) FROM entities WHERE entry_id=?`, id); n != 2 {
		t.Errorf("entities = %d, want 2 (Ada, JRN-12)", n)
	}
	if n := count(t, db, `SELECT COUNT(*) FROM entities WHERE type='People' AND norm_value IN ('i','we')`); n != 0 {
		t.Error("pronoun stored as a person")
	}
	if n := count(t, db, `SELECT COUNT(*) FROM entity_relations`); n != 0 {
		t.Errorf("relation to a dropped pronoun kept: %d", n)
	}
	if !strings.Contains(logs, "reason=pronoun") || strings.Contains(logs, `"I"`) {
		t.Errorf("drop must be logged by reason without content: %q", logs)
	}
	if !strings.Contains(prompt, "pronouns") {
		t.Error("prompt must exclude pronouns")
	}
}

const factBody ="# Release\nAda Lovelace reviewed ticket JE-42 on 2026-10-19 and measured 3.5 hours of work on the parser."

func TestFactSupportKeepsParaphrasesAndKeepsNumbersWhole(t *testing.T) {
	facts := []struct {
		text string
		kept bool
	}{
		{"Ada Lovelace reviewed the parser work, which took 3.5 hours.", true}, // paraphrase, short word 'took' unsupported
		{"On 2026-10-19 Ada reviewed JE-42.", true},                            // date and ticket id whole
		{"The parser work took 4.5 hours.", false},                             // decimal differs
		{"Ada reviewed the parser on 2026-10-20.", false},                      // date differs
		{"Ada Lovelace reviewed ticket JE-4.", false},                          // ticket id differs
		{"Ada Lovelace reviewed ticket JE-42 together with Zaphod.", false},    // invented entity-like word
		{"The quarterly budget was approved by the board.", false},             // unrelated
	}
	llm := stubLLM{complete: func(p string) string {
		if !strings.HasPrefix(p, "Extract facts") {
			return "\x00"
		}
		var parts []string
		for _, f := range facts {
			parts = append(parts, `"`+f.text+`"`)
		}
		return "[" + strings.Join(parts, ",") + "]"
	}}
	db, id, _ := runGroundedBody(t, factBody, llm)
	kept := 0
	for _, f := range facts {
		n := count(t, db, `SELECT COUNT(*) FROM chunks WHERE entry_id=? AND granularity='fact' AND text=?`, id, f.text)
		if f.kept {
			kept++
		}
		if (n == 1) != f.kept {
			t.Errorf("fact %q: stored %d times, want kept=%v", f.text, n, f.kept)
		}
	}
	if n := count(t, db, `SELECT COUNT(*) FROM chunks WHERE entry_id=? AND granularity='fact'`, id); n != kept {
		t.Errorf("facts = %d, want %d", n, kept)
	}
}
