package database

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

// seedSqlite creates a temp SQLite database with a table containing n rows and
// returns its path. It uses a raw (writable) handle since the adapter opens
// connections in read-only mode.
func seedSqlite(t *testing.T, n int) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	raw, err := sql.Open("sqlite3", f.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	if _, err := raw.Exec("CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT)"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < n; i++ {
		if _, err := raw.Exec("INSERT INTO items (name) VALUES (?)", "n"); err != nil {
			t.Fatal(err)
		}
	}
	return f.Name()
}

func TestRunReadonlyQueryRejectsWrites(t *testing.T) {
	a, err := NewSqliteAdapter(seedSqlite(t, 1))
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()

	if _, err := a.RunReadonlyQuery(context.Background(), "INSERT INTO items (name) VALUES ('x')"); err == nil {
		t.Fatal("expected write to be rejected under read-only enforcement")
	}
}

func TestRunReadonlyQueryCapsRows(t *testing.T) {
	a, err := NewSqliteAdapter(seedSqlite(t, MaxQueryRows*2))
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()

	out, err := a.RunReadonlyQuery(context.Background(), "SELECT * FROM items")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(out, `"id"`); got != MaxQueryRows {
		t.Fatalf("expected %d rows, got %d", MaxQueryRows, got)
	}
}

func TestGetSchemaValidatesTableName(t *testing.T) {
	a, err := NewSqliteAdapter(seedSqlite(t, 1))
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	ctx := context.Background()

	if _, err := a.GetSchema(ctx, "items; DROP TABLE items"); err == nil {
		t.Fatal("expected unknown/injection table name to be rejected")
	}
	if _, err := a.GetSchema(ctx, "items"); err != nil {
		t.Fatalf("GetSchema(items) failed: %v", err)
	}
}

func TestRunReadonlyQueryEmptyResultIsArray(t *testing.T) {
	a, err := NewSqliteAdapter(seedSqlite(t, 0))
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()

	out, err := a.RunReadonlyQuery(context.Background(), "SELECT * FROM items")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != "[]" {
		t.Fatalf("expected empty result to be [], got %q", out)
	}
}
