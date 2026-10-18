package worker_test

import (
	"context"
	"testing"

	"journaletto/internal/provider"
	"journaletto/internal/worker"
)

func TestMigrationSeedsDetectionRules(t *testing.T) {
	db, _ := setup(t)
	for _, id := range []string{"closed-ticket", "merged-pr"} {
		var name, desc, prompt string
		var enabled, version int
		var min float64
		err := db.QueryRow(`SELECT name,description,detection_prompt,enabled,minimum_confidence,version FROM detection_rules WHERE id=?`, id).
			Scan(&name, &desc, &prompt, &enabled, &min, &version)
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		if enabled != 1 || min != 0.8 || version != 1 || name == "" || desc == "" || prompt == "" {
			t.Errorf("%s: unexpected seed %q %q %q %d %v %d", id, name, desc, prompt, enabled, min, version)
		}
	}
}

func TestMatchingEntryGetsOccurrencesWithExcerptConfidenceAndProvenance(t *testing.T) {
	db, svc := setup(t)
	id := saveBody(t, svc, "Standup", "Closed ticket JRN-12 after the fix.\n\nMerged PR 45 into main.\n\nNothing else.")
	start(t, &worker.Worker{DB: db, Steps: worker.DefaultSteps(db, provider.Fake{})})
	waitStatus(t, db, id, "ready")

	for _, r := range []string{"closed-ticket", "merged-pr"} {
		var excerpt, prov, model, pv string
		var conf float64
		var ver int
		err := db.QueryRow(`SELECT excerpt,confidence,rule_version,provider,model,prompt_version FROM occurrences WHERE entry_id=? AND rule_id=?`, id, r).
			Scan(&excerpt, &conf, &ver, &prov, &model, &pv)
		if err != nil {
			t.Fatalf("%s: %v", r, err)
		}
		if excerpt == "" || conf < 0.8 || ver != 1 || prov != "fake" || model == "" || pv == "" {
			t.Errorf("%s: %q %v %d %s %s %s", r, excerpt, conf, ver, prov, model, pv)
		}
	}
}

func TestNonMatchingEntryHasNoOccurrences(t *testing.T) {
	db, svc := setup(t)
	id := saveBody(t, svc, "Quiet", "Went for a walk.")
	start(t, &worker.Worker{DB: db, Steps: worker.DefaultSteps(db, provider.Fake{})})
	waitStatus(t, db, id, "ready")
	if n := count(t, db, `SELECT COUNT(*) FROM occurrences WHERE entry_id=?`, id); n != 0 {
		t.Fatalf("got %d occurrences", n)
	}
}

func TestOccurrencesBelowMinimumConfidenceAreIgnored(t *testing.T) {
	db, svc := setup(t)
	id := saveBody(t, svc, "Unsure", "Maybe closed ticket JRN-9, not certain.")
	start(t, &worker.Worker{DB: db, Steps: worker.DefaultSteps(db, provider.Fake{})})
	waitStatus(t, db, id, "ready")
	if n := count(t, db, `SELECT COUNT(*) FROM occurrences WHERE entry_id=?`, id); n != 0 {
		t.Fatalf("low-confidence occurrence recorded: %d", n)
	}
}

func TestReprocessingNeverDoubleCountsAndDistinctEventsAreSeparate(t *testing.T) {
	db, svc := setup(t)
	id := saveBody(t, svc, "Busy", "Closed ticket JRN-1.\n\nClosed ticket JRN-2.\n\nClosed ticket JRN-2.")
	start(t, &worker.Worker{DB: db, Steps: worker.DefaultSteps(db, provider.Fake{})})
	waitDone(t, db, id, 1)
	q := `SELECT COUNT(*) FROM occurrences WHERE entry_id=? AND rule_id='closed-ticket'`
	if n := count(t, db, q, id); n != 2 {
		t.Fatalf("first run: %d occurrences, want 2 distinct events", n)
	}
	for i := 2; i <= 3; i++ {
		if err := worker.Resync(context.Background(), db, "u", id); err != nil {
			t.Fatal(err)
		}
		waitDone(t, db, id, i)
		if n := count(t, db, q, id); n != 2 {
			t.Fatalf("after reprocess %d: %d occurrences, want 2", i, n)
		}
	}
}

func TestRuleVersionChangeRequeuesActiveEntriesAndReplacesOccurrences(t *testing.T) {
	db, svc := setup(t)
	id := saveBody(t, svc, "Release", "Merged PR 7.")
	start(t, &worker.Worker{DB: db, Steps: worker.DefaultSteps(db, provider.Fake{})})
	waitDone(t, db, id, 1)

	n, err := worker.RequeueStaleRules(context.Background(), db)
	if err != nil || n != 0 {
		t.Fatalf("up-to-date entries requeued: %d, %v", n, err)
	}
	if _, err := db.Exec(`UPDATE detection_rules SET version=2 WHERE id='merged-pr'`); err != nil {
		t.Fatal(err)
	}
	if n, err = worker.RequeueStaleRules(context.Background(), db); err != nil || n != 1 {
		t.Fatalf("requeued %d, %v; want 1", n, err)
	}
	waitDone(t, db, id, 2)
	var ver int
	if err := db.QueryRow(`SELECT rule_version FROM occurrences WHERE entry_id=? AND rule_id='merged-pr'`, id).Scan(&ver); err != nil || ver != 2 {
		t.Fatalf("rule_version %d, %v; want 2", ver, err)
	}
	if total := count(t, db, `SELECT COUNT(*) FROM occurrences WHERE entry_id=?`, id); total != 1 {
		t.Fatalf("%d occurrences, want 1", total)
	}
}

func TestDisabledRuleIsNotEvaluated(t *testing.T) {
	db, svc := setup(t)
	if _, err := db.Exec(`UPDATE detection_rules SET enabled=0 WHERE id='merged-pr'`); err != nil {
		t.Fatal(err)
	}
	id := saveBody(t, svc, "Release", "Merged PR 7.")
	start(t, &worker.Worker{DB: db, Steps: worker.DefaultSteps(db, provider.Fake{})})
	waitStatus(t, db, id, "ready")
	if n := count(t, db, `SELECT COUNT(*) FROM occurrences WHERE entry_id=?`, id); n != 0 {
		t.Fatalf("disabled rule produced %d occurrences", n)
	}
}
