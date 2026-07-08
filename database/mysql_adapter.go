package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// MysqlAdapter implements DatabaseClient for MySQL.
type MysqlAdapter struct {
	db *sql.DB
}

// NewMysqlAdapter creates a new MysqlAdapter.
func NewMysqlAdapter(dsn string) (*MysqlAdapter, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("unable to connect to database: %v", err)
	}

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	// Test the connection
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("unable to ping database: %v", err)
	}

	return &MysqlAdapter{db: db}, nil
}

func (m *MysqlAdapter) ListTables(ctx context.Context) ([]string, error) {
	query := "SHOW TABLES"
	rows, err := m.db.QueryContext(ctx, query)
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

func (m *MysqlAdapter) GetSchema(ctx context.Context, tableName string) (string, error) {
	// Allowlist the identifier: it must be one of the real tables. This prevents
	// SQL injection since the name cannot be parameterized in a DESCRIBE.
	if err := validateTableName(ctx, m, tableName); err != nil {
		return "", err
	}

	query := fmt.Sprintf("DESCRIBE `%s`", strings.ReplaceAll(tableName, "`", "``"))
	rows, err := m.db.QueryContext(ctx, query)
	if err != nil {
		return "", fmt.Errorf("failed to execute query: %w", err)
	}
	defer rows.Close()

	var schema []map[string]interface{}
	for rows.Next() {
		var field, typ, null, key string
		var def sql.NullString
		var extra string
		if err := rows.Scan(&field, &typ, &null, &key, &def, &extra); err != nil {
			return "", fmt.Errorf("failed to scan row: %w", err)
		}

		colInfo := map[string]interface{}{
			"Field": field,
			"Type":  typ,
			"Null":  null,
			"Key":   key,
			"Extra": extra,
		}
		if def.Valid {
			colInfo["Default"] = def.String
		} else {
			colInfo["Default"] = nil
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

func (m *MysqlAdapter) RunReadonlyQuery(ctx context.Context, query string) (string, error) {
	// Enforce read-only at the database layer via START TRANSACTION READ ONLY.
	tx, err := m.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return "", fmt.Errorf("failed to begin read-only transaction: %w", err)
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx, query)
	if err != nil {
		return "", fmt.Errorf("failed to execute query: %w", err)
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return "", fmt.Errorf("failed to get columns: %w", err)
	}

	results := []map[string]interface{}{}

	// Make a slice for the values
	values := make([]sql.RawBytes, len(columns))

	// rows.Scan wants '[]interface{}' as an argument, so we must copy the
	// references into such a slice
	scanArgs := make([]interface{}, len(values))
	for i := range values {
		scanArgs[i] = &values[i]
	}

	for rows.Next() {
		if len(results) >= MaxQueryRows {
			break
		}
		err = rows.Scan(scanArgs...)
		if err != nil {
			return "", fmt.Errorf("failed to scan row: %w", err)
		}

		rowMap := make(map[string]interface{})
		for i, col := range values {
			// Here we can check if the value is nil (NULL value)
			if col == nil {
				rowMap[columns[i]] = nil
			} else {
				rowMap[columns[i]] = string(col)
			}
		}
		results = append(results, rowMap)
	}

	if len(results) < MaxQueryRows {
		if err := rows.Err(); err != nil {
			return "", fmt.Errorf("rows iteration error: %w", err)
		}
	}

	jsonResult, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal result to JSON: %w", err)
	}

	return string(jsonResult), nil
}

// Close closes the underlying connection pool.
func (m *MysqlAdapter) Close() error {
	return m.db.Close()
}
