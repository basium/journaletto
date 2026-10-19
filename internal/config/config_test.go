package config_test

import (
	"testing"

	"journaletto/internal/config"
)

func TestEnvOverridesDefaults(t *testing.T) {
	t.Setenv("CONFIG_FILE", "")
	t.Setenv("DEV_USER_ID", "alice")
	t.Setenv("PORT", "4000")
	c, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.DevUserID != "alice" || c.Port != 4000 {
		t.Fatalf("got %+v", c)
	}
}

func TestOllamaTimeoutDefaultCoversColdCPULoad(t *testing.T) {
	t.Setenv("CONFIG_FILE", "")
	t.Setenv("OLLAMA_TIMEOUT_SECONDS", "")
	c, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.OllamaTimeoutSeconds < 600 {
		t.Fatalf("default timeout %ds too low for cold CPU load", c.OllamaTimeoutSeconds)
	}
}

func TestInvalidProviderRejected(t *testing.T) {
	t.Setenv("CONFIG_FILE", "")
	t.Setenv("PROVIDER", "bogus")
	if _, err := config.Load(); err == nil {
		t.Fatal("expected error")
	}
}
