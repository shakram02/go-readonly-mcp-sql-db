package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// MySQLAdapter implements DBAdapter for MySQL databases.
type MySQLAdapter struct{}

func (a *MySQLAdapter) DriverName() string { return "mysql" }
func (a *MySQLAdapter) ServerName() string { return "mysql-readonly-mcp-server" }
func (a *MySQLAdapter) URIScheme() string  { return "mysql" }

func (a *MySQLAdapter) BuildDSN() (string, error) {
	host := os.Getenv("MCP_MYSQL_HOST")
	port := os.Getenv("MCP_MYSQL_PORT")
	db := os.Getenv("MCP_MYSQL_DB")
	user := os.Getenv("MCP_MYSQL_USER")
	password := os.Getenv("MCP_MYSQL_PASSWORD")

	var missing []string
	if host == "" {
		missing = append(missing, "MCP_MYSQL_HOST")
	}
	if port == "" {
		missing = append(missing, "MCP_MYSQL_PORT")
	}
	if db == "" {
		missing = append(missing, "MCP_MYSQL_DB")
	}
	if user == "" {
		missing = append(missing, "MCP_MYSQL_USER")
	}
	if password == "" {
		missing = append(missing, "MCP_MYSQL_PASSWORD")
	}

	if len(missing) > 0 {
		return "", fmt.Errorf("missing required environment variables: %v", missing)
	}

	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s", user, password, host, port, db), nil
}

func (a *MySQLAdapter) DatabaseName(dsn string) string {
	// DSN format: user:password@tcp(host:port)/dbname?params
	parts := strings.Split(dsn, "/")
	if len(parts) < 2 {
		return ""
	}
	dbPart := parts[len(parts)-1]
	if idx := strings.Index(dbPart, "?"); idx != -1 {
		dbPart = dbPart[:idx]
	}
	return dbPart
}

func (a *MySQLAdapter) EnforceReadOnly(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, "SET SESSION TRANSACTION READ ONLY")
	return err
}

func (a *MySQLAdapter) ListTablesQuery(databaseName string) (string, []any) {
	return `SELECT table_name FROM information_schema.tables WHERE table_schema = ?`,
		[]any{databaseName}
}

func (a *MySQLAdapter) ReadSchemaQuery(databaseName, tableName string) (string, []any) {
	return `SELECT column_name, data_type, is_nullable, column_key, column_default, extra
		FROM information_schema.columns
		WHERE table_schema = ? AND table_name = ?
		ORDER BY ordinal_position`, []any{databaseName, tableName}
}

func (a *MySQLAdapter) ScanSchemaRow(rows *sql.Rows) (map[string]any, error) {
	var colName, dataType, isNullable, colKey string
	var colDefault, extra sql.NullString

	if err := rows.Scan(&colName, &dataType, &isNullable, &colKey, &colDefault, &extra); err != nil {
		return nil, err
	}

	col := map[string]any{
		"column_name": colName,
		"data_type":   dataType,
		"is_nullable": isNullable,
		"column_key":  colKey,
	}
	if colDefault.Valid {
		col["column_default"] = colDefault.String
	}
	if extra.Valid && extra.String != "" {
		col["extra"] = extra.String
	}
	return col, nil
}

