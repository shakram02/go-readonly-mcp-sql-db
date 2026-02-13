package main

import (
	"context"
	"database/sql"
)

// DBAdapter defines the contract for database-specific behavior.
// Each supported database (MySQL, PostgreSQL, SQLite) implements this interface.
type DBAdapter interface {
	// DriverName returns the database/sql driver name (e.g., "mysql", "postgres", "sqlite").
	DriverName() string

	// ServerName returns the MCP server name for this adapter.
	ServerName() string

	// URIScheme returns the resource URI scheme (e.g., "mysql", "postgres", "sqlite").
	URIScheme() string

	// BuildDSN constructs a DSN from environment variables.
	BuildDSN() (string, error)

	// DatabaseName extracts the database/file name from a DSN string.
	DatabaseName(dsn string) string

	// EnforceReadOnly configures the database connection for read-only access.
	EnforceReadOnly(ctx context.Context, db *sql.DB) error

	// ListTablesQuery returns the SQL query and arguments to list all tables.
	ListTablesQuery(databaseName string) (string, []any)

	// ReadSchemaQuery returns the SQL query and arguments to read column info for a table.
	ReadSchemaQuery(databaseName, tableName string) (string, []any)

	// ScanSchemaRow scans a single row from the schema query result into a column map.
	ScanSchemaRow(rows *sql.Rows) (map[string]any, error)

	// ValidateQuery validates that a SQL query is safe and read-only.
	ValidateQuery(sql string) error

	// RemoveStringsAndComments strips string literals and comments from SQL
	// for safe keyword detection.
	RemoveStringsAndComments(sql string) string
}
