package main

import (
	"fmt"
	"regexp"
	"strings"
)

// MaxResultRows is overridable via MCP_MAX_ROWS env var
var MaxResultRows = 10000

// commonDangerousKeywords are DML/DDL keywords blocked by all databases.
var commonDangerousKeywords = []struct {
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
}

// validateCommon runs validation checks shared across all database types.
// sqlQuery is the original query; cleanedSQL has strings/comments removed.
func validateCommon(sqlQuery string, cleanedSQL string) error {
	trimmed := strings.TrimSpace(sqlQuery)
	if trimmed == "" {
		return fmt.Errorf("empty query")
	}

	upper := strings.ToUpper(trimmed)

	// Must start with an allowed prefix
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
	if strings.Contains(cleanedSQL, ";") {
		parts := strings.SplitN(cleanedSQL, ";", 2)
		if len(parts) > 1 && strings.TrimSpace(parts[1]) != "" {
			return fmt.Errorf("multiple statements are not allowed")
		}
	}

	// Common dangerous keywords
	for _, dk := range commonDangerousKeywords {
		re := regexp.MustCompile(dk.pattern)
		if re.MatchString(cleanedSQL) {
			return fmt.Errorf("query contains forbidden keyword: %s", dk.desc)
		}
	}

	// Block SET statements (but not column/table names containing 'set')
	setPattern := regexp.MustCompile(`(?i)(?:^|;)\s*SET\b`)
	if setPattern.MatchString(cleanedSQL) {
		return fmt.Errorf("SET statements are not allowed")
	}

	return nil
}
