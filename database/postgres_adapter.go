package database

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresAdapter implements DatabaseClient for PostgreSQL.
type PostgresAdapter struct {
	pool *pgxpool.Pool
}

// NewPostgresAdapter creates a new PostgresAdapter.
func NewPostgresAdapter(ctx context.Context, dsn string) (*PostgresAdapter, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("unable to connect to database: %v", err)
	}

	// Test the connection
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("unable to ping database: %v", err)
	}

	return &PostgresAdapter{pool: pool}, nil
}

func (p *PostgresAdapter) ListTables(ctx context.Context) ([]string, error) {
	query := `
		SELECT table_name 
		FROM information_schema.tables 
		WHERE table_schema = 'public'
	`
	rows, err := p.pool.Query(ctx, query)
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

func (p *PostgresAdapter) GetSchema(ctx context.Context, tableName string) (string, error) {
	query := `
		SELECT column_name, data_type, is_nullable, column_default
		FROM information_schema.columns
		WHERE table_name = $1 AND table_schema = 'public'
		ORDER BY ordinal_position
	`
	rows, err := p.pool.Query(ctx, query, tableName)
	if err != nil {
		return "", fmt.Errorf("failed to execute query: %w", err)
	}
	defer rows.Close()

	var schema []map[string]interface{}
	for rows.Next() {
		var colName, dataType, isNullable string
		var colDefault *string // Can be null
		if err := rows.Scan(&colName, &dataType, &isNullable, &colDefault); err != nil {
			return "", fmt.Errorf("failed to scan row: %w", err)
		}

		colInfo := map[string]interface{}{
			"column_name": colName,
			"data_type":   dataType,
			"is_nullable": isNullable,
		}
		if colDefault != nil {
			colInfo["column_default"] = *colDefault
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

func (p *PostgresAdapter) RunReadonlyQuery(ctx context.Context, query string) (string, error) {
	// Simple append of LIMIT 50. In a robust system, you'd want to parse the SQL properly
	// to inject LIMIT safely, but string appending works for simple cases.
	// Check if the query already has a LIMIT (basic check).
	upperQuery := strings.ToUpper(query)
	if !strings.Contains(upperQuery, "LIMIT ") {
		query = strings.TrimSpace(query)
		if strings.HasSuffix(query, ";") {
			query = query[:len(query)-1] // Remove trailing semicolon
		}
		query = fmt.Sprintf("%s LIMIT 50;", query)
	}

	rows, err := p.pool.Query(ctx, query)
	if err != nil {
		return "", fmt.Errorf("failed to execute query: %w", err)
	}
	defer rows.Close()

	fieldDescriptions := rows.FieldDescriptions()
	var columns []string
	for _, fd := range fieldDescriptions {
		columns = append(columns, string(fd.Name))
	}

	var results []map[string]interface{}
	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			return "", fmt.Errorf("failed to scan row: %w", err)
		}

		rowMap := make(map[string]interface{})
		for i, col := range columns {
			rowMap[col] = values[i]
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
