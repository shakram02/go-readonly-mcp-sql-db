# go-readonly-mcp

[![Docker Pulls](https://img.shields.io/docker/pulls/shakram02/readonly-sql-db-mcp)](https://hub.docker.com/r/shakram02/readonly-sql-db-mcp)
[![Docker Image Size](https://img.shields.io/docker/image-size/shakram02/readonly-sql-db-mcp/latest)](https://hub.docker.com/r/shakram02/readonly-sql-db-mcp)
[![License](https://img.shields.io/badge/license-MIT-blue)](LICENSE)

![go-readonly-mcp](assets/Golang%20Mascott%20-%20Post%20MCP.png)

A read-only database MCP (Model Context Protocol) server written in Go. Allows AI assistants like Claude to safely query **MySQL**, **PostgreSQL**, and **SQLite** databases without risk of data modification.

## Features

- **Multi-database support** - MySQL, PostgreSQL, and SQLite via a single binary
- **Read-only by design** - Only SELECT, SHOW, DESCRIBE, and EXPLAIN queries allowed
- **Security hardened** - Per-database blocking of dangerous functions and patterns
- **Single binary** - No runtime dependencies (pure Go SQLite, no CGO required)
- **Cross-platform** - Builds for Linux, macOS, and Windows
- **Low resource usage** - Minimal memory footprint
- **Backward compatible** - Defaults to MySQL when no driver is specified

## Supported Databases

| Database   | Driver                  | `MCP_DB_DRIVER` value   |
|------------|-------------------------|-------------------------|
| MySQL      | go-sql-driver/mysql     | `mysql` (default)       |
| PostgreSQL | lib/pq                  | `postgres`              |
| SQLite     | modernc.org/sqlite      | `sqlite`                |

## Installation

### Download Binary

Download the latest release for your platform from [Releases](https://github.com/shakram02/readonly-sql-db-mcp/releases).

### Build from Source

```bash
go install github.com/shakram02/readonly-sql-db-mcp@latest
```

Or clone and build:

```bash
git clone https://github.com/shakram02/readonly-sql-db-mcp.git
cd go-readonly-mcp-mysql
CGO_ENABLED=0 go build -o readonly-mcp-server .
```

### Docker

```bash
docker build -t readonly-mcp-server .
```

## Usage

### Selecting a Database Driver

Set `MCP_DB_DRIVER` to choose your database. If not set, defaults to `mysql`.

```bash
export MCP_DB_DRIVER=mysql    # or postgres, sqlite
```

### Query Limits

These optional env vars override the defaults for all database drivers:

| Variable | Description | Default |
|----------|-------------|---------|
| `MCP_QUERY_TIMEOUT` | Query timeout in seconds | `30` |
| `MCP_MAX_ROWS` | Maximum rows returned per query | `10000` |

### MySQL

#### Environment Variables

| Variable | Description | Example |
|----------|-------------|---------|
| `MCP_DB_DRIVER` | Database driver | `mysql` (default) |
| `MCP_MYSQL_HOST` | Database host | `localhost` |
| `MCP_MYSQL_PORT` | Database port | `3306` |
| `MCP_MYSQL_DB` | Database name | `mydb` |
| `MCP_MYSQL_USER` | Database user | `readonly` |
| `MCP_MYSQL_PASSWORD` | Database password | `secret` |

```bash
readonly-mcp-server
```

#### DSN Argument

```bash
readonly-mcp-server 'user:password@tcp(localhost:3306)/database'
```

The DSN follows the [go-sql-driver/mysql](https://github.com/go-sql-driver/mysql#dsn-data-source-name) format.

### PostgreSQL

#### Environment Variables

| Variable | Description | Example |
|----------|-------------|---------|
| `MCP_DB_DRIVER` | Database driver | `postgres` |
| `MCP_PG_HOST` | Database host | `localhost` |
| `MCP_PG_PORT` | Database port | `5432` |
| `MCP_PG_DB` | Database name | `mydb` |
| `MCP_PG_USER` | Database user | `readonly` |
| `MCP_PG_PASSWORD` | Database password | `secret` |
| `MCP_PG_SSLMODE` | SSL mode | `prefer` (default) |

```bash
MCP_DB_DRIVER=postgres readonly-mcp-server
```

#### DSN Argument

```bash
MCP_DB_DRIVER=postgres readonly-mcp-server 'postgres://user:password@localhost:5432/mydb?sslmode=prefer'
```

### SQLite

#### Environment Variables

| Variable | Description | Example |
|----------|-------------|---------|
| `MCP_DB_DRIVER` | Database driver | `sqlite` |
| `MCP_SQLITE_PATH` | Path to SQLite database file | `/data/mydb.db` |

```bash
MCP_DB_DRIVER=sqlite readonly-mcp-server
```

#### DSN Argument

```bash
MCP_DB_DRIVER=sqlite readonly-mcp-server '/data/mydb.db'
```

> **Note:** SQLite connections are opened in read-only mode (`?mode=ro`) and additionally set `PRAGMA query_only = ON` as defense-in-depth.

## Claude Code Setup

### MySQL

```json
{
  "mcpServers": {
    "readonly-db": {
      "command": "/path/to/readonly-mcp-server",
      "env": {
        "MCP_MYSQL_HOST": "localhost",
        "MCP_MYSQL_PORT": "3306",
        "MCP_MYSQL_DB": "mydb",
        "MCP_MYSQL_USER": "readonly",
        "MCP_MYSQL_PASSWORD": "secret"
      }
    }
  }
}
```

### PostgreSQL

```json
{
  "mcpServers": {
    "readonly-db": {
      "command": "/path/to/readonly-mcp-server",
      "env": {
        "MCP_DB_DRIVER": "postgres",
        "MCP_PG_HOST": "localhost",
        "MCP_PG_PORT": "5432",
        "MCP_PG_DB": "mydb",
        "MCP_PG_USER": "readonly",
        "MCP_PG_PASSWORD": "secret"
      }
    }
  }
}
```

### SQLite

```json
{
  "mcpServers": {
    "readonly-db": {
      "command": "/path/to/readonly-mcp-server",
      "env": {
        "MCP_DB_DRIVER": "sqlite",
        "MCP_SQLITE_PATH": "/data/mydb.db"
      }
    }
  }
}
```

### Docker Setup

**Linux (recommended):**

```json
{
  "mcpServers": {
    "readonly-db": {
      "command": "docker",
      "args": [
        "run", "-i", "--rm",
        "--network=host",
        "-e", "MCP_MYSQL_HOST=127.0.0.1",
        "-e", "MCP_MYSQL_PORT=3306",
        "-e", "MCP_MYSQL_DB=mydb",
        "-e", "MCP_MYSQL_USER=readonly",
        "-e", "MCP_MYSQL_PASSWORD=secret",
        "shakram02/readonly-sql-db-mcp"
      ]
    }
  }
}
```

**macOS/Windows:**

```json
{
  "mcpServers": {
    "readonly-db": {
      "command": "docker",
      "args": [
        "run", "-i", "--rm",
        "-e", "MCP_MYSQL_HOST=host.docker.internal",
        "-e", "MCP_MYSQL_PORT=3306",
        "-e", "MCP_MYSQL_DB=mydb",
        "-e", "MCP_MYSQL_USER=readonly",
        "-e", "MCP_MYSQL_PASSWORD=secret",
        "shakram02/readonly-sql-db-mcp"
      ]
    }
  }
}
```

> **Note:** On Linux, `--network=host` lets the container access localhost directly. On macOS/Windows, use `host.docker.internal` instead. For remote databases, use the actual hostname or IP.

**Docker with PostgreSQL:**

Add `-e MCP_DB_DRIVER=postgres` and replace the MySQL env vars with `MCP_PG_*` equivalents.

**Docker with SQLite:**

Mount the database file and set the driver:

```json
{
  "mcpServers": {
    "readonly-db": {
      "command": "docker",
      "args": [
        "run", "-i", "--rm",
        "-v", "/path/to/mydb.db:/data/mydb.db:ro",
        "-e", "MCP_DB_DRIVER=sqlite",
        "-e", "MCP_SQLITE_PATH=/data/mydb.db",
        "shakram02/readonly-sql-db-mcp"
      ]
    }
  }
}
```

## MCP Tools

### query

Execute a read-only SQL query in the native dialect of the configured database.

**Parameters:**
- `sql` (string, required): The SQL query to execute

**Allowed statements:**
- `SELECT`
- `SHOW`
- `DESCRIBE` / `DESC`
- `EXPLAIN`

**Example:**
```json
{
  "name": "query",
  "arguments": {
    "sql": "SELECT * FROM users LIMIT 10"
  }
}
```

## MCP Resources

The server exposes table schemas as resources:

- **URI format:** `<driver>://database/table/schema` (e.g., `mysql://mydb/users/schema`, `postgres://mydb/users/schema`, `sqlite://mydb/users/schema`)
- **Content:** JSON array of column definitions

## Security

### Query Validation

All queries are validated before execution. A shared validation layer blocks common dangerous operations, and each database adapter adds its own specific checks.

**Common (all databases):**
- Must start with allowed keywords (SELECT, SHOW, DESCRIBE, EXPLAIN)
- Blocked keywords: INSERT, UPDATE, DELETE, DROP, CREATE, ALTER, TRUNCATE, GRANT, REVOKE, SET
- Multi-statement queries are rejected

**MySQL-specific:**
- Blocked patterns: INTO OUTFILE, INTO DUMPFILE, LOAD_FILE, INTO @variable
- Blocked DoS functions: SLEEP, BENCHMARK, GET_LOCK, RELEASE_LOCK, IS_FREE_LOCK, IS_USED_LOCK
- Blocked keywords: CALL, EXEC, EXECUTE, REPLACE, LOAD, HANDLER, RENAME

**PostgreSQL-specific:**
- Blocked patterns: COPY TO/FROM, pg_read_file, pg_read_binary_file, pg_ls_dir, lo_import, lo_export
- Blocked DoS functions: pg_sleep, pg_sleep_for, pg_sleep_until, pg_advisory_lock, pg_advisory_xact_lock
- Blocked keywords: CALL, EXECUTE, COPY, LISTEN, NOTIFY, PREPARE, DEALLOCATE, VACUUM, REINDEX, CLUSTER

**SQLite-specific:**
- Blocked functions: load_extension, writefile, edit, fts3_tokenizer
- Blocked keywords: REPLACE, ATTACH, DETACH, REINDEX, VACUUM
- PRAGMA writes blocked (e.g., `PRAGMA journal_mode = WAL`), read-only PRAGMAs allowed

### Connection Security

| Database   | Read-only enforcement                                        |
|------------|--------------------------------------------------------------|
| MySQL      | `SET SESSION TRANSACTION READ ONLY`                          |
| PostgreSQL | `SET SESSION CHARACTERISTICS AS TRANSACTION READ ONLY`       |
| SQLite     | DSN `?mode=ro` + `PRAGMA query_only = ON` (defense-in-depth) |

- Query timeout: 30 seconds (configurable via `MCP_QUERY_TIMEOUT`)
- Result limit: 10,000 rows (configurable via `MCP_MAX_ROWS`)

### Recommendations

- Use a dedicated read-only database user
- Restrict the user to specific databases/tables
- Use TLS for remote connections

**MySQL user setup:**
```sql
CREATE USER 'mcp_readonly'@'%' IDENTIFIED BY 'secure_password';
GRANT SELECT ON mydb.* TO 'mcp_readonly'@'%';
FLUSH PRIVILEGES;
```

**PostgreSQL user setup:**
```sql
CREATE USER mcp_readonly WITH PASSWORD 'secure_password';
GRANT CONNECT ON DATABASE mydb TO mcp_readonly;
GRANT USAGE ON SCHEMA public TO mcp_readonly;
GRANT SELECT ON ALL TABLES IN SCHEMA public TO mcp_readonly;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT ON TABLES TO mcp_readonly;
```

## Building

### All Platforms

```bash
# Linux
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o dist/readonly-mcp-server-linux-amd64 .

# macOS (Intel)
GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build -o dist/readonly-mcp-server-darwin-amd64 .

# macOS (Apple Silicon)
GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -o dist/readonly-mcp-server-darwin-arm64 .

# Windows
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o dist/readonly-mcp-server-windows-amd64.exe .
```

### Running Tests

```bash
go test -v ./...
```

## License

MIT
