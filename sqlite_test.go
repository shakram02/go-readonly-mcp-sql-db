package main

import (
	"strings"
	"testing"
)

func TestSQLiteValidateQuery_AllowedQueries(t *testing.T) {
	adapter := &SQLiteAdapter{}
	allowedQueries := []string{
		"SELECT * FROM users",
		"SELECT id, name FROM users WHERE id = 1",
		"select * from users",
		"SHOW TABLES",
		"DESCRIBE users",
		"DESC users",
		"EXPLAIN SELECT * FROM users",
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

func TestSQLiteValidateQuery_BlockedQueries(t *testing.T) {
	adapter := &SQLiteAdapter{}
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
		// SQLite-specific blocked queries
		{"SELECT load_extension('hack.so')", "load_extension"},
		{"SELECT writefile('/tmp/data', content)", "writefile"},
		{"SELECT edit(content)", "edit"},
		{"SELECT fts3_tokenizer('simple')", "fts3_tokenizer"},
		{"REPLACE INTO users VALUES (1, 'test')", "REPLACE"},
		{"ATTACH DATABASE '/tmp/other.db' AS other", "ATTACH"},
		{"DETACH DATABASE other", "DETACH"},
		{"REINDEX users", "REINDEX"},
		{"VACUUM", "VACUUM"},
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

func TestSQLiteValidateQuery_PragmaWriteBlocked(t *testing.T) {
	adapter := &SQLiteAdapter{}
	blockedPragmas := []string{
		"EXPLAIN PRAGMA journal_mode = WAL",
		"EXPLAIN PRAGMA synchronous = OFF",
		"EXPLAIN PRAGMA foreign_keys = ON",
	}

	for _, query := range blockedPragmas {
		t.Run(query, func(t *testing.T) {
			err := adapter.ValidateQuery(query)
			if err == nil {
				t.Errorf("Expected PRAGMA write to be blocked: %s", query)
			}
		})
	}
}

func TestSQLiteValidateQuery_EmptyQuery(t *testing.T) {
	adapter := &SQLiteAdapter{}

	err := adapter.ValidateQuery("")
	if err == nil {
		t.Error("Expected empty query to be rejected")
	}

	err = adapter.ValidateQuery("   ")
	if err == nil {
		t.Error("Expected whitespace-only query to be rejected")
	}
}

func TestSQLiteValidateQuery_CommentInjection(t *testing.T) {
	adapter := &SQLiteAdapter{}
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

func TestSQLiteRemoveStringsAndComments(t *testing.T) {
	adapter := &SQLiteAdapter{}
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
			name:     "backtick identifier preserved",
			input:    "SELECT * FROM `table_name`",
			expected: "SELECT * FROM `table_name`",
		},
		{
			name:     "bracket identifier preserved",
			input:    "SELECT * FROM [table_name]",
			expected: "SELECT * FROM [table_name]",
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

func TestSQLiteRemoveStringsAndComments_NoHash(t *testing.T) {
	adapter := &SQLiteAdapter{}
	// # is NOT a comment in SQLite
	input := "SELECT # FROM users"
	result := adapter.RemoveStringsAndComments(input)
	if !strings.Contains(result, "#") {
		t.Errorf("# should not be treated as a comment in SQLite: %s", result)
	}
}
