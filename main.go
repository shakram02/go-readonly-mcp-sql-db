package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	_ "github.com/go-sql-driver/mysql"
)

func getDSN() (string, error) {
	// If DSN provided as argument, use it directly
	if len(os.Args) >= 2 {
		return os.Args[1], nil
	}

	// Build DSN from environment variables
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
		return "", fmt.Errorf("missing required environment variables: %v\n\nUsage: mysql-mcp-server <dsn>\n   or: set MCP_MYSQL_HOST, MCP_MYSQL_PORT, MCP_MYSQL_DB, MCP_MYSQL_USER, MCP_MYSQL_PASSWORD", missing)
	}

	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s", user, password, host, port, db), nil
}

func main() {
	dsn, err := getDSN()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// Create context that cancels on interrupt signals
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		logError("Received shutdown signal")
		cancel()
	}()

	server, err := NewMySQLMCPServer(ctx, dsn)
	if err != nil {
		logError("Failed to create server: %v", err)
		os.Exit(1)
	}
	defer server.Close()

	logError("MySQL MCP Server started (read-only mode)")

	if err := server.Run(); err != nil {
		if err == context.Canceled {
			logError("Server shutdown gracefully")
		} else {
			logError("Server error: %v", err)
			os.Exit(1)
		}
	}
}
