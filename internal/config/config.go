// Package config loads and validates the server's runtime configuration.
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config holds all settings needed to run the journal server.
//
// Only the fields required by the current slice are populated/validated;
// later-milestone fields (SQLite path, vector extension, model provider,
// etc.) live in config.example.yaml as documented-but-unused placeholders
// until the slices that need them are implemented.
type Config struct {
	// DevUserID is the immutable development user ID under which all raw
	// journal files are stored. Authentication is out of scope for the MVP,
	// so this stands in for a real per-user identity.
	DevUserID string `yaml:"dev_user_id"`

	// JournalRoot is the filesystem directory under which per-user journal
	// directories are created. May be relative; resolved relative to the
	// process's working directory.
	JournalRoot string `yaml:"journal_root"`

	// MaxEntryBytes is the maximum size, in bytes, of a fully rendered
	// journal entry file (front matter + heading + body).
	MaxEntryBytes int `yaml:"max_entry_bytes"`

	// MaxTags is the maximum number of user tags allowed on one entry.
	MaxTags int `yaml:"max_tags"`

	// ListenAddr is the address the HTTP server binds to, e.g. ":8080".
	ListenAddr string `yaml:"listen_addr"`
}

const (
	defaultMaxEntryBytes = 100 * 1024
	defaultMaxTags       = 100
	defaultListenAddr    = ":8080"
)

// Load reads and parses the YAML config file at path, applying defaults for
// any absent optional field, then validates the result.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("reading config file %q: %w", path, err)
	}

	cfg := Config{
		MaxEntryBytes: defaultMaxEntryBytes,
		MaxTags:       defaultMaxTags,
		ListenAddr:    defaultListenAddr,
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parsing config file %q: %w", path, err)
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Validate checks that required fields are present and sane.
func (c Config) Validate() error {
	if c.DevUserID == "" {
		return fmt.Errorf("dev_user_id is required")
	}
	if c.JournalRoot == "" {
		return fmt.Errorf("journal_root is required")
	}
	if c.MaxEntryBytes <= 0 {
		return fmt.Errorf("max_entry_bytes must be positive")
	}
	if c.MaxTags <= 0 {
		return fmt.Errorf("max_tags must be positive")
	}
	if c.ListenAddr == "" {
		return fmt.Errorf("listen_addr is required")
	}
	return nil
}
