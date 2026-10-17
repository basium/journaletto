// Package provider defines the model-provider interfaces (LLM + embeddings) and their adapters.
package provider

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
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

func (Fake) Complete(_ context.Context, prompt string) (string, error) {
	if strings.HasPrefix(prompt, EntityPromptPrefix) {
		return `[{"type":"Concepts","value":"Journaling","relations":[{"to":"Fake Project","kind":"part_of"}]},` +
			`{"type":"Projects","value":"Fake Project","relations":[]}]`, nil
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
