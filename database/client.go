package database

import (
	"context"
	"fmt"
)

// MaxQueryRows caps the number of rows returned by RunReadonlyQuery. The cap is
// enforced while scanning the result set, so it cannot be bypassed by the query
// text (unlike appending a LIMIT clause as a string).
const MaxQueryRows = 50

// DatabaseClient defines the interface for interacting with different database engines.
type DatabaseClient interface {
	// ListTables returns a list of all tables in the database.
	ListTables(ctx context.Context) ([]string, error)

	// GetSchema returns the schema or metadata for a specific table.
	GetSchema(ctx context.Context, tableName string) (string, error)

	// RunReadonlyQuery executes a query inside a read-only transaction and
	// returns at most MaxQueryRows rows to prevent memory overload.
	RunReadonlyQuery(ctx context.Context, query string) (string, error)

	// Close releases the underlying database connections.
	Close() error
}

// tableLister is the subset of DatabaseClient needed to validate identifiers.
type tableLister interface {
	ListTables(ctx context.Context) ([]string, error)
}

// validateTableName confirms tableName is a real table, so that adapters which
// must interpolate the identifier (MySQL DESCRIBE, SQLite PRAGMA) can do so
// safely. An allowlist is far more robust than a character denylist.
func validateTableName(ctx context.Context, l tableLister, tableName string) error {
	tables, err := l.ListTables(ctx)
	if err != nil {
		return fmt.Errorf("failed to validate table name: %w", err)
	}
	for _, t := range tables {
		if t == tableName {
			return nil
		}
	}
	return fmt.Errorf("table '%s' not found", tableName)
}
