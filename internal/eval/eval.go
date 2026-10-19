// Package eval measures extraction quality (entities and detection-rule hits) of any provider
// against a small labelled set of sample entries under testdata/.
//
// Each sample is two files: NAME.md (the entry body) and NAME.json
// ({"title": ..., "entities": [values], "rules": [rule ids]}). Entities are compared by
// normalised value (case-insensitive, whitespace-collapsed); rule hits by rule id. The run goes
// through the real worker steps (grounding included) against a throwaway database.
package eval

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"journaletto/internal/entries"
	"journaletto/internal/provider"
	"journaletto/internal/store"
	"journaletto/internal/worker"
)

// Sample is one labelled entry.
type Sample struct {
	Name     string
	Title    string
	Body     string
	Entities []string // expected entity values
	Rules    []string // ids of rules expected to fire
	// Relations are expected entity links written "from > to" (entity values; kind is not compared).
	Relations []string
}

// Score is precision and recall of a set of results against the expected set.
type Score struct{ Precision, Recall float64 }

// Result is the outcome for one sample.
type Result struct {
	Name     string
	Entities Score
	Rules    Score
	// Relations scores stored entity relations ("from > to"); only meaningful when HasRelations.
	Relations Score
	// HasRelations is true when the sample expects at least one relation; other samples are
	// shown as n/a and left out of the relation mean.
	HasRelations bool
	Got          struct{ Entities, Rules, Relations []string }

	// SummaryClean is true when the stored summary is non-empty and has no leading LLM preamble.
	SummaryClean bool
}

func norm(s string) string { return strings.ToLower(strings.Join(strings.Fields(s), " ")) }

func set(vals []string) map[string]bool {
	m := map[string]bool{}
	for _, v := range vals {
		if n := norm(v); n != "" {
			m[n] = true
		}
	}
	return m
}

// Measure returns precision (share of got that is expected) and recall (share of expected that
// was got). Comparison is case-insensitive and whitespace-tolerant. An empty got has precision 1
// and an empty expected has recall 1, so a correct "nothing" scores perfectly.
func Measure(expected, got []string) Score {
	e, g := set(expected), set(got)
	hit := 0
	for v := range g {
		if e[v] {
			hit++
		}
	}
	s := Score{Precision: 1, Recall: 1}
	if len(g) > 0 {
		s.Precision = float64(hit) / float64(len(g))
	}
	if len(e) > 0 {
		s.Recall = float64(hit) / float64(len(e))
	}
	return s
}

// Load reads every NAME.md/NAME.json pair in dir, sorted by name.
func Load(dir string) ([]Sample, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	var out []Sample
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		var exp struct {
			Title     string   `json:"title"`
			Entities  []string `json:"entities"`
			Rules     []string `json:"rules"`
			Relations []string `json:"relations"`
		}
		if err := json.Unmarshal(raw, &exp); err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		name := strings.TrimSuffix(filepath.Base(f), ".json")
		body, err := os.ReadFile(filepath.Join(dir, name+".md"))
		if err != nil {
			return nil, err
		}
		out = append(out, Sample{Name: name, Title: exp.Title, Body: string(body), Entities: exp.Entities, Rules: exp.Rules, Relations: exp.Relations})
	}
	return out, nil
}

