package database

import "context"

// DatabaseClient defines the interface for interacting with different database engines.
type DatabaseClient interface {
	// ListTables returns a list of all tables in the database.
	ListTables(ctx context.Context) ([]string, error)

	// GetSchema returns the schema or metadata for a specific table.
	GetSchema(ctx context.Context, tableName string) (string, error)

	// RunReadonlyQuery executes a query with a mandatory LIMIT to ensure safety and prevent large results.
	RunReadonlyQuery(ctx context.Context, query string) (string, error)
}
