package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadEnv(t *testing.T) {
	const key = "SCRUMPOKER_ENV_TEST"
	t.Setenv(key, "")
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte(key+"=loaded\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := LoadEnv(path); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv(key); got != "" {
		t.Fatalf("existing environment value was overwritten with %q", got)
	}
}

func TestLoadEnvSetsMissingVariable(t *testing.T) {
	const key = "SCRUMPOKER_MISSING_ENV_TEST"
	_ = os.Unsetenv(key)
	t.Cleanup(func() { _ = os.Unsetenv(key) })
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte(key+"=loaded\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := LoadEnv(path); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv(key); got != "loaded" {
		t.Fatalf("got %q, want loaded", got)
	}
}

func TestLoadEnvAllowsMissingFile(t *testing.T) {
	if err := LoadEnv(filepath.Join(t.TempDir(), ".env")); err != nil {
		t.Fatal(err)
	}
}
