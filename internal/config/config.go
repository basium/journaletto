// Package config loads settings from an optional YAML file, overridden by environment variables.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

// Config holds every runtime setting. See config.example.yaml and .env.example.
type Config struct {
	Port              int    `yaml:"port"`
	DevUserID         string `yaml:"dev_user_id"`
	DataRoot          string `yaml:"data_root"`
	DBPath            string `yaml:"db_path"`
	Provider          string `yaml:"provider"` // fake | local | external
	MaxRetries        int    `yaml:"max_retries"`
	MaxEntryBytes     int    `yaml:"max_entry_bytes"`
	WorkerConcurrency int    `yaml:"worker_concurrency"`
	Version           string `yaml:"-"`
	Commit            string `yaml:"-"`

	// Ollama adapter (provider: local).
	OllamaEndpoint       string  `yaml:"ollama_endpoint"`
	OllamaLLMModel       string  `yaml:"ollama_llm_model"`
	OllamaEmbedModel     string  `yaml:"ollama_embed_model"`
	OllamaTimeoutSeconds int     `yaml:"ollama_timeout_seconds"`
	OllamaNumCtx         int     `yaml:"ollama_num_ctx"`
	OllamaTemperature    float64 `yaml:"ollama_temperature"`
}

func defaults() Config {
	return Config{
		Port: 3000, DevUserID: "dev-user", DataRoot: "./data", DBPath: "./data/journaletto.db",
		Provider: "fake", MaxRetries: 3, MaxEntryBytes: 102400, WorkerConcurrency: 1,
		Version: "dev", Commit: "unknown",
		OllamaEndpoint: "http://localhost:11434", OllamaLLMModel: "llama3.1", OllamaEmbedModel: "nomic-embed-text",
		OllamaTimeoutSeconds: 600, OllamaNumCtx: 8192,
	}
}

// Load reads CONFIG_FILE (default ./config.yaml, optional), then applies env overrides.
func Load() (Config, error) {
	c := defaults()
	path := os.Getenv("CONFIG_FILE")
	explicit := path != ""
	if !explicit {
		path = "config.yaml"
	}
	if b, err := os.ReadFile(path); err == nil {
		if err := yaml.Unmarshal(b, &c); err != nil {
			return c, fmt.Errorf("parse %s: %w", path, err)
		}
	} else if explicit || !errors.Is(err, os.ErrNotExist) {
		return c, fmt.Errorf("read %s: %w", path, err)
	}
	setStr := func(k string, dst *string) {
		if v := os.Getenv(k); v != "" {
			*dst = v
		}
	}
	var err error
	setInt := func(k string, dst *int) {
		if v := os.Getenv(k); v != "" {
			n, e := strconv.Atoi(v)
			if e != nil {
				err = fmt.Errorf("%s must be an integer: %w", k, e)
				return
			}
			*dst = n
		}
	}
	setInt("PORT", &c.Port)
	setStr("DEV_USER_ID", &c.DevUserID)
	setStr("DATA_ROOT", &c.DataRoot)
	setStr("DB_PATH", &c.DBPath)
	setStr("PROVIDER", &c.Provider)
	setInt("MAX_RETRIES", &c.MaxRetries)
	setInt("MAX_ENTRY_BYTES", &c.MaxEntryBytes)
	setInt("WORKER_CONCURRENCY", &c.WorkerConcurrency)
	setStr("OLLAMA_ENDPOINT", &c.OllamaEndpoint)
	setStr("OLLAMA_LLM_MODEL", &c.OllamaLLMModel)
	setStr("OLLAMA_EMBED_MODEL", &c.OllamaEmbedModel)
	setInt("OLLAMA_TIMEOUT_SECONDS", &c.OllamaTimeoutSeconds)
	setInt("OLLAMA_NUM_CTX", &c.OllamaNumCtx)
	if v := os.Getenv("OLLAMA_TEMPERATURE"); v != "" {
		f, e := strconv.ParseFloat(v, 64)
		if e != nil {
			return c, fmt.Errorf("OLLAMA_TEMPERATURE must be a number: %w", e)
		}
		c.OllamaTemperature = f
	}
	setStr("VERSION", &c.Version)
	setStr("COMMIT", &c.Commit)
	if err != nil {
		return c, err
	}
	if c.WorkerConcurrency < 1 || c.MaxRetries < 0 {
		return c, fmt.Errorf("worker_concurrency must be >= 1 and max_retries >= 0")
	}
	switch c.Provider {
	case "fake", "local", "external":
	default:
		return c, fmt.Errorf("provider must be fake|local|external, got %q", c.Provider)
	}
	return c, nil
}
