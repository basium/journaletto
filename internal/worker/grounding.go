package worker

import (
	"log"
	"strings"
	"unicode"
)

// Grounding keeps LLM output only when the entry text supports it. Section chunks (parser output)
// and the summary (a paraphrase) are exempt. Drops are logged by reason only: entry id, type and
// reason, never the dropped content.

// foldText normalises text for comparison: lower case, every run of non-letter/digit characters
// (whitespace, markdown punctuation such as * _ ` # > [ ] ( ), ordinary punctuation) becomes one
// space.
func foldText(s string) string {
	var b strings.Builder
	space := true
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			space = false
		} else if !space {
			b.WriteByte(' ')
			space = true
		}
	}
	return strings.TrimSpace(b.String())
}

// grounding holds the normalised entry text (title and body) that output is checked against.
type grounding struct {
	entryID string
	text    string          // folded title + body
	words   map[string]bool // folded words of the entry
}

func newGrounding(e Entry) grounding {
	text := foldText(e.Title + "\n" + e.Body)
	g := grounding{entryID: e.ID, text: text, words: map[string]bool{}}
	for _, w := range strings.Fields(text) {
		g.words[w] = true
	}
	return g
}

// contains reports whether s, normalised, occurs verbatim in the entry text. It is the test for
// entity values and rule excerpts. Text that normalises to nothing is never supported.
func (g grounding) contains(s string) bool {
	f := foldText(s)
	return f != "" && strings.Contains(g.text, f)
}

var stopWords = func() map[string]bool {
	m := map[string]bool{}
	for _, w := range strings.Fields(`the and for are but not you all any can had her was one our out has have this that with from
		they been were will would there their what about which when who how its into than then them these those also very
		just only over such some more most other after before while during because being does did doing his she him her`) {
		m[w] = true
	}
	return m
}()

// minFactSupport is the share of a fact's content words that must appear in the entry.
const minFactSupport = 0.6

// supportsFact is the test for LLM-written fact sentences, which paraphrase and so cannot be
// matched verbatim. A fact is supported when (1) it has at least one content word (a word of
// 3+ characters that is not a stop word), (2) every number in it appears in the entry, and
// (3) at least minFactSupport (60%) of its content words appear in the entry. A word appears when
// the entry has the same word or one sharing its first 5 letters (so "review" matches "reviewed").
// The entry title counts as entry text.
func (g grounding) supportsFact(fact string) bool {
	var content, found int
	for _, w := range strings.Fields(foldText(fact)) {
		numeric := strings.IndexFunc(w, unicode.IsDigit) >= 0
		if !numeric && (len(w) < 3 || stopWords[w]) {
			continue
		}
		if numeric && !g.words[w] {
			return false
		}
		content++
		if g.hasWord(w) {
			found++
		}
	}
	return content > 0 && float64(found) >= minFactSupport*float64(content)
}

func (g grounding) hasWord(w string) bool {
	if g.words[w] {
		return true
	}
	p := prefix5(w)
	if p == "" {
		return false
	}
	for ew := range g.words {
		if prefix5(ew) == p {
			return true
		}
	}
	return false
}

// prefix5 is the first 5 letters of w, or "" when w is shorter.
func prefix5(w string) string {
	r := []rune(w)
	if len(r) < 5 {
		return ""
	}
	return string(r[:5])
}

// drop logs a dropped item by reason only.
func (g grounding) drop(kind, reason string) {
	log.Printf("grounding: entry %s: dropped %s (reason=%s)", g.entryID, kind, reason)
}

const notInEntry = "not supported by entry text"