// Run extracts entities and rule hits for every sample with p and scores them.
func Run(ctx context.Context, p provider.Provider, samples []Sample) ([]Result, error) {
	dir, err := os.MkdirTemp("", "journaletto-eval-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	db, err := store.Open(filepath.Join(dir, "eval.db"))
	if err != nil {
		return nil, err
	}
	defer db.Close()
	svc := &entries.Service{DB: db, DataRoot: dir, UserID: "eval"}
	steps := []worker.Step{worker.SummaryTagsStep(db, p), worker.EntitiesStep(db, p), worker.RulesStep(db, p)}

	var results []Result
	for _, s := range samples {
		saved, err := svc.Save(ctx, entries.Input{Title: s.Title, Description: s.Body})
		if err != nil {
			return nil, fmt.Errorf("%s: %w", s.Name, err)
		}
		e := worker.Entry{ID: saved.ID, UserID: "eval", Title: s.Title, Body: s.Body, Path: saved.Path}
		for _, st := range steps {
			if err := st.Run(ctx, e); err != nil {
				return nil, fmt.Errorf("%s: %s: %w", s.Name, st.Name, err)
			}
		}
		r := Result{Name: s.Name}
		if r.Got.Entities, err = column(ctx, db, `SELECT value FROM entities WHERE entry_id=?`, saved.ID); err != nil {
			return nil, err
		}
		if r.Got.Rules, err = column(ctx, db, `SELECT DISTINCT rule_id FROM occurrences WHERE entry_id=?`, saved.ID); err != nil {
			return nil, err
		}
		sum, err := column(ctx, db, `SELECT content FROM entry_artifacts WHERE entry_id=? AND kind=?`, saved.ID, worker.KindSummary)
		if err != nil {
			return nil, err
		}
		r.SummaryClean = len(sum) == 1 && sum[0] != "" && worker.StripPreamble(sum[0]) == sum[0]
		if r.Got.Relations, err = column(ctx, db, `SELECT f.value || ' > ' || t.value FROM entity_relations r
			JOIN entities f ON f.id=r.from_id JOIN entities t ON t.id=r.to_id WHERE f.entry_id=?`, saved.ID); err != nil {
			return nil, err
		}
		r.Relations, r.HasRelations = Measure(s.Relations, r.Got.Relations), len(set(s.Relations)) > 0
		r.Entities, r.Rules = Measure(s.Entities, r.Got.Entities), Measure(s.Rules, r.Got.Rules)
		results = append(results, r)
	}
	return results, nil
}

func column(ctx context.Context, db *sql.DB, q string, args ...any) ([]string, error) {
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// Report formats per-entry precision/recall plus the mean over all entries.
func Report(results []Result) string {
	var b strings.Builder
	var ep, er, rp, rr, lp, lr float64
	withRel := 0
	fmt.Fprintf(&b, "%-28s %9s %9s   %9s %9s   %9s %9s\n", "entry", "ent.prec", "ent.rec", "rule.prec", "rule.rec", "rel.prec", "rel.rec")
	for _, r := range results {
		rel := fmt.Sprintf("%9s %9s", "n/a", "n/a")
		if r.HasRelations {
			rel = fmt.Sprintf("%9.2f %9.2f", r.Relations.Precision, r.Relations.Recall)
			lp, lr = lp+r.Relations.Precision, lr+r.Relations.Recall
			withRel++
		}
		fmt.Fprintf(&b, "%-28s %9.2f %9.2f   %9.2f %9.2f   %s\n", r.Name, r.Entities.Precision, r.Entities.Recall, r.Rules.Precision, r.Rules.Recall, rel)
		ep, er, rp, rr = ep+r.Entities.Precision, er+r.Entities.Recall, rp+r.Rules.Precision, rr+r.Rules.Recall
	}
	fmt.Fprintf(&b, "samples expecting relations: %d/%d (relation mean covers only these)\n", withRel, len(results))
	clean := 0
	for _, r := range results {
		if r.SummaryClean {
			clean++
		}
	}
	fmt.Fprintf(&b, "summaries without preamble: %d/%d\n", clean, len(results))
	if n := float64(len(results)); n > 0 {
		relMean := fmt.Sprintf("%9s %9s", "n/a", "n/a")
		if withRel > 0 {
			relMean = fmt.Sprintf("%9.2f %9.2f", lp/float64(withRel), lr/float64(withRel))
		}
		fmt.Fprintf(&b, "%-28s %9.2f %9.2f   %9.2f %9.2f   %s\n", "MEAN", ep/n, er/n, rp/n, rr/n, relMean)
	}
	return b.String()
}
