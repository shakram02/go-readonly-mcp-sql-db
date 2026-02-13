package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	_ "modernc.org/sqlite"
)

func selectAdapter() (DBAdapter, error) {
	driver := strings.ToLower(os.Getenv("MCP_DB_DRIVER"))
	if driver == "" {
		driver = "mysql" // backward compatibility
	}

	switch driver {
	case "mysql":
		return &MySQLAdapter{}, nil
	case "postgres", "postgresql":
		return &PostgresAdapter{}, nil
	case "sqlite", "sqlite3":
		return &SQLiteAdapter{}, nil
	default:
		return nil, fmt.Errorf("unsupported database driver: %s (supported: mysql, postgres, sqlite)", driver)
	}
}

func getDSN(adapter DBAdapter) (string, error) {
	// If DSN provided as argument, use it directly
	if len(os.Args) >= 2 {
		return os.Args[1], nil
	}

	// Build DSN from environment variables using the adapter
	return adapter.BuildDSN()
}

func loadConfig() {
	if v := os.Getenv("MCP_QUERY_TIMEOUT"); v != "" {
		secs, err := strconv.Atoi(v)
		if err != nil || secs <= 0 {
			fmt.Fprintf(os.Stderr, "Invalid MCP_QUERY_TIMEOUT=%q, using default %v\n", v, QueryTimeout)
		} else {
			QueryTimeout = time.Duration(secs) * time.Second
		}
	}

	if v := os.Getenv("MCP_MAX_ROWS"); v != "" {
		rows, err := strconv.Atoi(v)
		if err != nil || rows <= 0 {
			fmt.Fprintf(os.Stderr, "Invalid MCP_MAX_ROWS=%q, using default %d\n", v, MaxResultRows)
		} else {
			MaxResultRows = rows
		}
	}
}

func main() {
	loadConfig()

	adapter, err := selectAdapter()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	dsn, err := getDSN(adapter)
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

	server, err := NewMCPServer(ctx, adapter, dsn)
	if err != nil {
		logError("Failed to create server: %v", err)
		os.Exit(1)
	}
	defer server.Close()

	logError("%s started (read-only mode)", adapter.ServerName())

	if err := server.Run(); err != nil {
		if err == context.Canceled {
			logError("Server shutdown gracefully")
		} else {
			logError("Server error: %v", err)
			os.Exit(1)
		}
	}
}
