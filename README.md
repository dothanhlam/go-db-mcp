# go-db-mcp

A multi-database [Model Context Protocol (MCP)](https://modelcontextprotocol.io/) server written in Go. It lets AI agents and editors like Cursor introspect and safely query **PostgreSQL, MySQL, SQLite, and MongoDB** — all through one server, over HTTP.

Connections are configured **at runtime through the chat** (via the `configure_connection` tool), so there are no secrets to bake into config files. The server speaks both the current **Streamable HTTP** transport and the legacy **SSE** transport on the same port.

> For a deep dive on how the pieces fit together, see [ARCHITECTURE.md](ARCHITECTURE.md).

---

## Security Notice

> [!WARNING]
> **This tool is intended for local / trusted-network use.**
>
> - Queries run inside **database-enforced read-only transactions** (`READ ONLY` on Postgres/MySQL, `PRAGMA query_only` on SQLite, find/aggregate-only on MongoDB) and results are **capped at 50 rows**.
> - This is **not** a substitute for a properly scoped, least-privilege database user — use one for anything sensitive.
> - The `configure_connection` tool accepts **arbitrary DSNs**, so anyone who can reach the port can make the server connect to any database. When running natively the server binds **`127.0.0.1`** by default. Do **not** expose the port publicly (in Docker it binds `0.0.0.0` inside the container — keep the published port on a trusted host/network).
> - Running against production databases without strict isolation is strongly discouraged.

---

## Features

- **Multi-database**: connect to PostgreSQL, MySQL, SQLite, and MongoDB simultaneously.
- **Runtime configuration**: add or switch connections mid-session via chat — no restart, no `.env`.
- **Dynamic routing**: every tool takes a `connection_id` to target a specific database.
- **Safe read-only querying**: DB-enforced read-only transactions, a destructive-keyword pre-check, and a 50-row cap enforced while scanning (so it can't be bypassed by the query text).
- **Dual transport**: Streamable HTTP (`/mcp`) and SSE (`/sse`) served together.
- **Runs anywhere**: native desktop build with a system-tray icon, or fully headless in Docker.

---

## Available MCP Tools

| Tool | Arguments | Description |
| :--- | :--- | :--- |
| `configure_connection` | `connection_id`, `db_type`, `dsn` | Register a connection at runtime. `db_type`: `postgres`, `mysql`, `sqlite`, or `mongodb`. |
| `list_connections` | (none) | List the connection IDs currently configured. |
| `list_tables` | `connection_id` | List all tables (SQL) or collections (MongoDB) for a connection. |
| `get_schema` | `connection_id`, `table_name` | Return column metadata (SQL) or inferred field types (MongoDB) for a table/collection. |
| `run_readonly_query` | `connection_id`, `sql_query` | Run a read-only query (capped at 50 rows). SQL takes a `SELECT`; MongoDB takes a JSON find/aggregate spec — see [MongoDB](#mongodb). |

---

## Getting Started

### Prerequisites
- **Go 1.25+** (see `go.mod`).
- A build toolchain for CGO — SQLite uses `mattn/go-sqlite3` and the tray uses GTK. On macOS the Xcode command-line tools suffice; on Debian/Ubuntu install `gcc pkg-config libgtk-3-dev libayatana-appindicator3-dev`.
- Access to at least one PostgreSQL, MySQL, SQLite, or MongoDB database.

### Build
```bash
go mod tidy
go build -o go-db-mcp .
```

### Run
The server listens on port `6969` and exposes both transports:

- **Streamable HTTP** (current spec, used by Cursor): `http://localhost:6969/mcp`
- **SSE** (legacy): `http://localhost:6969/sse`

```bash
./go-db-mcp
```

When run natively it also shows a **system-tray icon** (menu-bar on macOS) with About/Quit.

### Configuration (environment variables)

| Variable | Default | Purpose |
| :--- | :--- | :--- |
| `PORT` / `MCP_PORT` | `6969` | Port to listen on (`PORT` takes precedence). |
| `HOST` | `127.0.0.1` (native) / `0.0.0.0` (headless) | Interface to bind. Set `HOST=0.0.0.0` to accept non-loopback connections. |
| `HEADLESS` | unset | Set to `1`/`true` to run without the system tray (no display/GTK/D-Bus needed). Also defaults `HOST` to `0.0.0.0`. |

Examples:
```bash
PORT=8080 ./go-db-mcp            # different port
HOST=0.0.0.0 ./go-db-mcp         # listen on all interfaces
HEADLESS=1 ./go-db-mcp           # server only, no tray
```

Endpoints:
- **Streamable HTTP**: `POST`/`GET http://localhost:6969/mcp`
- **SSE**: `GET http://localhost:6969/sse` (the message endpoint is advertised by the SSE stream at `/message`)

---

## Docker

The `Dockerfile` uses a multi-stage build and runs **headless** (`HEADLESS=1`) — no display, D-Bus, or Xvfb required. Headless mode binds `0.0.0.0` inside the container so the published port is reachable from the host.

```bash
docker build -t go-db-mcp .
docker run -d -p 6969:6969 --name go-db-mcp go-db-mcp

# verify
docker logs go-db-mcp        # expect: "Running in headless mode" + "Starting MCP server on 0.0.0.0:6969"
```

Then point your MCP client at `http://localhost:6969/mcp`.

---

## Connecting from Cursor / Antigravity

### Streamable HTTP (recommended)
Add the server to your MCP config (Cursor's `mcp.json`):
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
> Use the full `/mcp` path, **not** the bare host. Configuring `http://localhost:6969`
> (no path) produces errors like *"Transient error connecting to streamableHttp
> server: fetch failed"* because the root path serves nothing.

### SSE (legacy)
For clients that only support SSE:
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

---

## Configuring Database Connections

Connections are **not** read from environment variables. Instead you create them at runtime with the `configure_connection` tool — which the AI will call for you when you describe the database in chat:

> "Configure a new postgres connection named `my_db` with DSN `postgres://user:pass@localhost:5432/dbname`"

The AI invokes `configure_connection` with:
- `connection_id`: `my_db`
- `db_type`: `postgres`
- `dsn`: `postgres://user:pass@localhost:5432/dbname`

After that, reference the database by its `connection_id` in the other tools ("list the tables in `my_db`", "show the schema of the `orders` table in `my_db`", …). Re-using an existing `connection_id` closes the old connection and replaces it.

### DSN formats by engine

| `db_type` | Example DSN |
| :--- | :--- |
| `postgres` | `postgres://user:pass@localhost:5432/dbname?sslmode=disable` |
| `mysql` | `user:pass@tcp(localhost:3306)/dbname` |
| `sqlite` | `/absolute/path/to/database.db` |
| `mongodb` | `mongodb://user:pass@localhost:27017/mydb` (must include the database name) |

### MongoDB

MongoDB is not SQL, so the tools map to Mongo concepts:

- **`list_tables`** → lists **collections**.
- **`get_schema`** → samples up to 100 documents and reports each field's observed **BSON types** (MongoDB is schemaless, so the schema is inferred).
- **`run_readonly_query`** → the `sql_query` argument takes a **JSON find/aggregate spec** instead of SQL. It's read-only by construction: only `find`/`aggregate` execute, the `$out`/`$merge` write stages are rejected, and results are capped at 50 documents.

**Find** (`collection` plus optional `filter`, `projection`, `sort`, `limit`):
```json
{
  "collection": "users",
  "filter": { "age": { "$gt": 21 } },
  "projection": { "name": 1, "email": 1 },
  "sort": { "name": 1 },
  "limit": 20
}
```

**Aggregation** (`collection` plus a `pipeline`):
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

## Development

```bash
go build ./...     # compile everything
go vet ./...       # static checks
go test ./...      # run the test suite
```

---

*Built with the [mark3labs/mcp-go](https://github.com/mark3labs/mcp-go) SDK.*
