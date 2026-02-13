package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
)

// PostgresAdapter implements DBAdapter for PostgreSQL databases.
type PostgresAdapter struct{}

func (a *PostgresAdapter) DriverName() string { return "postgres" }
func (a *PostgresAdapter) ServerName() string { return "postgres-readonly-mcp-server" }
func (a *PostgresAdapter) URIScheme() string  { return "postgres" }

func (a *PostgresAdapter) BuildDSN() (string, error) {
	host := os.Getenv("MCP_PG_HOST")
	port := os.Getenv("MCP_PG_PORT")
	db := os.Getenv("MCP_PG_DB")
	user := os.Getenv("MCP_PG_USER")
	password := os.Getenv("MCP_PG_PASSWORD")
	sslmode := os.Getenv("MCP_PG_SSLMODE")
	if sslmode == "" {
		sslmode = "prefer"
	}

	var missing []string
	if host == "" {
		missing = append(missing, "MCP_PG_HOST")
	}
	if port == "" {
		missing = append(missing, "MCP_PG_PORT")
	}
	if db == "" {
		missing = append(missing, "MCP_PG_DB")
	}
	if user == "" {
		missing = append(missing, "MCP_PG_USER")
	}
	if password == "" {
		missing = append(missing, "MCP_PG_PASSWORD")
	}

	if len(missing) > 0 {
		return "", fmt.Errorf("missing required environment variables: %v", missing)
	}

	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s",
		url.PathEscape(user), url.PathEscape(password), host, port, db, sslmode), nil
}

func (a *PostgresAdapter) DatabaseName(dsn string) string {
	u, err := url.Parse(dsn)
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(u.Path, "/")
}

func (a *PostgresAdapter) EnforceReadOnly(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, "SET SESSION CHARACTERISTICS AS TRANSACTION READ ONLY")
	return err
}

func (a *PostgresAdapter) ListTablesQuery(databaseName string) (string, []any) {
	return `SELECT table_name FROM information_schema.tables WHERE table_schema = 'public' AND table_catalog = $1`,
		[]any{databaseName}
}

func (a *PostgresAdapter) ReadSchemaQuery(databaseName, tableName string) (string, []any) {
	return `SELECT column_name, data_type, is_nullable, column_default
		FROM information_schema.columns
		WHERE table_catalog = $1 AND table_schema = 'public' AND table_name = $2
		ORDER BY ordinal_position`, []any{databaseName, tableName}
}

func (a *PostgresAdapter) ScanSchemaRow(rows *sql.Rows) (map[string]any, error) {
	var colName, dataType, isNullable string
	var colDefault sql.NullString

	if err := rows.Scan(&colName, &dataType, &isNullable, &colDefault); err != nil {
		return nil, err
	}

	col := map[string]any{
		"column_name": colName,
		"data_type":   dataType,
		"is_nullable": isNullable,
	}
	if colDefault.Valid {
		col["column_default"] = colDefault.String
	}
	return col, nil
}

