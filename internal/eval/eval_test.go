package eval_test

import (
	"context"
	"math"
	"strings"
	"testing"

	"journaletto/internal/eval"
	"journaletto/internal/provider"
)

func TestSampleSetLoadsAndRunsWithFakeProvider(t *testing.T) {
	samples, err := eval.Load("testdata")
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) < 10 {
		t.Fatalf("samples = %d, want ~10", len(samples))
	}
	for _, s := range samples {
		if s.Title == "" || strings.TrimSpace(s.Body) == "" || len(s.Entities) == 0 {
			t.Errorf("%s: incomplete sample", s.Name)
		}
	}
	results, err := eval.Run(context.Background(), provider.Fake{}, samples)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != len(samples) {
		t.Fatalf("results = %d", len(results))
	}
	for _, r := range results {
		if !r.SummaryClean {
			t.Errorf("%s: summary not clean", r.Name)
		}
		for _, v := range []float64{r.Entities.Precision, r.Entities.Recall, r.Rules.Precision, r.Rules.Recall} {
			if v < 0 || v > 1 || math.IsNaN(v) {
				t.Errorf("%s: score out of range: %v", r.Name, v)
			}
		}
	}
	if !strings.Contains(eval.Report(results), "MEAN") {
		t.Error("report lacks mean row")
	}
	if !strings.Contains(eval.Report(results), "summaries without preamble") {
		t.Error("report lacks summary preamble check")
	}
}

func TestMeasurePrecisionAndRecall(t *testing.T) {
	s := eval.Measure([]string{"Ada Lovelace", "JRN-12", "Acme"}, []string{"  ada   lovelace", "jrn-12", "Zed", "Zed"})
	if math.Abs(s.Precision-2.0/3) > 1e-9 || math.Abs(s.Recall-2.0/3) > 1e-9 {
		t.Errorf("score = %+v, want 2/3 each", s)
	}
	if e := eval.Measure(nil, nil); e.Precision != 1 || e.Recall != 1 {
		t.Errorf("empty vs empty = %+v, want perfect", e)
	}
	if e := eval.Measure([]string{"a"}, nil); e.Precision != 1 || e.Recall != 0 {
		t.Errorf("nothing found = %+v", e)
	}
}
