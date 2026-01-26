package main

import (
	"strings"
	"testing"
)

func TestValidateReadOnlyQuery_AllowedQueries(t *testing.T) {
	allowedQueries := []string{
		"SELECT * FROM users",
		"SELECT id, name FROM users WHERE id = 1",
		"select * from users", // lowercase
		"SHOW TABLES",
		"SHOW DATABASES",
		"DESCRIBE users",
		"DESC users",
		"EXPLAIN SELECT * FROM users",
		"SELECT * FROM settings", // 'settings' contains 'set' but should be allowed
		"SELECT * FROM user_settings WHERE setting_name = 'theme'",
		"SELECT created_at FROM orders",  // 'created' contains 'create'
		"SELECT updated_at FROM products", // 'updated' contains 'update'
		"SELECT deleted FROM items",       // 'deleted' contains 'delete'
		"SELECT * FROM users WHERE name = 'DROP TABLE users'", // keyword in string literal
	}

	for _, query := range allowedQueries {
		t.Run(query, func(t *testing.T) {
			err := validateReadOnlyQuery(query)
			if err != nil {
				t.Errorf("Expected query to be allowed, but got error: %v", err)
			}
		})
	}
}

func TestValidateReadOnlyQuery_BlockedQueries(t *testing.T) {
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
		{"CALL some_procedure()", "CALL"},
		{"EXECUTE some_statement", "EXECUTE"},
		{"SET @var = 1", "SET"},
		{"SELECT * INTO OUTFILE '/tmp/data.txt' FROM users", "INTO OUTFILE"},
		{"SELECT * INTO DUMPFILE '/tmp/data.bin' FROM users", "INTO DUMPFILE"},
		{"SELECT LOAD_FILE('/etc/passwd')", "LOAD_FILE"},
		{"SELECT SLEEP(10)", "SLEEP"},
		{"SELECT BENCHMARK(1000000, SHA1('test'))", "BENCHMARK"},
		{"SELECT GET_LOCK('lock', 10)", "GET_LOCK"},
		{"SELECT 1; DROP TABLE users", "multiple statements"},
		{"SELECT 1; -- comment\nDROP TABLE users", "multiple statements"},
		{"LOAD DATA INFILE '/tmp/data.txt' INTO TABLE users", "LOAD"},
		{"REPLACE INTO users VALUES (1, 'test')", "REPLACE"},
		{"HANDLER users OPEN", "HANDLER"},
		{"RENAME TABLE users TO users_old", "RENAME"},
	}

	for _, tc := range blockedQueries {
		t.Run(tc.query, func(t *testing.T) {
			err := validateReadOnlyQuery(tc.query)
			if err == nil {
				t.Errorf("Expected query to be blocked for %s, but it was allowed", tc.shouldBlock)
			}
		})
	}
}

func TestValidateReadOnlyQuery_EmptyQuery(t *testing.T) {
	err := validateReadOnlyQuery("")
	if err == nil {
		t.Error("Expected empty query to be rejected")
	}

	err = validateReadOnlyQuery("   ")
	if err == nil {
		t.Error("Expected whitespace-only query to be rejected")
	}
}

func TestValidateReadOnlyQuery_CommentInjection(t *testing.T) {
	blockedQueries := []string{
		"SELECT 1 -- ; DROP TABLE users",
		"SELECT 1 /* ; DROP TABLE users */",
		"SELECT 1 # ; DROP TABLE users",
	}

	for _, query := range blockedQueries {
		t.Run(query, func(t *testing.T) {
			err := validateReadOnlyQuery(query)
			// These should be allowed as the dangerous part is in a comment
			// But multi-statement check should not false-positive on comments
			if err != nil && strings.Contains(err.Error(), "multiple statements") {
				t.Errorf("False positive on comment: %v", err)
			}
		})
	}
}

func TestRemoveStringsAndComments(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "SELECT * FROM users WHERE name = 'DROP TABLE'",
			expected: "SELECT * FROM users WHERE name = ''",
		},
		{
			input:    "SELECT * FROM users -- comment",
			expected: "SELECT * FROM users  ",
		},
		{
			input:    "SELECT * FROM users /* comment */",
			expected: "SELECT * FROM users  ",
		},
		{
			input:    "SELECT * FROM `table_name`",
			expected: "SELECT * FROM `table_name`",
		},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			result := removeStringsAndComments(tc.input)
			if result != tc.expected {
				t.Errorf("Expected %q, got %q", tc.expected, result)
			}
		})
	}
}