func (a *PostgresAdapter) ValidateQuery(sqlQuery string) error {
	cleaned := a.RemoveStringsAndComments(sqlQuery)

	if err := validateCommon(sqlQuery, cleaned); err != nil {
		return err
	}

	// PostgreSQL-specific forbidden patterns
	forbiddenPatterns := []struct {
		pattern string
		desc    string
	}{
		{`(?i)\bCOPY\s+.*\bTO\b`, "COPY ... TO"},
		{`(?i)\bCOPY\s+.*\bFROM\b`, "COPY ... FROM"},
		{`(?i)\bpg_read_file\s*\(`, "pg_read_file()"},
		{`(?i)\bpg_read_binary_file\s*\(`, "pg_read_binary_file()"},
		{`(?i)\bpg_ls_dir\s*\(`, "pg_ls_dir()"},
		{`(?i)\blo_import\s*\(`, "lo_import()"},
		{`(?i)\blo_export\s*\(`, "lo_export()"},
	}

	for _, fp := range forbiddenPatterns {
		re := regexp.MustCompile(fp.pattern)
		if re.MatchString(sqlQuery) {
			return fmt.Errorf("query contains forbidden pattern: %s", fp.desc)
		}
	}

	// PostgreSQL-specific DoS functions
	dosFunctions := []struct {
		pattern string
		desc    string
	}{
		{`(?i)\bpg_sleep\s*\(`, "pg_sleep()"},
		{`(?i)\bpg_sleep_for\s*\(`, "pg_sleep_for()"},
		{`(?i)\bpg_sleep_until\s*\(`, "pg_sleep_until()"},
		{`(?i)\bpg_advisory_lock\s*\(`, "pg_advisory_lock()"},
		{`(?i)\bpg_advisory_xact_lock\s*\(`, "pg_advisory_xact_lock()"},
		{`(?i)\bpg_try_advisory_lock\s*\(`, "pg_try_advisory_lock()"},
	}

	for _, df := range dosFunctions {
		re := regexp.MustCompile(df.pattern)
		if re.MatchString(sqlQuery) {
			return fmt.Errorf("query contains forbidden function: %s", df.desc)
		}
	}

	// PostgreSQL-specific dangerous keywords
	extraKeywords := []struct {
		pattern string
		desc    string
	}{
		{`(?i)(?:^|[^a-zA-Z_])CALL(?:[^a-zA-Z_]|$)`, "CALL"},
		{`(?i)(?:^|[^a-zA-Z_])EXECUTE(?:[^a-zA-Z_]|$)`, "EXECUTE"},
		{`(?i)(?:^|[^a-zA-Z_])COPY(?:[^a-zA-Z_]|$)`, "COPY"},
		{`(?i)(?:^|[^a-zA-Z_])LISTEN(?:[^a-zA-Z_]|$)`, "LISTEN"},
		{`(?i)(?:^|[^a-zA-Z_])NOTIFY(?:[^a-zA-Z_]|$)`, "NOTIFY"},
		{`(?i)(?:^|[^a-zA-Z_])PREPARE(?:[^a-zA-Z_]|$)`, "PREPARE"},
		{`(?i)(?:^|[^a-zA-Z_])DEALLOCATE(?:[^a-zA-Z_]|$)`, "DEALLOCATE"},
		{`(?i)(?:^|[^a-zA-Z_])VACUUM(?:[^a-zA-Z_]|$)`, "VACUUM"},
		{`(?i)(?:^|[^a-zA-Z_])REINDEX(?:[^a-zA-Z_]|$)`, "REINDEX"},
		{`(?i)(?:^|[^a-zA-Z_])CLUSTER(?:[^a-zA-Z_]|$)`, "CLUSTER"},
	}

	for _, dk := range extraKeywords {
		re := regexp.MustCompile(dk.pattern)
		if re.MatchString(cleaned) {
			return fmt.Errorf("query contains forbidden keyword: %s", dk.desc)
		}
	}

	return nil
}

// RemoveStringsAndComments strips string literals and comments from SQL
// for safe keyword detection. PostgreSQL-specific: no # comments, no backtick
// identifiers, handles $$ dollar-quoted strings, no backslash escaping by default.
func (a *PostgresAdapter) RemoveStringsAndComments(sql string) string {
	var result strings.Builder
	i := 0
	n := len(sql)

	for i < n {
		// Single-line comment starting with --
		if i+1 < n && sql[i] == '-' && sql[i+1] == '-' {
			for i < n && sql[i] != '\n' {
				i++
			}
			result.WriteByte(' ')
			continue
		}

		// Multi-line comment /* */
		if i+1 < n && sql[i] == '/' && sql[i+1] == '*' {
			i += 2
			for i+1 < n && !(sql[i] == '*' && sql[i+1] == '/') {
				i++
			}
			i += 2 // Skip */
			result.WriteByte(' ')
			continue
		}

		// Dollar-quoted string $tag$...$tag$ or $$...$$
		if sql[i] == '$' {
			tagEnd := strings.Index(sql[i+1:], "$")
			if tagEnd >= 0 {
				tag := sql[i : i+tagEnd+2] // e.g., "$$" or "$tag$"
				closeIdx := strings.Index(sql[i+len(tag):], tag)
				if closeIdx >= 0 {
					i += len(tag) + closeIdx + len(tag)
					result.WriteString("''") // Placeholder for string content
					continue
				}
			}
		}

		// Single-quoted string (no backslash escaping in standard PostgreSQL)
		if sql[i] == '\'' {
			i++
			for i < n {
				if sql[i] == '\'' {
					if i+1 < n && sql[i+1] == '\'' {
						i += 2 // Escaped quote ''
						continue
					}
					i++
					break
				}
				i++
			}
			result.WriteString("''") // Placeholder for string
			continue
		}

		// Double-quoted identifier (PostgreSQL standard identifier quoting)
		if sql[i] == '"' {
			result.WriteByte('"')
			i++
			for i < n {
				if sql[i] == '"' {
					if i+1 < n && sql[i+1] == '"' {
						result.WriteString(`""`)
						i += 2
						continue
					}
					result.WriteByte('"')
					i++
					break
				}
				result.WriteByte(sql[i])
				i++
			}
			continue
		}

		result.WriteByte(sql[i])
		i++
	}

	return result.String()
}
