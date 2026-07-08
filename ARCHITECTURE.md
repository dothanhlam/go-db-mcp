# Architecture

This document describes how `go-db-mcp` is put together: the layers, the request
flow, the safety model, and the decisions worth knowing before changing the code.

## Overview

`go-db-mcp` is a single Go binary that exposes a set of [MCP](https://modelcontextprotocol.io/)
tools over HTTP. An MCP client (Cursor, another editor, or any MCP-capable agent)
calls those tools; each tool routes to one of several database **adapters** through
a central **connection manager**. Databases are registered at runtime rather than
from configuration, so the same running server can talk to many databases and add
new ones mid-session.

```
            ┌──────────────────────────────────────────────────────────┐
 MCP client │                      go-db-mcp (one process)             │
 (Cursor) ──┼─▶ HTTP transports ─▶ MCP server ─▶ tools ─▶ ConnectionManager
            │   /mcp  (Streamable)                 │            │
            │   /sse  (legacy)                     │            ▼
            │                                      │      DatabaseClient
            │                                      │      ┌───────────────┐
            │                                      └────▶ │ postgres      │─▶ PostgreSQL
            │                                             │ mysql         │─▶ MySQL
            │  (optional) system-tray icon                │ sqlite        │─▶ SQLite file
            │                                             │ mongo         │─▶ MongoDB
            │                                             └───────────────┘
            └──────────────────────────────────────────────────────────┘
```

## Layers

### 1. Entry point & process model — `main.go`

`main.go` decides *how* the process runs and then starts the HTTP server.

- **Native mode**: `systray.Run(onReady, onExit)` shows a menu-bar/tray icon
  (About + Quit). `onReady` launches the server in a background goroutine so the
  GUI event loop stays responsive.
- **Headless mode** (`HEADLESS=1`): the tray is skipped entirely and the server
  runs directly on the main goroutine. No display, GTK session, D-Bus, or Xvfb is
  required — this is what the Docker image uses.

Both paths call the same `runServer()`, which builds the MCP server, registers the
tools, resolves the listen address, and serves. The `ConnectionManager` is held in
a package-level variable so `onExit` can close every open connection on shutdown.

**Address resolution.** Port comes from `PORT`, then `MCP_PORT`, then `6969`. Host
comes from `HOST`; if unset it defaults to `127.0.0.1` natively (so the open-ended
`configure_connection` tool isn't network-reachable) and `0.0.0.0` in headless mode
(so a container's published port works).

### 2. Transport — dual HTTP servers on one port

The MCP SDK ([mark3labs/mcp-go](https://github.com/mark3labs/mcp-go)) provides two
HTTP transports, and we mount both on a single `http.ServeMux`:

| Path | Transport | Handler |
| :--- | :--- | :--- |
| `/mcp` | Streamable HTTP (current MCP spec) | `server.NewStreamableHTTPServer` |
| `/sse` | SSE stream (legacy) | `server.NewSSEServer` |
| `/message` | SSE message channel | same SSE server |

Serving both means modern clients (Cursor uses Streamable HTTP) and older SSE-only
clients both work without reconfiguration. Each SDK handler does exact-path matching
internally, so mounting them on distinct mux patterns is safe.

### 3. Tools — `tools/`

Each file registers one MCP tool and contains only *protocol glue*: read and
validate arguments, resolve the target adapter from the manager, call the adapter,
and shape the result. Business logic lives in the adapters, not here.

| File | Tool | Responsibility |
| :--- | :--- | :--- |
| `tools/config.go` | `configure_connection` | Register a `(connection_id, db_type, dsn)` with the manager. |
| `tools/list_tables.go` | `list_tables` | Return tables/collections for a connection. |
| `tools/get_schema.go` | `get_schema` | Return schema/field metadata for one table/collection. |
| `tools/readonly_query.go` | `run_readonly_query` | Pre-check the query, then run it read-only via the adapter. |

Every data tool takes a `connection_id` as its first argument — that's how a single
server multiplexes across many databases.

`list_connections` is a small informational tool registered inline in `main.go`.

### 4. Connection management — `database/manager.go`

`ConnectionManager` is a concurrency-safe registry mapping `connection_id → DatabaseClient`.

- **`AddConnection(ctx, id, dbType, dsn)`** is the factory: it switches on
  `dbType` to build the right adapter, and if the `id` already exists it **closes
  the previous client first** to avoid leaking pools/sockets.
- **`GetClient(id)`** resolves a client for the tools; unknown IDs return an error.
- **`GetAvailableConnections()`** lists registered IDs.
- **`CloseAll()`** closes everything, called on shutdown.

A `sync.RWMutex` guards the map: reads (query routing) take the read lock; mutations
(`AddConnection`, `CloseAll`) take the write lock.

### 5. Database adapters — `database/*_adapter.go`

The heart of the extensibility model. `database/client.go` defines the contract every
engine implements:

```go
type DatabaseClient interface {
    ListTables(ctx context.Context) ([]string, error)
    GetSchema(ctx context.Context, tableName string) (string, error)
    RunReadonlyQuery(ctx context.Context, query string) (string, error)
    Close() error
}
```

| Adapter | Driver | Notes |
| :--- | :--- | :--- |
| `postgres_adapter.go` | `jackc/pgx/v5` (pool) | `information_schema` for metadata; parameterized `$1` schema lookup. |
| `mysql_adapter.go` | `go-sql-driver/mysql` | `SHOW TABLES` / `DESCRIBE`; pool tuning; identifier allow-listing. |
| `sqlite_adapter.go` | `mattn/go-sqlite3` (CGO) | `sqlite_master` / `PRAGMA table_info`; single connection; read-only via a driver ConnectHook. |
| `mongo_adapter.go` | `mongo-driver` (v1) | Collections instead of tables; schema inferred by sampling documents; JSON find/aggregate instead of SQL. |

Adding a new engine is: implement `DatabaseClient`, add a `case` in
`AddConnection`. Nothing else changes — tools and transport are engine-agnostic.

## Request flow

A typical `run_readonly_query` call:

```
MCP client
   │  POST /mcp  { tool: "run_readonly_query", connection_id, sql_query }
   ▼
Streamable HTTP transport (mcp-go)
   ▼
run_readonly_query handler (tools/readonly_query.go)
   │  1. Require & read connection_id and sql_query
   │  2. isReadonlyQuery() pre-check (SQL keyword denylist; JSON passes through)
   │  3. manager.GetClient(connection_id)
   ▼
adapter.RunReadonlyQuery(ctx, query)
   │  4. Open a read-only transaction (SQL) / build a find|aggregate (Mongo)
   │  5. Execute; scan at most 50 rows; marshal to JSON
   ▼
Result string ──▶ tool result ──▶ transport ──▶ client
```

## Safety model

Read-only enforcement is **layered**, so no single bypass defeats it:

1. **Tool-level pre-check** (`isReadonlyQuery`): a fast denylist rejecting
   `DROP/DELETE/UPDATE/INSERT/TRUNCATE/ALTER/GRANT/REVOKE` in SQL. Inputs that look
   like JSON (MongoDB specs) skip this check and are validated by the Mongo adapter
   instead. This is a convenience gate, **not** the real guarantee.

2. **Engine-level enforcement** (the actual guarantee):
   - **Postgres**: query runs inside `BEGIN … READ ONLY`.
   - **MySQL**: `START TRANSACTION READ ONLY`.
   - **SQLite**: connections are opened through a registered driver whose
     `ConnectHook` runs `PRAGMA query_only = ON`.
   - **MongoDB**: only `find`/`aggregate` are ever issued, and the `$out`/`$merge`
     write stages are rejected before execution.

3. **Result cap**: every adapter stops after `MaxQueryRows` (50) rows while
   *scanning the cursor* — not by appending `LIMIT` to the SQL text — so it can't be
   evaded by comments, string literals, or stacked statements.

4. **Identifier safety** for `get_schema`, where the table name can't be a bound
   parameter (MySQL `DESCRIBE`, SQLite `PRAGMA`): the name is **allow-listed**
   against the real `ListTables()` output before being interpolated. Postgres uses a
   bound `$1` parameter directly.

5. **Network posture**: default bind is loopback; `configure_connection` accepts
   arbitrary DSNs, so exposure is deliberately limited (see the README security note).

## Concurrency

- The manager's `RWMutex` makes registration and lookup safe under concurrent tool
  calls.
- SQL adapters rely on their driver connection **pools** (`database/sql` /
  `pgxpool`); SQLite is pinned to a single connection (`SetMaxOpenConns(1)`) to avoid
  writer-lock contention.
- Per-request `context.Context` flows from the transport through the tools into the
  drivers, so cancellations and timeouts propagate.

## Testing

- `database/sqlite_adapter_test.go` exercises the real read-only enforcement, the
  row cap, identifier allow-listing, and empty-result shaping against a temp SQLite DB.
- `database/mongo_adapter_test.go` covers the aggregation write-stage rejection.
- `tools/readonly_query_test.go` covers the SQL denylist and JSON pass-through.
- `main_smoke_test.go` stands up the mux with `httptest` and performs a real
  Streamable HTTP `initialize` handshake against `/mcp`.

## Directory map

```
.
├── main.go                     # entry point, run modes, transport wiring
├── main_smoke_test.go          # /mcp handshake smoke test
├── database/
│   ├── client.go               # DatabaseClient interface + shared helpers (row cap, table validation)
│   ├── manager.go              # ConnectionManager registry/factory
│   ├── postgres_adapter.go     # PostgreSQL adapter
│   ├── mysql_adapter.go        # MySQL adapter
│   ├── sqlite_adapter.go       # SQLite adapter (read-only driver)
│   └── mongo_adapter.go        # MongoDB adapter
├── tools/
│   ├── config.go               # configure_connection
│   ├── list_tables.go          # list_tables
│   ├── get_schema.go           # get_schema
│   └── readonly_query.go       # run_readonly_query + read-only pre-check
├── icon/                       # embedded tray icon
├── Dockerfile                  # headless multi-stage build
└── README.md                   # user-facing guide
```
