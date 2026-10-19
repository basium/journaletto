package worker

import (
	"log"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Grounding keeps LLM output only when the entry text supports it. Section chunks (parser output)
// and the summary (a paraphrase) are exempt. Drops are logged by reason only: entry id, type and
// reason, never the dropped content.
//
// Matching works on tokens, not substrings, so "Ada" never matches inside "Canada".
//
// A token is a whitespace-delimited word split further at characters that are neither letters,
// digits, combining marks nor one of the symbols + # . - / @. Markdown wrappers (* _ ` ( ) [ ] > ...)
// therefore fall away, while symbol-bearing tokens survive whole: C++, C#, .NET, node.js, JE-42,
// #118, 2026-10-19 and 3.5. Edge punctuation is trimmed (a trailing "." or "-", a leading "-"),
// but a trailing + or # and a single leading # are kept, so "C++" and "C" stay distinct tokens.
//
// Each token is reduced to a key: lower case, combining marks dropped and common precomposed Latin
// letters mapped to their base letter (see accentFold), so NFC and NFD spellings of a name, and
// "José"/"Jose", compare equal. golang.org/x/text is not a dependency, so this stands in for
// NFKC plus case folding; scripts outside Latin are only lower-cased.

type token struct {
	raw       string // as written, used for capitalisation checks
	key       string // normalised
	sentStart bool   // first token of a sentence
}

var accentFold = strings.NewReplacer(
	"à", "a", "á", "a", "â", "a", "ã", "a", "ä", "a", "å", "a", "ā", "a", "ă", "a", "ą", "a",
	"ç", "c", "ć", "c", "ĉ", "c", "ċ", "c", "č", "c", "ď", "d", "đ", "d",
	"è", "e", "é", "e", "ê", "e", "ë", "e", "ē", "e", "ĕ", "e", "ė", "e", "ę", "e", "ě", "e",
	"ĝ", "g", "ğ", "g", "ġ", "g", "ģ", "g", "ĥ", "h", "ħ", "h",
	"ì", "i", "í", "i", "î", "i", "ï", "i", "ĩ", "i", "ī", "i", "ĭ", "i", "į", "i", "ı", "i", "ĵ", "j", "ķ", "k",
	"ĺ", "l", "ļ", "l", "ľ", "l", "ŀ", "l", "ł", "l", "ñ", "n", "ń", "n", "ņ", "n", "ň", "n",
	"ò", "o", "ó", "o", "ô", "o", "õ", "o", "ö", "o", "ø", "o", "ō", "o", "ŏ", "o", "ő", "o",
	"ŕ", "r", "ŗ", "r", "ř", "r", "ś", "s", "ŝ", "s", "ş", "s", "š", "s", "ţ", "t", "ť", "t", "ŧ", "t",
	"ù", "u", "ú", "u", "û", "u", "ü", "u", "ũ", "u", "ū", "u", "ŭ", "u", "ů", "u", "ű", "u", "ų", "u",
	"ŵ", "w", "ý", "y", "ÿ", "y", "ŷ", "y", "ź", "z", "ż", "z", "ž", "z",
	"ß", "ss", "æ", "ae", "œ", "oe",
)

func tokenKey(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsMark(r) {
			continue
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return accentFold.Replace(b.String())
}

func isTokenRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r) || strings.ContainsRune("+#.-/@", r)
}

// trimToken removes edge punctuation that is not part of the token.
func trimToken(seg string) string {
	seg = strings.TrimRight(seg, ".-/@")
	rest := strings.TrimLeft(seg, "#")
	if hashes := len(seg) - len(rest); hashes == 1 && rest != "" {
		seg = "#" + rest
	} else {
		seg = rest
	}
	seg = strings.TrimLeft(seg, "-/@+")
	if strings.HasPrefix(seg, ".") {
		// ".NET" keeps its dot; a stray leading dot before a non-letter does not.
		if r, _ := utf8.DecodeRuneInString(seg[1:]); !unicode.IsLetter(r) {
			seg = seg[1:]
		}
	}
	if !strings.ContainsFunc(seg, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }) {
		return ""
	}
	return seg
}

