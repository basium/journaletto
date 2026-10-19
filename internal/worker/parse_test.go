package worker

import "testing"

// answers every step's parser must cope with, whatever item type the step decodes into.
var wrapped = []struct {
	name, in string
	wantLen  int
	wantErr  bool
}{
	{"bare array", `[{"x":1}]`, 1, false},
	{"prose and fence", "Here is the result:\n```json\n[{\"x\":1},{\"x\":2}]\n```\nHope it helps!", 2, false},
	{"brackets in prose before and after", "See [note] and {braces}: [{\"x\":1}] (cf. [1])", 1, false},
	{"empty array", "Nothing found: []", 0, false},
	{"object wrapping an array is the wrong shape", `{"items":[{"x":1}]}`, 0, true},
	{"prose then wrapping object", "Result: {\"items\":[{\"x\":1}]}", 0, true},
	{"no JSON", "I could not find anything.", 0, true},
}

func TestParseJSONArrayForEntities(t *testing.T) {
	for _, c := range wrapped {
		var got []rawEntity
		err := parseJSONArray(c.in, &got)
		if (err != nil) != c.wantErr || len(got) != c.wantLen {
			t.Errorf("%s: len=%d err=%v, want len=%d err=%v", c.name, len(got), err, c.wantLen, c.wantErr)
		}
	}
}

func TestParseJSONArrayForRuleOccurrences(t *testing.T) {
	for _, c := range wrapped {
		var got []rawOccurrence
		err := parseJSONArray(c.in, &got)
		if (err != nil) != c.wantErr || len(got) != c.wantLen {
			t.Errorf("%s: len=%d err=%v, want len=%d err=%v", c.name, len(got), err, c.wantLen, c.wantErr)
		}
	}
}

func TestParseJSONArrayForFactChunks(t *testing.T) {
	for _, c := range wrapped {
		var got []rawFact
		err := parseJSONArray(c.in, &got)
		if (err != nil) != c.wantErr || len(got) != c.wantLen {
			t.Errorf("%s: len=%d err=%v, want len=%d err=%v", c.name, len(got), err, c.wantLen, c.wantErr)
		}
	}
	var facts []rawFact
	if err := parseJSONArray("Facts: [\"Ada closed JRN-12\", {\"fact\":\"PR 45 merged\",\"section\":\"Done\"}]", &facts); err != nil || len(facts) != 2 {
		t.Errorf("string and object facts: %v %v", facts, err)
	}
}
