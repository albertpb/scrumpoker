package postgres

import (
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestConnectionConfigDisablesPreparedStatementCache(t *testing.T) {
	config, err := connectionConfig("postgresql://user:password@localhost:5432/scrumpoker")
	if err != nil {
		t.Fatal(err)
	}
	if config.DefaultQueryExecMode != pgx.QueryExecModeExec {
		t.Fatalf("got query mode %q, want %q", config.DefaultQueryExecMode, pgx.QueryExecModeExec)
	}
}
