package provider_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"journaletto/internal/provider"
)

var _ provider.Provider = (*provider.Ollama)(nil)

func TestOllamaCompleteSendsModelPromptAndOptions(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/generate" || r.Method != http.MethodPost {
			t.Errorf("request %s %s", r.Method, r.URL.Path)
		}
		json.NewDecoder(r.Body).Decode(&got)
		w.Write([]byte(`{"response":"hello","done":true}`))
	}))
	defer srv.Close()
	o := provider.NewOllama(provider.OllamaConfig{Endpoint: srv.URL + "/", LLMModel: "llm-x", EmbedModel: "emb-y", NumCtx: 4096, Temperature: 0.2})
	out, err := o.Complete(context.Background(), "Extract entities: hi")
	if err != nil || out != "hello" {
		t.Fatalf("out=%q err=%v", out, err)
	}
	opts, _ := got["options"].(map[string]any)
	if got["model"] != "llm-x" || got["prompt"] != "Extract entities: hi" || got["stream"] != false ||
		opts["num_ctx"] != float64(4096) || opts["temperature"] != 0.2 {
		t.Errorf("request body: %v", got)
	}
	if name, model := o.Identity(); name != "ollama" || model != "llm-x" {
		t.Errorf("identity %s/%s", name, model)
	}
}

func TestOllamaEmbedReturnsVector(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/embed" {
			t.Errorf("path %s", r.URL.Path)
		}
		json.NewDecoder(r.Body).Decode(&got)
		w.Write([]byte(`{"embeddings":[[0.5,-1,2]]}`))
	}))
	defer srv.Close()
	o := provider.NewOllama(provider.OllamaConfig{Endpoint: srv.URL, LLMModel: "l", EmbedModel: "emb-y"})
	v, err := o.Embed(context.Background(), "text")
	if err != nil || len(v) != 3 || v[0] != 0.5 || v[1] != -1 || v[2] != 2 {
		t.Fatalf("v=%v err=%v", v, err)
	}
	if got["model"] != "emb-y" || got["input"] != "text" {
		t.Errorf("request body: %v", got)
	}
}

func TestOllamaClassifiesHTTPStatuses(t *testing.T) {
	var code int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", code)
	}))
	defer srv.Close()
	o := provider.NewOllama(provider.OllamaConfig{Endpoint: srv.URL, LLMModel: "l", EmbedModel: "e"})
	for _, tc := range []struct {
		code        int
		unavailable bool
	}{{500, true}, {503, true}, {429, true}, {400, false}, {404, false}} {
		code = tc.code
		_, err := o.Embed(context.Background(), "x")
		if err == nil || errors.Is(err, provider.ErrUnavailable) != tc.unavailable || !strings.Contains(err.Error(), strconv.Itoa(tc.code)) {
			t.Errorf("HTTP %d: err=%v, want unavailable=%v", tc.code, err, tc.unavailable)
		}
	}
}

func TestOllamaErrorsAreReportedAndTimeoutApplies(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/generate":
			time.Sleep(300 * time.Millisecond)
			w.Write([]byte(`{"response":"late"}`))
		default:
			http.Error(w, `{"error":"model not found"}`, http.StatusNotFound)
		}
	}))
	defer srv.Close()
	o := provider.NewOllama(provider.OllamaConfig{Endpoint: srv.URL, LLMModel: "l", EmbedModel: "e", Timeout: 50 * time.Millisecond})
	if _, err := o.Complete(context.Background(), "p"); err == nil {
		t.Error("expected timeout error")
	}
	_, err := o.Embed(context.Background(), "x")
	if err == nil || !strings.Contains(err.Error(), "404") {
		t.Errorf("err = %v, want HTTP 404", err)
	}
	// An unreachable endpoint is reported as unavailable so the worker retries.
	srv.Close()
	if _, err := o.Embed(context.Background(), "x"); err == nil || !strings.Contains(err.Error(), provider.ErrUnavailable.Error()) {
		t.Errorf("err = %v, want unavailable", err)
	}
}