func tokenize(s string) []token {
	var out []token
	start := true
	for _, field := range strings.Fields(s) {
		for _, seg := range strings.FieldsFunc(field, func(r rune) bool { return !isTokenRune(r) }) {
			t := trimToken(seg)
			if t == "" {
				continue
			}
			out = append(out, token{raw: t, key: tokenKey(t), sentStart: start})
			start = false
		}
		if last, _ := utf8.DecodeLastRuneInString(field); last == '.' || last == '!' || last == '?' {
			start = true
		}
	}
	return out
}

func tokenKeys(ts []token) []string {
	k := make([]string, len(ts))
	for i, t := range ts {
		k[i] = t.key
	}
	return k
}

// grounding holds the tokenised entry text (title and body) that output is checked against.
type grounding struct {
	entryID string
	keys    []string        // token keys of title + body, in order
	words   map[string]bool // set of keys
}

func newGrounding(e Entry) grounding {
	g := grounding{entryID: e.ID, words: map[string]bool{}}
	g.keys = tokenKeys(tokenize(e.Title + "\n" + e.Body))
	for _, k := range g.keys {
		g.words[k] = true
	}
	return g
}

// contains reports whether s occurs in the entry text as a run of whole tokens, ignoring case,
// accents and markdown. It is the test for entity values and rule excerpts. Text with no tokens is
// never supported.
func (g grounding) contains(s string) bool {
	want := tokenKeys(tokenize(s))
	if len(want) == 0 {
		return false
	}
	for i := 0; i+len(want) <= len(g.keys); i++ {
		j := 0
		for j < len(want) && g.keys[i+j] == want[j] {
			j++
		}
		if j == len(want) {
			return true
		}
	}
	return false
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

// minFactSupport is the weighted share of a fact's content tokens that must appear in the entry.
const minFactSupport = 0.6

// shortWordWeight is the weight of a 3-4 letter word, which can only match exactly and is more
// likely to differ through inflection; longer words and numbers weigh 1.
const shortWordWeight = 0.5

// supportsFact is the test for LLM-written fact sentences, which paraphrase and so cannot be
// matched verbatim. The heuristic, in order:
//  1. Numeric tokens (any token with a digit: 45, 3.5, 2026-10-19, JE-42, #118) stay whole and
//     every one must appear in the entry as the same token.
//  2. An entity-like token (capitalised, not at the start of a sentence, not a stop word) must
//     appear in the entry, so an invented name is rejected even when the rest is supported.
//  3. Content tokens are those of 3+ characters (or any token with a symbol, like "c#") that are
//     not stop words. At least minFactSupport (60%) of them, by weight, must appear in the entry.
//     A word appears when the entry has the same token, or, for words of 5+ letters, one sharing
//     its first 5 letters ("review" matches "reviewed"). 3-4 letter words match exactly and weigh
//     shortWordWeight; everything else weighs 1.
//
// The entry title counts as entry text.
func (g grounding) supportsFact(fact string) bool {
	var total, found float64
	for _, t := range tokenize(fact) {
		w := t.key
		numeric := strings.IndexFunc(w, unicode.IsDigit) >= 0
		symbol := strings.IndexFunc(w, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) >= 0
		n := utf8.RuneCountInString(w)
		if !numeric && !symbol && (n < 3 || stopWords[w]) {
			continue
		}
		if numeric && !g.words[w] {
			return false
		}
		present := g.hasWord(w)
		if !numeric && !t.sentStart && !stopWords[w] && n >= 3 {
			if first, _ := utf8.DecodeRuneInString(t.raw); unicode.IsUpper(first) && !present {
				return false
			}
		}
		weight := 1.0
		if !numeric && !symbol && n < 5 {
			weight = shortWordWeight
		}
		total += weight
		if present {
			found += weight
		}
	}
	return total > 0 && found >= minFactSupport*total
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

// prefix5 is the first 5 letters of w, or "" when w is shorter or its first 5 characters are not
// all letters (so ticket ids and symbol tokens never match by prefix).
func prefix5(w string) string {
	r := []rune(w)
	if len(r) < 5 {
		return ""
	}
	for _, c := range r[:5] {
		if !unicode.IsLetter(c) {
			return ""
		}
	}
	return string(r[:5])
}

// drop logs a dropped item by reason only.
func (g grounding) drop(kind, reason string) {
	log.Printf("grounding: entry %s: dropped %s (reason=%s)", g.entryID, kind, reason)
}

const notInEntry = "not supported by entry text"
