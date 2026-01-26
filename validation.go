package main

import (
	"fmt"
	"regexp"
	"strings"
)

// Query validation constants
const (
	MaxResultRows = 10000
)

// validateReadOnlyQuery ensures the SQL query is safe and read-only.
// Returns an error if the query is not allowed.
func validateReadOnlyQuery(sql string) error {
	trimmed := strings.TrimSpace(sql)
	if trimmed == "" {
		return fmt.Errorf("empty query")
	}

	upper := strings.ToUpper(trimmed)

	// Must start with SELECT, SHOW, DESCRIBE, or EXPLAIN
	allowedPrefixes := []string{"SELECT ", "SHOW ", "DESCRIBE ", "DESC ", "EXPLAIN "}
	hasAllowedPrefix := false
	for _, prefix := range allowedPrefixes {
		if strings.HasPrefix(upper, prefix) || upper == strings.TrimSpace(prefix) {
			hasAllowedPrefix = true
			break
		}
	}
	if !hasAllowedPrefix {
		return fmt.Errorf("only SELECT, SHOW, DESCRIBE, and EXPLAIN queries are allowed")
	}

	// Check for multiple statements
	// Remove string literals and comments first to avoid false positives
	cleaned := removeStringsAndComments(sql)
	if strings.Contains(cleaned, ";") {
		// Check if there's anything meaningful after the semicolon
		parts := strings.SplitN(cleaned, ";", 2)
		if len(parts) > 1 && strings.TrimSpace(parts[1]) != "" {
			return fmt.Errorf("multiple statements are not allowed")
		}
	}

	// Forbidden patterns that could modify data or filesystem
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
		if re.MatchString(sql) {
			return fmt.Errorf("query contains forbidden pattern: %s", fp.desc)
		}
	}

	// DoS prevention - block time/resource consuming functions
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
		if re.MatchString(sql) {
			return fmt.Errorf("query contains forbidden function: %s", df.desc)
		}
	}

	// Dangerous keywords that should never appear in a read-only query
	// Using word boundaries to avoid false positives (e.g., "settings" matching "SET")
	dangerousKeywords := []struct {
		pattern string
		desc    string
	}{
		{`(?i)(?:^|[^a-zA-Z_])INSERT(?:[^a-zA-Z_]|$)`, "INSERT"},
		{`(?i)(?:^|[^a-zA-Z_])UPDATE(?:[^a-zA-Z_]|$)`, "UPDATE"},
		{`(?i)(?:^|[^a-zA-Z_])DELETE(?:[^a-zA-Z_]|$)`, "DELETE"},
		{`(?i)(?:^|[^a-zA-Z_])DROP(?:[^a-zA-Z_]|$)`, "DROP"},
		{`(?i)(?:^|[^a-zA-Z_])CREATE(?:[^a-zA-Z_]|$)`, "CREATE"},
		{`(?i)(?:^|[^a-zA-Z_])ALTER(?:[^a-zA-Z_]|$)`, "ALTER"},
		{`(?i)(?:^|[^a-zA-Z_])TRUNCATE(?:[^a-zA-Z_]|$)`, "TRUNCATE"},
		{`(?i)(?:^|[^a-zA-Z_])GRANT(?:[^a-zA-Z_]|$)`, "GRANT"},
		{`(?i)(?:^|[^a-zA-Z_])REVOKE(?:[^a-zA-Z_]|$)`, "REVOKE"},
		{`(?i)(?:^|[^a-zA-Z_])CALL(?:[^a-zA-Z_]|$)`, "CALL"},
		{`(?i)(?:^|[^a-zA-Z_])EXEC(?:[^a-zA-Z_]|$)`, "EXEC"},
		{`(?i)(?:^|[^a-zA-Z_])EXECUTE(?:[^a-zA-Z_]|$)`, "EXECUTE"},
		{`(?i)(?:^|[^a-zA-Z_])REPLACE(?:[^a-zA-Z_]|$)`, "REPLACE"},
		{`(?i)(?:^|[^a-zA-Z_])LOAD(?:[^a-zA-Z_]|$)`, "LOAD"},
		{`(?i)(?:^|[^a-zA-Z_])HANDLER(?:[^a-zA-Z_]|$)`, "HANDLER"},
		{`(?i)(?:^|[^a-zA-Z_])RENAME(?:[^a-zA-Z_]|$)`, "RENAME"},
	}

	// Check against cleaned SQL (without strings/comments) to avoid false positives
	// from string literals containing these keywords
	for _, dk := range dangerousKeywords {
		re := regexp.MustCompile(dk.pattern)
		if re.MatchString(cleaned) {
			return fmt.Errorf("query contains forbidden keyword: %s", dk.desc)
		}
	}

	// Block SET statements (but not column/table names containing 'set')
	// SET must appear as a statement keyword, not as part of an identifier
	setPattern := regexp.MustCompile(`(?i)(?:^|;)\s*SET\b`)
	if setPattern.MatchString(cleaned) {
		return fmt.Errorf("SET statements are not allowed")
	}

	return nil
}

// removeStringsAndComments removes string literals and comments from SQL
// to allow accurate keyword detection without false positives from literals.
func removeStringsAndComments(sql string) string {
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

		// Single-line comment starting with #
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
					i += 2 // Escaped character
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
					i += 2 // Escaped character
					continue
				}
				i++
			}
			result.WriteString(`""`) // Placeholder for string
			continue
		}

		// Backtick-quoted identifier
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