func (a *MySQLAdapter) ValidateQuery(sqlQuery string) error {
	cleaned := a.RemoveStringsAndComments(sqlQuery)

	if err := validateCommon(sqlQuery, cleaned); err != nil {
		return err
	}

	// MySQL-specific forbidden patterns
	forbiddenPatterns := []struct {
		pattern string
		desc    string
	}{
		{`(?i)\bINTO\s+OUTFILE\b`, "INTO OUTFILE"},
		{`(?i)\bINTO\s+DUMPFILE\b`, "INTO DUMPFILE"},
		{`(?i)\bLOAD_FILE\s*\(`, "LOAD_FILE()"},
		{`(?i)\bINTO\s+@`, "INTO @variable"},
	}

	for _, fp := range forbiddenPatterns {
		re := regexp.MustCompile(fp.pattern)
		if re.MatchString(sqlQuery) {
			return fmt.Errorf("query contains forbidden pattern: %s", fp.desc)
		}
	}

	// MySQL-specific DoS functions
	dosFunctions := []struct {
		pattern string
		desc    string
	}{
		{`(?i)\bSLEEP\s*\(`, "SLEEP()"},
		{`(?i)\bBENCHMARK\s*\(`, "BENCHMARK()"},
		{`(?i)\bGET_LOCK\s*\(`, "GET_LOCK()"},
		{`(?i)\bRELEASE_LOCK\s*\(`, "RELEASE_LOCK()"},
		{`(?i)\bIS_FREE_LOCK\s*\(`, "IS_FREE_LOCK()"},
		{`(?i)\bIS_USED_LOCK\s*\(`, "IS_USED_LOCK()"},
		{`(?i)\bWAIT_FOR_EXECUTED_GTID_SET\s*\(`, "WAIT_FOR_EXECUTED_GTID_SET()"},
		{`(?i)\bWAIT_UNTIL_SQL_THREAD_AFTER_GTIDS\s*\(`, "WAIT_UNTIL_SQL_THREAD_AFTER_GTIDS()"},
		{`(?i)\bMASTER_POS_WAIT\s*\(`, "MASTER_POS_WAIT()"},
		{`(?i)\bSOURCE_POS_WAIT\s*\(`, "SOURCE_POS_WAIT()"},
	}

	for _, df := range dosFunctions {
		re := regexp.MustCompile(df.pattern)
		if re.MatchString(sqlQuery) {
			return fmt.Errorf("query contains forbidden function: %s", df.desc)
		}
	}

	// MySQL-specific dangerous keywords (beyond common set)
	extraKeywords := []struct {
		pattern string
		desc    string
	}{
		{`(?i)(?:^|[^a-zA-Z_])CALL(?:[^a-zA-Z_]|$)`, "CALL"},
		{`(?i)(?:^|[^a-zA-Z_])EXEC(?:[^a-zA-Z_]|$)`, "EXEC"},
		{`(?i)(?:^|[^a-zA-Z_])EXECUTE(?:[^a-zA-Z_]|$)`, "EXECUTE"},
		{`(?i)(?:^|[^a-zA-Z_])REPLACE(?:[^a-zA-Z_]|$)`, "REPLACE"},
		{`(?i)(?:^|[^a-zA-Z_])LOAD(?:[^a-zA-Z_]|$)`, "LOAD"},
		{`(?i)(?:^|[^a-zA-Z_])HANDLER(?:[^a-zA-Z_]|$)`, "HANDLER"},
		{`(?i)(?:^|[^a-zA-Z_])RENAME(?:[^a-zA-Z_]|$)`, "RENAME"},
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
// for safe keyword detection. MySQL-specific: supports # comments, backtick
// identifiers, and backslash escaping in strings.
func (a *MySQLAdapter) RemoveStringsAndComments(sql string) string {
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

		// Single-line comment starting with # (MySQL-specific)
		if sql[i] == '#' {
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

		// Single-quoted string
		if sql[i] == '\'' {
			i++
			for i < n {
				if sql[i] == '\'' {
					if i+1 < n && sql[i+1] == '\'' {
						i += 2 // Escaped quote
						continue
					}
					i++
					break
				}
				if sql[i] == '\\' && i+1 < n {
					i += 2 // Escaped character (MySQL-specific)
					continue
				}
				i++
			}
			result.WriteString("''") // Placeholder for string
			continue
		}

		// Double-quoted string (identifier in MySQL with ANSI_QUOTES, or string)
		if sql[i] == '"' {
			i++
			for i < n {
				if sql[i] == '"' {
					if i+1 < n && sql[i+1] == '"' {
						i += 2 // Escaped quote
						continue
					}
					i++
					break
				}
				if sql[i] == '\\' && i+1 < n {
					i += 2 // Escaped character (MySQL-specific)
					continue
				}
				i++
			}
			result.WriteString(`""`) // Placeholder for string
			continue
		}

		// Backtick-quoted identifier (MySQL-specific)
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

		result.WriteByte(sql[i])
		i++
	}

	return result.String()
}
