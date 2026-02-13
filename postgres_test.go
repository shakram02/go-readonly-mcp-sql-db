package main

import (
	"strings"
	"testing"
)

func TestPostgresValidateQuery_AllowedQueries(t *testing.T) {
	adapter := &PostgresAdapter{}
	allowedQueries := []string{
		"SELECT * FROM users",
		"SELECT id, name FROM users WHERE id = 1",
		"select * from users",
		"SHOW server_version",
		"DESCRIBE users",
		"DESC users",
		"EXPLAIN SELECT * FROM users",
		"EXPLAIN ANALYZE SELECT * FROM users",
		"SELECT * FROM settings",
		"SELECT * FROM user_settings WHERE setting_name = 'theme'",
		"SELECT created_at FROM orders",
		"SELECT updated_at FROM products",
		"SELECT deleted FROM items",
		"SELECT * FROM users WHERE name = 'DROP TABLE users'", // keyword in string literal
	}

	for _, query := range allowedQueries {
		t.Run(query, func(t *testing.T) {
			err := adapter.ValidateQuery(query)
			if err != nil {
				t.Errorf("Expected query to be allowed, but got error: %v", err)
			}
		})
	}
}

func TestPostgresValidateQuery_BlockedQueries(t *testing.T) {
	adapter := &PostgresAdapter{}
	blockedQueries := []struct {
		query       string
		shouldBlock string
	}{
		{"INSERT INTO users VALUES (1, 'test')", "INSERT"},
		{"UPDATE users SET name = 'test'", "UPDATE"},
		{"DELETE FROM users", "DELETE"},
		{"DROP TABLE users", "DROP"},
		{"CREATE TABLE test (id INT)", "CREATE"},
		{"ALTER TABLE users ADD COLUMN age INT", "ALTER"},
		{"TRUNCATE TABLE users", "TRUNCATE"},
		{"GRANT ALL ON *.* TO 'user'", "GRANT"},
		{"REVOKE ALL ON *.* FROM 'user'", "REVOKE"},
		{"SET @var = 1", "SET"},
		{"SELECT 1; DROP TABLE users", "multiple statements"},
		// PostgreSQL-specific blocked queries
		{"SELECT pg_sleep(10)", "pg_sleep"},
		{"SELECT pg_sleep_for('5 seconds')", "pg_sleep_for"},
		{"SELECT pg_sleep_until('2025-01-01')", "pg_sleep_until"},
		{"SELECT pg_advisory_lock(1)", "pg_advisory_lock"},
		{"SELECT pg_advisory_xact_lock(1)", "pg_advisory_xact_lock"},
		{"SELECT pg_try_advisory_lock(1)", "pg_try_advisory_lock"},
		{"SELECT pg_read_file('/etc/passwd')", "pg_read_file"},
		{"SELECT pg_read_binary_file('/etc/passwd')", "pg_read_binary_file"},
		{"SELECT pg_ls_dir('/tmp')", "pg_ls_dir"},
		{"SELECT lo_import('/etc/passwd')", "lo_import"},
		{"SELECT lo_export(12345, '/tmp/out')", "lo_export"},
		{"COPY users TO '/tmp/data.csv'", "COPY TO"},
		{"COPY users FROM '/tmp/data.csv'", "COPY FROM"},
		{"CALL some_procedure()", "CALL"},
		{"EXECUTE some_statement", "EXECUTE"},
		{"LISTEN channel", "LISTEN"},
		{"NOTIFY channel", "NOTIFY"},
		{"PREPARE stmt AS SELECT 1", "PREPARE"},
		{"DEALLOCATE stmt", "DEALLOCATE"},
		{"VACUUM users", "VACUUM"},
		{"REINDEX TABLE users", "REINDEX"},
		{"CLUSTER users", "CLUSTER"},
	}

	for _, tc := range blockedQueries {
		t.Run(tc.query, func(t *testing.T) {
			err := adapter.ValidateQuery(tc.query)
			if err == nil {
				t.Errorf("Expected query to be blocked for %s, but it was allowed", tc.shouldBlock)
			}
		})
	}
}

func TestPostgresValidateQuery_EmptyQuery(t *testing.T) {
	adapter := &PostgresAdapter{}

	err := adapter.ValidateQuery("")
	if err == nil {
		t.Error("Expected empty query to be rejected")
	}

	err = adapter.ValidateQuery("   ")
	if err == nil {
		t.Error("Expected whitespace-only query to be rejected")
	}
}

func TestPostgresValidateQuery_CommentInjection(t *testing.T) {
	adapter := &PostgresAdapter{}
	queries := []string{
		"SELECT 1 -- ; DROP TABLE users",
		"SELECT 1 /* ; DROP TABLE users */",
	}

	for _, query := range queries {
		t.Run(query, func(t *testing.T) {
			err := adapter.ValidateQuery(query)
			if err != nil && strings.Contains(err.Error(), "multiple statements") {
				t.Errorf("False positive on comment: %v", err)
			}
		})
	}
}

func TestPostgresRemoveStringsAndComments(t *testing.T) {
	adapter := &PostgresAdapter{}
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "single-quoted string stripped",
			input:    "SELECT * FROM users WHERE name = 'DROP TABLE'",
			expected: "SELECT * FROM users WHERE name = ''",
		},
		{
			name:     "-- comment stripped",
			input:    "SELECT * FROM users -- comment",
			expected: "SELECT * FROM users  ",
		},
		{
			name:     "/* */ comment stripped",
			input:    "SELECT * FROM users /* comment */",
			expected: "SELECT * FROM users  ",
		},
		{
			name:     "double-quoted identifier preserved",
			input:    `SELECT * FROM "table_name"`,
			expected: `SELECT * FROM "table_name"`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := adapter.RemoveStringsAndComments(tc.input)
			if result != tc.expected {
				t.Errorf("Expected %q, got %q", tc.expected, result)
			}
		})
	}
}

func TestPostgresRemoveStringsAndComments_DollarQuoting(t *testing.T) {
	adapter := &PostgresAdapter{}

	// $$ dollar-quoted string should be stripped
	input := "SELECT * FROM t WHERE body = $$DROP TABLE users$$"
	result := adapter.RemoveStringsAndComments(input)
	if strings.Contains(result, "DROP") {
		t.Errorf("Dollar-quoted string content was not stripped: %s", result)
	}

	// $tag$ tagged dollar-quoted string should be stripped
	input = "SELECT * FROM t WHERE body = $tag$DROP TABLE users$tag$"
	result = adapter.RemoveStringsAndComments(input)
	if strings.Contains(result, "DROP") {
		t.Errorf("Tagged dollar-quoted string content was not stripped: %s", result)
	}
}

func TestPostgresRemoveStringsAndComments_NoHash(t *testing.T) {
	adapter := &PostgresAdapter{}
	// # is NOT a comment in PostgreSQL
	input := "SELECT # FROM users"
	result := adapter.RemoveStringsAndComments(input)
	if !strings.Contains(result, "#") {
		t.Errorf("# should not be treated as a comment in PostgreSQL: %s", result)
	}
}
