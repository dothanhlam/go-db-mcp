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

- **Multi-Database Support**: Connect to multiple databases (PostgreSQL, MySQL, SQLite) simultaneously.
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
- Access to a PostgreSQL, MySQL, or SQLite database.

### 2. Build the Server
```bash
go mod tidy
go build -o go-db-mcp main.go
```

### 3. Running the Server
The server runs as an HTTP server using the Server-Sent Events (SSE) transport by default on port `6969`.

```bash
./go-db-mcp
```

You can override the port using the `PORT` or `MCP_PORT` environment variables:
```bash
PORT=8080 ./go-db-mcp
```

The endpoints are:
- **SSE Connection**: `GET http://localhost:6969/sse`
- **Messages**: `POST http://localhost:6969/messages`

### 4. Docker Deployment

The project can be run as a Docker container. The provided `Dockerfile` uses a multi-stage build to keep the image as small as possible.

**Important Note on System Tray:**
The application natively uses `github.com/energye/systray` to display an icon in your operating system's menu bar (e.g., the Mac topbar). Docker containers run in isolated Linux environments without access to your host OS's native GUI APIs. To prevent the application from crashing in Docker, it is configured to run headlessly within a virtual framebuffer (`xvfb-run`) and a mock D-Bus session.
**As a result, the tray icon will NOT be visible on your host machine when running via Docker.** If you require the tray icon to be visible, please run the application natively on your host machine.

**Build and Run:**
```bash
docker build -t go-db-mcp .
docker run -d -p 6969:6969 --name go-db-mcp go-db-mcp
```

### 5. Integration with Cursor / Antigravity
To configure this server, add it to your MCP settings using the **SSE** transport type.

#### SSE Configuration (Recommended)
Add a new MCP server with the type `SSE` and the following URL:
```text
http://localhost:6969/sse
```

In your configuration JSON, it should look like this:
```json
{
  "mcpServers": {
    "go-db-mcp": {
      "type": "sse",
      "url": "http://localhost:6969/sse"
    }
  }
}
```

#### Database Configuration
This server does NOT use environment variables for database connections anymore. Instead, you configure your databases dynamically via the MCP interface using the `configure_connection` tool.

**Benefits:**
- No need to manage secrets in your shell or `.env` files.
- Add or switch databases at runtime without restarting the server.
- The AI can automatically set up the connection if you provide the DSN in the chat.

**How to configure:**
Ask the AI (Cursor/Antigravity) to:
> "Configure a new postgres connection named 'my_db' with DSN 'postgres://user:pass@localhost:5432/dbname'"

The AI will use the `configure_connection` tool:
- `connection_id`: `my_db`
- `db_type`: `postgres`
- `dsn`: `postgres://user:pass@localhost:5432/dbname`

---
*Built using the [mark3labs/mcp-go](https://github.com/mark3labs/mcp-go) SDK.*
