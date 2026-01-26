package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	_ "github.com/go-sql-driver/mysql"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: mysql-mcp-server <dsn>")
		fmt.Fprintln(os.Stderr, "Example: mysql-mcp-server 'user:password@tcp(localhost:3306)/dbname'")
		os.Exit(1)
	}

	dsn := os.Args[1]

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
