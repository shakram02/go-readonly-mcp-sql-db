package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// SQLiteAdapter implements DBAdapter for SQLite databases.
type SQLiteAdapter struct{}

func (a *SQLiteAdapter) DriverName() string { return "sqlite" }
func (a *SQLiteAdapter) ServerName() string { return "sqlite-readonly-mcp-server" }
func (a *SQLiteAdapter) URIScheme() string  { return "sqlite" }

func (a *SQLiteAdapter) BuildDSN() (string, error) {
	dbPath := os.Getenv("MCP_SQLITE_PATH")
	if dbPath == "" {
		return "", fmt.Errorf("missing required environment variable: MCP_SQLITE_PATH")
	}
	// Enforce read-only mode via DSN parameter
	if !strings.Contains(dbPath, "?") {
		return dbPath + "?mode=ro", nil
	}
	if !strings.Contains(dbPath, "mode=") {
		return dbPath + "&mode=ro", nil
	}
	return dbPath, nil
}

func (a *SQLiteAdapter) DatabaseName(dsn string) string {
	// DSN is a file path, possibly with ?mode=ro
	path := dsn
	if idx := strings.Index(path, "?"); idx != -1 {
		path = path[:idx]
	}
	// Extract just the filename without directory
	parts := strings.Split(path, "/")
	name := parts[len(parts)-1]
	// Remove common extensions for display
	name = strings.TrimSuffix(name, ".db")
	name = strings.TrimSuffix(name, ".sqlite")
	name = strings.TrimSuffix(name, ".sqlite3")
	return name
}

func (a *SQLiteAdapter) EnforceReadOnly(ctx context.Context, db *sql.DB) error {
	// Read-only is primarily enforced via ?mode=ro in the DSN.
	// PRAGMA query_only provides defense-in-depth.
	_, err := db.ExecContext(ctx, "PRAGMA query_only = ON")
	return err
}

func (a *SQLiteAdapter) ListTablesQuery(databaseName string) (string, []any) {
	// SQLite has no information_schema. Use sqlite_master.
	// databaseName is ignored (SQLite has one DB per file).
	return `SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name`,
		nil
}

func (a *SQLiteAdapter) ReadSchemaQuery(databaseName, tableName string) (string, []any) {
	// PRAGMA table_info cannot use ? placeholders, so we embed the table name safely.
	return fmt.Sprintf("PRAGMA table_info('%s')", strings.ReplaceAll(tableName, "'", "''")),
		nil
}

func (a *SQLiteAdapter) ScanSchemaRow(rows *sql.Rows) (map[string]any, error) {
	// PRAGMA table_info returns: cid, name, type, notnull, dflt_value, pk
	var cid int
	var name, colType string
	var notNull, pk int
	var dfltValue sql.NullString

	if err := rows.Scan(&cid, &name, &colType, &notNull, &dfltValue, &pk); err != nil {
		return nil, err
	}

	isNullable := "YES"
	if notNull == 1 {
		isNullable = "NO"
	}

	col := map[string]any{
		"column_name": name,
		"data_type":   colType,
		"is_nullable": isNullable,
	}
	if pk > 0 {
		col["column_key"] = "PRI"
	}
	if dfltValue.Valid {
		col["column_default"] = dfltValue.String
	}
	return col, nil
}

func (a *SQLiteAdapter) ValidateQuery(sqlQuery string) error {
	cleaned := a.RemoveStringsAndComments(sqlQuery)

	if err := validateCommon(sqlQuery, cleaned); err != nil {
		return err
	}

	// SQLite-specific forbidden patterns
	forbiddenPatterns := []struct {
		pattern string
		desc    string
	}{
		{`(?i)\bload_extension\s*\(`, "load_extension()"},
		{`(?i)\bwritefile\s*\(`, "writefile()"},
		{`(?i)\bedit\s*\(`, "edit()"},
		{`(?i)\bfts3_tokenizer\s*\(`, "fts3_tokenizer()"},
	}

	for _, fp := range forbiddenPatterns {
		re := regexp.MustCompile(fp.pattern)
		if re.MatchString(sqlQuery) {
			return fmt.Errorf("query contains forbidden pattern: %s", fp.desc)
		}
	}

	// SQLite-specific dangerous keywords
	extraKeywords := []struct {
		pattern string
		desc    string
	}{
		{`(?i)(?:^|[^a-zA-Z_])REPLACE(?:[^a-zA-Z_]|$)`, "REPLACE"},
		{`(?i)(?:^|[^a-zA-Z_])ATTACH(?:[^a-zA-Z_]|$)`, "ATTACH"},
		{`(?i)(?:^|[^a-zA-Z_])DETACH(?:[^a-zA-Z_]|$)`, "DETACH"},
		{`(?i)(?:^|[^a-zA-Z_])REINDEX(?:[^a-zA-Z_]|$)`, "REINDEX"},
		{`(?i)(?:^|[^a-zA-Z_])VACUUM(?:[^a-zA-Z_]|$)`, "VACUUM"},
	}

	for _, dk := range extraKeywords {
		re := regexp.MustCompile(dk.pattern)
		if re.MatchString(cleaned) {
			return fmt.Errorf("query contains forbidden keyword: %s", dk.desc)
		}
	}

	// Block PRAGMA writes (PRAGMA x = value), but allow read PRAGMAs
	pragmaWritePattern := regexp.MustCompile(`(?i)\bPRAGMA\s+\w+\s*=`)
	if pragmaWritePattern.MatchString(cleaned) {
		return fmt.Errorf("PRAGMA writes are not allowed")
	}

	return nil
}

// RemoveStringsAndComments strips string literals and comments from SQL
// for safe keyword detection. SQLite-specific: no # comments, no backslash
// escaping, supports backtick and [bracket] identifiers.
func (a *SQLiteAdapter) RemoveStringsAndComments(sql string) string {
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

		// Single-quoted string (no backslash escaping in SQLite)
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

		// Double-quoted identifier/string
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

		// Backtick-quoted identifier (SQLite compatibility)
		if sql[i] == '`' {
			result.WriteByte('`')
			i++
			for i < n && sql[i] != '`' {
				result.WriteByte(sql[i])
				i++
			}
			if i < n {
				result.WriteByte('`')
				i++
			}
			continue
		}

		// [bracket]-quoted identifier (SQL Server compatibility in SQLite)
		if sql[i] == '[' {
			result.WriteByte('[')
			i++
			for i < n && sql[i] != ']' {
				result.WriteByte(sql[i])
				i++
			}
			if i < n {
				result.WriteByte(']')
				i++
			}
			continue
		}

		result.WriteByte(sql[i])
		i++
	}

	return result.String()
}
