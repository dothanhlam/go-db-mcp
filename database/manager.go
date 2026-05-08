package database

import (
	"context"
	"fmt"
	"os"
)

// ConnectionManager manages multiple database connections.
type ConnectionManager struct {
	clients map[string]DatabaseClient
}

// NewConnectionManager creates and initializes a ConnectionManager from environment variables.
func NewConnectionManager(ctx context.Context) (*ConnectionManager, error) {
	manager := &ConnectionManager{
		clients: make(map[string]DatabaseClient),
	}

	// Initialize PostgreSQL connections
	pgDSN := os.Getenv("POSTGRES_DSN")
	if pgDSN != "" {
		pgAdapter, err := NewPostgresAdapter(ctx, pgDSN)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize PostgreSQL adapter: %w", err)
		}
		manager.clients["pg_main"] = pgAdapter
		fmt.Println("Initialized PostgreSQL connection: pg_main")
	}

	// Initialize MySQL connections
	mysqlDSN := os.Getenv("MYSQL_DSN")
	if mysqlDSN != "" {
		mysqlAdapter, err := NewMysqlAdapter(mysqlDSN)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize MySQL adapter: %w", err)
		}
		manager.clients["mysql_legacy"] = mysqlAdapter
		fmt.Println("Initialized MySQL connection: mysql_legacy")
	}

	return manager, nil
}

// GetClient retrieves a database client by its connection ID.
func (m *ConnectionManager) GetClient(connectionID string) (DatabaseClient, error) {
	client, exists := m.clients[connectionID]
	if !exists {
		return nil, fmt.Errorf("connection ID '%s' not found", connectionID)
	}
	return client, nil
}

// GetAvailableConnections returns a list of configured connection IDs.
func (m *ConnectionManager) GetAvailableConnections() []string {
	var connections []string
	for id := range m.clients {
		connections = append(connections, id)
	}
	return connections
}
