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

func TestInvalidProviderRejected(t *testing.T) {
	t.Setenv("CONFIG_FILE", "")
	t.Setenv("PROVIDER", "bogus")
	if _, err := config.Load(); err == nil {
		t.Fatal("expected error")
	}
}
