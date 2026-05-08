# go-db-mcp

A Multi-Database Model Context Protocol (MCP) Server built in Golang. This server implements the [Model Context Protocol](https://modelcontextprotocol.io/) to expose database introspection and safe querying capabilities over STDIO, allowing AI agents and editors like Cursor to seamlessly interact with multiple databases.

## Security Notice

> [!WARNING]
> **This tool is designed for LOCAL usage only.**
> While it provides basic protections (like filtering destructive keywords and appending limits), it bypasses robust database security mechanisms (like proper Role-Based Access Control or strict SQL parsing). Exposing this tool over a public network or running it against production databases without strict isolation is highly discouraged.

## Architecture

The project is structured with a strong focus on modularity and extensibility:

- **`database/client.go`**: Defines the `DatabaseClient` interface, ensuring a unified contract (`ListTables`, `GetSchema`, `RunReadonlyQuery`) for any database engine.
- **Adapters (`postgres_adapter.go`, `mysql_adapter.go`)**: Concrete implementations of `DatabaseClient` for PostgreSQL and MySQL. They handle engine-specific logic like querying `information_schema` vs. `DESCRIBE`.
- **`database/manager.go`**: The `ConnectionManager` acts as a factory and registry, reading configurations from the environment and initializing available adapters under logical connection IDs (e.g., `pg_main`, `mysql_legacy`).
- **`tools/`**: Registers MCP tools. Every tool expects a `connection_id` to route requests to the correct database adapter via the Connection Manager. 
- **`main.go`**: The entry point. Initializes the manager and tools, and starts the MCP server listening on STDIO.

## Features

- **Multi-Database Support**: Connect to multiple databases (PostgreSQL, MySQL) simultaneously.
- **Dynamic Routing**: Route queries to specific databases using connection IDs.
- **Safe Read-Only Querying**: The `run_readonly_query` tool strictly filters out destructive SQL keywords (`DROP`, `DELETE`, `UPDATE`, `INSERT`, `TRUNCATE`, `ALTER`) and automatically enforces a `LIMIT 50` on all queries to prevent memory overload.

## Available MCP Tools

| Tool Name | Arguments | Description |
| :--- | :--- | :--- |
| `list_connections` | (none) | Get a list of available database connection IDs configured in the server. |
| `list_tables` | `connection_id` | Get a list of all tables in the specified database connection. |
| `get_schema` | `connection_id`, `table_name` | Extract schema metadata (columns, types, nullability) for a specific table. |
| `run_readonly_query` | `connection_id`, `sql_query` | Execute a read-only SELECT query safely. |

## Usage

### 1. Prerequisites
- Go 1.21 or later
- Access to a PostgreSQL and/or MySQL database.

### 2. Build the Server
```bash
go mod tidy
go build -o go-db-mcp main.go
```

### 3. Running the Server (Standalone)
Because this is an MCP server running over STDIO, running it directly in a standard terminal will just wait for JSON-RPC messages. 
```bash
./go-db-mcp
```

### 4. Integration with Cursor / Antigravity
To configure this server in Cursor or Antigravity, add it to your MCP settings. You can pass the database credentials directly in the `env` block:

```json
{
  "mcpServers": {
    "go-db-mcp": {
      "command": "/absolute/path/to/go-db-mcp/go-db-mcp",
      "env": {
        "POSTGRES_DSN": "postgres://user:password@localhost:5432/dbname?sslmode=disable",
        "MYSQL_DSN": "user:password@tcp(localhost:3306)/dbname"
      }
    }
  }
}
```
*Note: Make sure the `command` points to the absolute path of the built binary.*

---
*Built using the [mark3labs/mcp-go](https://github.com/mark3labs/mcp-go) SDK.*
