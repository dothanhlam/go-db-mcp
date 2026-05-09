package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

// SqliteAdapter implements DatabaseClient for SQLite.
type SqliteAdapter struct {
	db *sql.DB
}

// NewSqliteAdapter creates a new SqliteAdapter.
func NewSqliteAdapter(dsn string) (*SqliteAdapter, error) {
	// DSN for SQLite is just the file path
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("unable to connect to database: %v", err)
	}

	// Test the connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("unable to ping database: %v", err)
	}

	return &SqliteAdapter{db: db}, nil
}

func (s *SqliteAdapter) ListTables(ctx context.Context) ([]string, error) {
	query := `
		SELECT name FROM sqlite_master 
		WHERE type='table' AND name NOT LIKE 'sqlite_%'
		ORDER BY name;
	`
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		tables = append(tables, tableName)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error: %w", err)
	}

	return tables, nil
}

func (s *SqliteAdapter) GetSchema(ctx context.Context, tableName string) (string, error) {
	// Prevent very basic SQL injection
	if strings.ContainsAny(tableName, " ;'`\"") {
		return "", fmt.Errorf("invalid table name")
	}

	query := fmt.Sprintf("PRAGMA table_info('%s')", tableName)
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return "", fmt.Errorf("failed to execute query: %w", err)
	}
	defer rows.Close()

	var schema []map[string]interface{}
	for rows.Next() {
		var cid int
		var name, typ string
		var notnull, pk int
		var dfltValue sql.NullString

		if err := rows.Scan(&cid, &name, &typ, &notnull, &dfltValue, &pk); err != nil {
			return "", fmt.Errorf("failed to scan row: %w", err)
		}

		colInfo := map[string]interface{}{
			"cid":       cid,
			"name":      name,
			"type":      typ,
			"notnull":   notnull,
			"pk":        pk,
		}
		if dfltValue.Valid {
			colInfo["dflt_value"] = dfltValue.String
		} else {
			colInfo["dflt_value"] = nil
		}

		schema = append(schema, colInfo)
	}

	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("rows iteration error: %w", err)
	}

	if len(schema) == 0 {
		return "", fmt.Errorf("table '%s' not found or has no columns", tableName)
	}

	jsonSchema, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal schema to JSON: %w", err)
	}

	return string(jsonSchema), nil
}

func (s *SqliteAdapter) RunReadonlyQuery(ctx context.Context, query string) (string, error) {
	upperQuery := strings.ToUpper(query)
	if !strings.Contains(upperQuery, "LIMIT ") {
		query = strings.TrimSpace(query)
		if strings.HasSuffix(query, ";") {
			query = query[:len(query)-1]
		}
		query = fmt.Sprintf("%s LIMIT 50;", query)
	}

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return "", fmt.Errorf("failed to execute query: %w", err)
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return "", fmt.Errorf("failed to get columns: %w", err)
	}

	var results []map[string]interface{}
	
	// Create a slice of interface{} to hold values
	values := make([]interface{}, len(columns))
	for i := range values {
		values[i] = new(interface{})
	}

	for rows.Next() {
		err = rows.Scan(values...)
		if err != nil {
			return "", fmt.Errorf("failed to scan row: %w", err)
		}

		rowMap := make(map[string]interface{})
		for i, colName := range columns {
			val := *(values[i].(*interface{}))
			
			// Handle byte slices (often strings in sqlite)
			if b, ok := val.([]byte); ok {
				rowMap[colName] = string(b)
			} else {
				rowMap[colName] = val
			}
		}
		results = append(results, rowMap)
	}

	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("rows iteration error: %w", err)
	}

	jsonResult, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal result to JSON: %w", err)
	}

	return string(jsonResult), nil
}
