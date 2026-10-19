package worker

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	"journaletto/internal/provider"
)

// RulesPromptVersion is recorded with each occurrence; bump when the prompt template changes.
const RulesPromptVersion = "rules-v1"

// Rule is a developer-configured detection rule (stored in detection_rules; there is no UI).
type Rule struct {
	ID, Name, Description, DetectionPrompt string
	Enabled                                bool
	MinimumConfidence                      float64
	Version                                int
}

type rawOccurrence struct {
	Event      string  `json:"event"`
	Excerpt    string  `json:"excerpt"`
	Confidence float64 `json:"confidence"`
}

func rulePrompt(r Rule, e Entry) string {
	return provider.DetectPromptPrefix + " for rule " + r.ID + ".\n" + r.DetectionPrompt +
		"\nAnswer with only a JSON array of {\"event\":E,\"excerpt\":X,\"confidence\":C} with one item per distinct event, " +
		"where E is a short identifier of the event, X is the supporting text quoted from the entry and C is between 0 and 1.\n\nEntry:\n" +
		e.Title + "\n\n" + e.Body
}

func enabledRules(ctx context.Context, db *sql.DB) ([]Rule, error) {
	rows, err := db.QueryContext(ctx, `SELECT id,name,description,detection_prompt,enabled,minimum_confidence,version
		FROM detection_rules WHERE enabled=1 ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var rules []Rule
	for rows.Next() {
		var r Rule
		if err := rows.Scan(&r.ID, &r.Name, &r.Description, &r.DetectionPrompt, &r.Enabled, &r.MinimumConfidence, &r.Version); err != nil {
			return nil, err
		}
		rules = append(rules, r)
	}
	return rules, rows.Err()
}

// RulesStep evaluates every enabled detection rule against the entry and records occurrences
// with excerpt, confidence and provenance. Occurrences below the rule's minimum confidence are
// ignored. Each rule's occurrences for the entry are replaced wholesale and are unique per
// distinct event, so reprocessing never double-counts.
func RulesStep(db *sql.DB, p provider.Provider) Step {
	return Step{Name: "rules", Run: func(ctx context.Context, e Entry) error {
		rules, err := enabledRules(ctx, db)
		if err != nil {
			return err
		}
		name, model := p.Identity()
		g := newGrounding(e)
		for _, r := range rules {
			out, err := p.Complete(ctx, rulePrompt(r, e))
			if err != nil {
				return err
			}
			var raw []rawOccurrence
			if err := parseJSONArray(out, &raw); err != nil {
				return fmt.Errorf("rules: rule %s: invalid JSON: %w", r.ID, err)
			}
			best := map[string]rawOccurrence{}
			var order []string
			for _, o := range raw {
				o.Excerpt = normSpace(o.Excerpt)
				key := strings.ToLower(normSpace(o.Event))
				if key == "" {
					key = strings.ToLower(o.Excerpt)
				}
				if key == "" || o.Excerpt == "" || o.Confidence < r.MinimumConfidence || o.Confidence > 1 {
					log.Printf("rules: entry %s: ignored occurrence of rule %s (confidence=%v)", e.ID, r.ID, o.Confidence)
					continue
				}
				if !g.contains(o.Excerpt) {
					g.drop("rule excerpt rule="+r.ID, notInEntry)
					continue
				}
				prev, seen := best[key]
				if !seen {
					order = append(order, key)
				}
				if !seen || o.Confidence > prev.Confidence {
					best[key] = o
				}
			}
			if err := storeOccurrences(ctx, db, e.ID, r, order, best, name, model); err != nil {
				return err
			}
		}
		return nil
	}}
}

func storeOccurrences(ctx context.Context, db *sql.DB, entryID string, r Rule, order []string,
	best map[string]rawOccurrence, name, model string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM occurrences WHERE entry_id=? AND rule_id=?`, entryID, r.ID); err != nil {
		return err
	}
	for _, key := range order {
		o := best[key]
		if _, err := tx.ExecContext(ctx, `INSERT INTO occurrences(entry_id,rule_id,rule_version,event_key,excerpt,confidence,provider,model,prompt_version,generated_at)
			VALUES(?,?,?,?,?,?,?,?,?,?)`, entryID, r.ID, r.Version, key, o.Excerpt, o.Confidence, name, model, RulesPromptVersion, now); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO rule_evaluations(entry_id,rule_id,rule_version) VALUES(?,?,?)
		ON CONFLICT(entry_id,rule_id) DO UPDATE SET rule_version=excluded.rule_version`, entryID, r.ID, r.Version); err != nil {
		return err
	}
	return tx.Commit()
}

// RequeueStaleRules queues reprocessing of active, finished entries that were last evaluated
// against an older version of an enabled rule (or never evaluated). It returns how many entries
// were requeued. Run it after changing a rule's version; the worker also runs it at startup.
func RequeueStaleRules(ctx context.Context, db *sql.DB) (int, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	const stale = `SELECT e.id FROM entries e WHERE e.status='active' AND e.processing_status='ready'
		AND EXISTS (SELECT 1 FROM detection_rules r WHERE r.enabled=1 AND NOT EXISTS
			(SELECT 1 FROM rule_evaluations v WHERE v.entry_id=e.id AND v.rule_id=r.id AND v.rule_version=r.version))`
	if _, err := tx.ExecContext(ctx, `INSERT INTO jobs(entry_id,state,run_after) SELECT s.id,'queued',0 FROM (`+stale+`) s
		WHERE NOT EXISTS (SELECT 1 FROM jobs j WHERE j.entry_id=s.id AND j.state IN ('queued','running'))`); err != nil {
		return 0, err
	}
	res, err := tx.ExecContext(ctx, `UPDATE entries SET processing_status='pending' WHERE id IN (`+stale+`)
		AND EXISTS (SELECT 1 FROM jobs j WHERE j.entry_id=entries.id AND j.state IN ('queued','running'))`)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(n), tx.Commit()
}
