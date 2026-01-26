# go-readonly-mcp-mysql

[![Docker Pulls](https://img.shields.io/docker/pulls/shakram02/go-readonly-mcp-mysql)](https://hub.docker.com/r/shakram02/go-readonly-mcp-mysql)
[![Docker Image Size](https://img.shields.io/docker/image-size/shakram02/go-readonly-mcp-mysql/latest)](https://hub.docker.com/r/shakram02/go-readonly-mcp-mysql)
[![License](https://img.shields.io/badge/license-MIT-blue)](LICENSE)

![go-readonly-mcp-mysql](assets/Golang%20Mascott%20-%20Post%20MCP.png)

A read-only MySQL MCP (Model Context Protocol) server written in Go. Allows AI assistants like Claude to safely query MySQL databases without risk of data modification.

## Features

- **Read-only by design** - Only SELECT, SHOW, DESCRIBE, and EXPLAIN queries allowed
- **Security hardened** - Blocks dangerous functions (SLEEP, BENCHMARK, LOAD_FILE, etc.)
- **Single binary** - No runtime dependencies, easy to deploy
- **Cross-platform** - Builds for Linux, macOS, and Windows
- **Low resource usage** - ~5MB binary, minimal memory footprint

## Installation

### Download Binary

Download the latest release for your platform from [Releases](https://github.com/shakram02/go-readonly-mcp-mysql/releases).

### Build from Source

```bash
go install github.com/shakram02/go-readonly-mcp-mysql@latest
```

Or clone and build:

```bash
git clone https://github.com/shakram02/go-readonly-mcp-mysql.git
cd go-readonly-mcp-mysql
CGO_ENABLED=0 go build -o mysql-mcp-server .
```

### Docker

```bash
docker build -t mysql-mcp-server .
```

## Usage

### Using Environment Variables

Set the following environment variables:

| Variable | Description | Example |
|----------|-------------|---------|
| `MCP_MYSQL_HOST` | Database host | `localhost` |
| `MCP_MYSQL_PORT` | Database port | `3306` |
| `MCP_MYSQL_DB` | Database name | `mydb` |
| `MCP_MYSQL_USER` | Database user | `readonly` |
| `MCP_MYSQL_PASSWORD` | Database password | `secret` |

Then run:

```bash
mysql-mcp-server
```

### Using DSN Argument

```bash
mysql-mcp-server 'user:password@tcp(localhost:3306)/database'
```

The DSN follows the [go-sql-driver/mysql](https://github.com/go-sql-driver/mysql#dsn-data-source-name) format:

```
user:password@tcp(host:port)/dbname?param=value
```

## Claude Code Setup

Using environment variables:

```json
{
  "mcpServers": {
    "go-readonly-mcp-mysql": {
      "command": "/path/to/mysql-mcp-server",
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

Using DSN directly:

```json
{
  "mcpServers": {
    "go-readonly-mcp-mysql": {
      "command": "/path/to/mysql-mcp-server",
      "args": ["user:password@tcp(localhost:3306)/mydb"]
    }
  }
}
```

### Docker Setup

**Linux (recommended):**

```json
{
  "mcpServers": {
    "go-readonly-mcp-mysql": {
      "command": "docker",
      "args": [
        "run", "-i", "--rm",
        "--network=host",
        "-e", "MCP_MYSQL_HOST=127.0.0.1",
        "-e", "MCP_MYSQL_PORT=3306",
        "-e", "MCP_MYSQL_DB=mydb",
        "-e", "MCP_MYSQL_USER=readonly",
        "-e", "MCP_MYSQL_PASSWORD=secret",
        "shakram02/go-readonly-mcp-mysql"
      ]
    }
  }
}
```

**macOS/Windows:**

```json
{
  "mcpServers": {
    "go-readonly-mcp-mysql": {
      "command": "docker",
      "args": [
        "run", "-i", "--rm",
        "-e", "MCP_MYSQL_HOST=host.docker.internal",
        "-e", "MCP_MYSQL_PORT=3306",
        "-e", "MCP_MYSQL_DB=mydb",
        "-e", "MCP_MYSQL_USER=readonly",
        "-e", "MCP_MYSQL_PASSWORD=secret",
        "shakram02/go-readonly-mcp-mysql"
      ]
    }
  }
}
```

> **Note:** On Linux, `--network=host` lets the container access localhost directly. On macOS/Windows, use `host.docker.internal` instead. For remote databases, use the actual hostname or IP.

## MCP Tools

### query

Execute a read-only SQL query.

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

- **URI format:** `mysql://database/table/schema`
- **Content:** JSON array of column definitions

## Security

### Query Validation

All queries are validated before execution:

- Must start with allowed keywords (SELECT, SHOW, DESCRIBE, EXPLAIN)
- Blocked keywords: INSERT, UPDATE, DELETE, DROP, CREATE, ALTER, TRUNCATE, etc.
- Blocked patterns: INTO OUTFILE, INTO DUMPFILE, LOAD_FILE
- Blocked functions: SLEEP, BENCHMARK, GET_LOCK (DoS prevention)
- Multi-statement queries are rejected

### Connection Security

- Session is set to `TRANSACTION READ ONLY` mode
- Query timeout: 30 seconds
- Result limit: 10,000 rows

### Recommendations

- Use a dedicated read-only MySQL user
- Restrict the user to specific databases/tables
- Use TLS for remote connections

Example MySQL user setup:
```sql
CREATE USER 'mcp_readonly'@'%' IDENTIFIED BY 'secure_password';
GRANT SELECT ON mydb.* TO 'mcp_readonly'@'%';
FLUSH PRIVILEGES;
```

## Building

### All Platforms

```bash
# Linux
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o dist/mysql-mcp-server-linux-amd64 .

# macOS (Intel)
GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build -o dist/mysql-mcp-server-darwin-amd64 .

# macOS (Apple Silicon)
GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -o dist/mysql-mcp-server-darwin-arm64 .

# Windows
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o dist/mysql-mcp-server-windows-amd64.exe .
```

### Running Tests

```bash
go test -v ./...
```

## License

MIT
