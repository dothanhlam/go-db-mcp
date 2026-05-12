package database

import (
	"context"
	"fmt"
	"log"
	"sync"
)

// ConnectionManager manages multiple database connections.
type ConnectionManager struct {
	clients map[string]DatabaseClient
	mu      sync.RWMutex
}

// NewConnectionManager creates an empty ConnectionManager.
func NewConnectionManager(ctx context.Context) (*ConnectionManager, error) {
	manager := &ConnectionManager{
		clients: make(map[string]DatabaseClient),
	}
	return manager, nil
}

// AddConnection dynamically registers a new database connection.
func (m *ConnectionManager) AddConnection(ctx context.Context, id, dbType, dsn string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var adapter DatabaseClient
	var err error

	switch dbType {
	case "postgres":
		adapter, err = NewPostgresAdapter(ctx, dsn)
	case "mysql":
		adapter, err = NewMysqlAdapter(dsn)
	case "sqlite":
		adapter, err = NewSqliteAdapter(dsn)
	default:
		return fmt.Errorf("unsupported database type: %s", dbType)
	}

	if err != nil {
		return err
	}

	m.clients[id] = adapter
	log.Printf("Dynamically registered %s connection: %s", dbType, id)
	return nil
}

// GetClient retrieves a database client by its connection ID.
func (m *ConnectionManager) GetClient(connectionID string) (DatabaseClient, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	client, exists := m.clients[connectionID]
	if !exists {
		return nil, fmt.Errorf("connection ID '%s' not found", connectionID)
	}
	return client, nil
}

// GetAvailableConnections returns a list of configured connection IDs.
func (m *ConnectionManager) GetAvailableConnections() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var connections []string
	for id := range m.clients {
		connections = append(connections, id)
	}
	return connections
}
