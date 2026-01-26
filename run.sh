#!/bin/bash
set -euo pipefail

# Get the directory where this script is located
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Load .env if it exists
if [[ -f "$SCRIPT_DIR/.env" ]]; then
    set -a
    source "$SCRIPT_DIR/.env"
    set +a
fi

# Validate required variables
missing=()
[[ -z "${MCP_MYSQL_HOST:-}" ]] && missing+=("MCP_MYSQL_HOST")
[[ -z "${MCP_MYSQL_PORT:-}" ]] && missing+=("MCP_MYSQL_PORT")
[[ -z "${MCP_MYSQL_DB:-}" ]] && missing+=("MCP_MYSQL_DB")
[[ -z "${MCP_MYSQL_USER:-}" ]] && missing+=("MCP_MYSQL_USER")
[[ -z "${MCP_MYSQL_PASSWORD:-}" ]] && missing+=("MCP_MYSQL_PASSWORD")

if [[ ${#missing[@]} -gt 0 ]]; then
    echo "Error: Missing required environment variables:" >&2
    printf '  %s\n' "${missing[@]}" >&2
    echo "" >&2
    echo "Set them in .env or export them. See example.env for reference." >&2
    exit 1
fi

# Build DSN from components
MYSQL_DSN="${MCP_MYSQL_USER}:${MCP_MYSQL_PASSWORD}@tcp(${MCP_MYSQL_HOST}:${MCP_MYSQL_PORT})/${MCP_MYSQL_DB}"

BINARY="${SCRIPT_DIR}/mysql-mcp-server"

# Build if binary doesn't exist or source is newer
if [[ ! -f "$BINARY" ]] || [[ $(find "$SCRIPT_DIR" -name "*.go" -newer "$BINARY" 2>/dev/null | head -1) ]]; then
    echo "Building..." >&2
    (cd "$SCRIPT_DIR" && CGO_ENABLED=0 go build -o mysql-mcp-server .)
fi

exec "$BINARY" "$MYSQL_DSN"
