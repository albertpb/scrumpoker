package main

import (
	"strings"
	"testing"
)

func TestOpenRepositorySelectsSQLite(t *testing.T) {
	t.Setenv("DATABASE_PATH", t.TempDir()+"/test.db")

	repository, err := openRepository("sqlite")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
}

func TestOpenRepositoryRequiresPostgresURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")

	_, err := openRepository("postgres")
	if err == nil || !strings.Contains(err.Error(), "DATABASE_URL is required") {
		t.Fatalf("got %v, want missing DATABASE_URL error", err)
	}
}

func TestOpenRepositoryRejectsUnknownDriver(t *testing.T) {
	_, err := openRepository("mysql")
	if err == nil || !strings.Contains(err.Error(), "unsupported DATABASE_DRIVER") {
		t.Fatalf("got %v, want unsupported driver error", err)
	}
}
