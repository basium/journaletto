// Package provider defines the model-provider interfaces (LLM + embeddings) and their adapters.
package provider

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
)

// LLM generates text.
type LLM interface {
	Complete(ctx context.Context, prompt string) (string, error)
}

// Embedder turns text into a fixed-size vector.
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

// Provider bundles both capabilities.
type Provider interface {
	LLM
	Embedder
	// Identity reports the provider name and model, recorded as provenance on generated artefacts.
	Identity() (name, model string)
}

// ErrUnavailable is returned by adapters whose backend is not reachable or not implemented yet.
var ErrUnavailable = errors.New("provider unavailable")

// New returns the adapter for a configured provider type: local | external | fake.
// local and external are placeholders until a real backend is wired in; they always
// report ErrUnavailable, which exercises the retry/failed path without affecting saves.
func New(kind string) (Provider, error) {
	switch kind {
	case "fake":
		return Fake{}, nil
	case "local", "external":
		return unavailable{kind: kind}, nil
	}
	return nil, fmt.Errorf("unknown provider %q", kind)
}

// FakeDims is the embedding size of the fake adapter.
const FakeDims = 8

// Fake is a deterministic, offline adapter for development and tests.
type Fake struct{}

// EntityPromptPrefix starts every entity-extraction prompt; the fake adapter answers it with
// canned JSON so the pipeline can be tested offline.
const EntityPromptPrefix = "Extract entities"

// FactPromptPrefix starts every fact-chunk prompt; the fake adapter answers with canned JSON.
const FactPromptPrefix = "Extract facts"

// DetectPromptPrefix starts every detection-rule prompt. The fake adapter answers it by
// keyword-matching lines of the entry (after "Entry:\n") for the rule named in the first line:
// "closed ticket" for closed-ticket, "merged pr" for merged-pr; lines containing "maybe" get
// low confidence.
const DetectPromptPrefix = "Detect occurrences"

func fakeDetect(prompt string) string {
	head, entry, _ := strings.Cut(prompt, "\n\nEntry:\n")
	first, _, _ := strings.Cut(head, "\n")
	keyword := map[string]string{"closed-ticket": "closed ticket", "merged-pr": "merged pr"}
	var kw string
	for id, k := range keyword {
		if strings.HasSuffix(strings.TrimSpace(first), "rule "+id+".") {
			kw = k
		}
	}
	out := []map[string]any{}
	for _, line := range strings.Split(entry, "\n") {
		l := strings.TrimSpace(line)
		low := strings.ToLower(l)
		if kw == "" || !strings.Contains(low, kw) {
			continue
		}
		conf := 0.9
		if strings.Contains(low, "maybe") {
			conf = 0.5
		}
		out = append(out, map[string]any{"event": low, "excerpt": l, "confidence": conf})
	}
	b, _ := json.Marshal(out)
	return string(b)
}

// fakeEntry splits an entity/fact prompt (instructions, blank line, title, blank line, body)
// into the entry title and body.
func fakeEntry(prompt string) (title, body string) {
	_, entry, _ := strings.Cut(prompt, "\n\n")
	title, body, _ = strings.Cut(entry, "\n\n")
	return strings.TrimSpace(title), body
}

func (Fake) Complete(_ context.Context, prompt string) (string, error) {
	if strings.HasPrefix(prompt, DetectPromptPrefix) {
		return fakeDetect(prompt), nil
	}
	if strings.HasPrefix(prompt, FactPromptPrefix) {
		// Grounded in the entry: the first non-heading body line.
		_, body := fakeEntry(prompt)
		fact := ""
		for _, l := range strings.Split(body, "\n") {
			if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(l, "#") {
				fact = l
				break
			}
		}
		b, _ := json.Marshal([]string{fact})
		return string(b), nil
	}
	if strings.HasPrefix(prompt, EntityPromptPrefix) {
		// The entry title is always in the entry text; "Fake Project" is kept only when the entry mentions it.
		title, _ := fakeEntry(prompt)
		b, _ := json.Marshal([]map[string]any{
			{"type": "Concepts", "value": title, "relations": []map[string]string{{"to": "Fake Project", "kind": "part_of"}}},
			{"type": "Projects", "value": "Fake Project", "relations": []any{}},
		})
		return string(b), nil
	}
	// Echo the entry text (after the instructions) so a long instruction line cannot crowd it out.
	if _, entry, ok := strings.Cut(prompt, "\n\n"); ok {
		prompt = entry
	}
	r := []rune(strings.TrimSpace(prompt))
	if len(r) > 80 {
		r = r[:80]
	}
	return "fake: " + string(r), nil
}

func (Fake) Identity() (string, string) { return "fake", "fake-1" }

func (Fake) Embed(_ context.Context, text string) ([]float32, error) {
	sum := sha256.Sum256([]byte(text))
	v := make([]float32, FakeDims)
	var norm float64
	for i := range v {
		v[i] = float32(binary.BigEndian.Uint32(sum[i*4:])%2000)/1000 - 1
		norm += float64(v[i]) * float64(v[i])
	}
	if norm > 0 {
		for i := range v {
			v[i] /= float32(math.Sqrt(norm))
		}
	}
	return v, nil
}

type unavailable struct{ kind string }

func (u unavailable) Identity() (string, string) { return u.kind, "unavailable" }

func (u unavailable) Complete(context.Context, string) (string, error) {
	return "", fmt.Errorf("%s: %w", u.kind, ErrUnavailable)
}

func (u unavailable) Embed(context.Context, string) ([]float32, error) {
	return nil, fmt.Errorf("%s: %w", u.kind, ErrUnavailable)
}
