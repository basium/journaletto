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
	Port          int    `yaml:"port"`
	DevUserID     string `yaml:"dev_user_id"`
	DataRoot      string `yaml:"data_root"`
	DBPath        string `yaml:"db_path"`
	Provider      string `yaml:"provider"` // fake | local | external
	MaxRetries    int    `yaml:"max_retries"`
	MaxEntryBytes int    `yaml:"max_entry_bytes"`
	Version       string `yaml:"-"`
	Commit        string `yaml:"-"`
}

func defaults() Config {
	return Config{
		Port: 3000, DevUserID: "dev-user", DataRoot: "./data", DBPath: "./data/journaletto.db",
		Provider: "fake", MaxRetries: 3, MaxEntryBytes: 65536,
		Version: "dev", Commit: "unknown",
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
	setStr("VERSION", &c.Version)
	setStr("COMMIT", &c.Commit)
	if err != nil {
		return c, err
	}
	switch c.Provider {
	case "fake", "local", "external":
	default:
		return c, fmt.Errorf("provider must be fake|local|external, got %q", c.Provider)
	}
	return c, nil
}
