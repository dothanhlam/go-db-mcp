# go-db-mcp

A Multi-Database Model Context Protocol (MCP) Server built in Golang. This server implements the [Model Context Protocol](https://modelcontextprotocol.io/) to expose database introspection and safe querying capabilities over an HTTP/SSE transport, allowing AI agents and editors like Cursor to seamlessly interact with multiple databases.

## Security Notice

> [!WARNING]
> **This tool is designed for LOCAL usage only.**
> Queries run inside database-enforced read-only transactions (`READ ONLY` on Postgres/MySQL, `PRAGMA query_only` on SQLite) and results are capped at 50 rows, but this is not a substitute for a properly scoped, least-privilege database user. The SSE server binds to `127.0.0.1` by default; because the `configure_connection` tool accepts arbitrary DSNs, do not expose the port publicly. Running against production databases without strict isolation is highly discouraged.

## Architecture

The project is structured with a strong focus on modularity and extensibility:

- **`database/client.go`**: Defines the `DatabaseClient` interface, ensuring a unified contract (`ListTables`, `GetSchema`, `RunReadonlyQuery`) for any database engine.
- **Adapters (`postgres_adapter.go`, `mysql_adapter.go`, `sqlite_adapter.go`, `mongo_adapter.go`)**: Concrete implementations of `DatabaseClient` for PostgreSQL, MySQL, SQLite, and MongoDB. They handle engine-specific logic (SQL `information_schema`/`DESCRIBE`/`PRAGMA`, or MongoDB collection listing and schema inference by sampling documents).
- **`database/manager.go`**: The `ConnectionManager` acts as a factory and registry. Connections are added at runtime via the `configure_connection` tool and stored under logical connection IDs (e.g., `pg_main`, `mysql_legacy`). Re-using an ID closes the previous connection.
- **`tools/`**: Registers MCP tools. Every tool expects a `connection_id` to route requests to the correct database adapter via the Connection Manager.
- **`main.go`**: The entry point. Initializes the manager and tools, and starts the MCP server over HTTP/SSE.

## Features

- **Multi-Database Support**: Connect to multiple databases (PostgreSQL, MySQL, SQLite, MongoDB) simultaneously.
- **Dynamic Routing**: Route queries to specific databases using connection IDs.
- **Safe Read-Only Querying**: The `run_readonly_query` tool runs every query inside a database-enforced read-only transaction, rejects a first-pass list of destructive keywords, and caps results at 50 rows (enforced while scanning, so it can't be bypassed by the query text).

## Available MCP Tools

| Tool Name | Arguments | Description |
| :--- | :--- | :--- |
| `configure_connection` | `connection_id`, `db_type`, `dsn` | Register a database connection at runtime (`db_type`: `postgres`, `mysql`, or `sqlite`). |
| `list_connections` | (none) | Get a list of available database connection IDs configured in the server. |
| `list_tables` | `connection_id` | Get a list of all tables in the specified database connection. |
| `get_schema` | `connection_id`, `table_name` | Extract schema metadata (columns, types, nullability) for a specific table. |
| `run_readonly_query` | `connection_id`, `sql_query` | Execute a read-only query safely (capped at 50 rows). SQL connections take a `SELECT`; MongoDB connections take a JSON find/aggregate spec (see below). |

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
The server runs as an HTTP server on port `6969` and exposes **both** MCP HTTP transports at once:

- **Streamable HTTP** (current spec, used by Cursor): `http://localhost:6969/mcp`
- **SSE** (legacy): `http://localhost:6969/sse`

```bash
./go-db-mcp
```

You can override the port using the `PORT` or `MCP_PORT` environment variables:
```bash
PORT=8080 ./go-db-mcp
```

By default the server binds to `127.0.0.1` (loopback only). To listen on all interfaces — for example inside a container — set `HOST`:
```bash
HOST=0.0.0.0 PORT=6969 ./go-db-mcp
```

The endpoints are:
- **Streamable HTTP**: `POST/GET http://localhost:6969/mcp`
- **SSE Connection**: `GET http://localhost:6969/sse` (message endpoint is advertised by the SSE stream at `/message`)

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

#### Streamable HTTP (Recommended)
Point your MCP client at the `/mcp` endpoint. In Cursor's `mcp.json`:
```json
{
  "mcpServers": {
    "go-db-mcp": {
      "url": "http://localhost:6969/mcp"
    }
  }
}
```

> [!NOTE]
> Use the full `/mcp` path, not the bare host. Configuring `http://localhost:6969`
> (no path) causes errors like *"Transient error connecting to streamableHttp
> server: fetch failed"* because the root path serves nothing.

If you run the server in Docker, make sure the container is reachable: the image
sets `HOST=0.0.0.0` and you publish the port with `-p 6969:6969`, so
`http://localhost:6969/mcp` on the host works.

#### SSE (Legacy)
Clients that only support the older SSE transport can use:
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

#### MongoDB

MongoDB is supported alongside the SQL engines. Configure it with a standard connection URI that **includes the database name**:
- `db_type`: `mongodb`
- `dsn`: `mongodb://user:pass@localhost:27017/mydb`

Because MongoDB is not SQL, the tools map as follows:
- `list_tables` — lists collections.
- `get_schema` — samples up to 100 documents and reports each field's observed BSON types (Mongo is schemaless, so this is inferred).
- `run_readonly_query` — the `sql_query` argument takes a **JSON find/aggregate spec** instead of SQL. Only reads run (writes and the `$out`/`$merge` aggregation stages are rejected), and results are capped at 50 documents.

**Find example:**
```json
{
  "collection": "users",
  "filter": { "age": { "$gt": 21 } },
  "projection": { "name": 1, "email": 1 },
  "sort": { "name": 1 },
  "limit": 20
}
```

**Aggregation example:**
```json
{
  "collection": "orders",
  "pipeline": [
    { "$match": { "status": "paid" } },
    { "$group": { "_id": "$country", "total": { "$sum": "$amount" } } }
  ]
}
```

---
*Built using the [mark3labs/mcp-go](https://github.com/mark3labs/mcp-go) SDK.*
