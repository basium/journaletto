//go:build eval

package eval_test

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"journaletto/internal/eval"
	"journaletto/internal/provider"
)

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// TestEvalRealModel runs the sample set against a real local model (Ollama) and prints
// per-entry precision/recall. Opt-in only:
//
//	go test ./internal/eval -run TestEvalRealModel -tags=eval -v -timeout 30m
//
// Configure with OLLAMA_ENDPOINT (default http://localhost:11434) and OLLAMA_LLM_MODEL (default llama3.1).
func TestEvalRealModel(t *testing.T) {
	timeout, _ := strconv.Atoi(env("OLLAMA_TIMEOUT_SECONDS", "300"))
	p := provider.NewOllama(provider.OllamaConfig{
		Endpoint: env("OLLAMA_ENDPOINT", "http://localhost:11434"), LLMModel: env("OLLAMA_LLM_MODEL", "llama3.1"),
		EmbedModel: env("OLLAMA_EMBED_MODEL", "nomic-embed-text"), Timeout: time.Duration(timeout) * time.Second,
	})
	samples, err := eval.Load("testdata")
	if err != nil {
		t.Fatal(err)
	}
	results, err := eval.Run(context.Background(), p, samples)
	if err != nil {
		t.Fatal(err)
	}
	_, model := p.Identity()
	t.Logf("model %s\n%s", model, eval.Report(results))
}
