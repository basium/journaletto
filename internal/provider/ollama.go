package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// OllamaConfig configures the local Ollama adapter.
type OllamaConfig struct {
	Endpoint    string        // base URL, e.g. http://localhost:11434
	LLMModel    string        // generation model
	EmbedModel  string        // embedding model
	Timeout     time.Duration // per request; 0 means 600s
	NumCtx      int           // context window (num_ctx); 0 leaves the model default
	Temperature float64       // sampling temperature
}

// Ollama implements Provider against a local Ollama server.
type Ollama struct {
	cfg    OllamaConfig
	client *http.Client
}

// NewOllama returns an adapter for cfg.
func NewOllama(cfg OllamaConfig) *Ollama {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 600 * time.Second
	}
	cfg.Endpoint = strings.TrimRight(cfg.Endpoint, "/")
	return &Ollama{cfg: cfg, client: &http.Client{Timeout: cfg.Timeout}}
}

// Identity reports the adapter name and generation model recorded as provenance.
func (o *Ollama) Identity() (string, string) { return "ollama", o.cfg.LLMModel }

// Complete calls /api/generate without streaming.
func (o *Ollama) Complete(ctx context.Context, prompt string) (string, error) {
	opts := map[string]any{"temperature": o.cfg.Temperature}
	if o.cfg.NumCtx > 0 {
		opts["num_ctx"] = o.cfg.NumCtx
	}
	var res struct {
		Response string `json:"response"`
	}
	if err := o.post(ctx, "/api/generate", map[string]any{
		"model": o.cfg.LLMModel, "prompt": prompt, "stream": false, "options": opts,
	}, &res); err != nil {
		return "", err
	}
	return res.Response, nil
}

// Embed calls /api/embed with the embedding model.
func (o *Ollama) Embed(ctx context.Context, text string) ([]float32, error) {
	var res struct {
		Embeddings [][]float32 `json:"embeddings"`
	}
	if err := o.post(ctx, "/api/embed", map[string]any{"model": o.cfg.EmbedModel, "input": text}, &res); err != nil {
		return nil, err
	}
	if len(res.Embeddings) != 1 || len(res.Embeddings[0]) == 0 {
		return nil, fmt.Errorf("ollama: no embedding returned")
	}
	return res.Embeddings[0], nil
}

func (o *Ollama) post(ctx context.Context, path string, body any, out any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.cfg.Endpoint+path, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := o.client.Do(req)
	if err != nil {
		return fmt.Errorf("ollama %s: %v: %w", path, err, ErrUnavailable)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return fmt.Errorf("ollama %s: %w", path, err)
	}
	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("ollama %s: HTTP %d: %s", path, resp.StatusCode, strings.TrimSpace(string(data)))
		if resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests {
			return fmt.Errorf("%w: %w", err, ErrUnavailable) // transient: worth retrying
		}
		return err // other 4xx: permanent
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("ollama %s: bad response: %w", path, err)
	}
	return nil
}
