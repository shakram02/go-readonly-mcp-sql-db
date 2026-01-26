package main

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// Server configuration constants
const (
	QueryTimeout       = 30 * time.Second
	ConnectionTimeout  = 10 * time.Second
	MaxConnectionsIdle = 5
	MaxConnectionsOpen = 10
)

// MySQLMCPServer handles MCP protocol over stdio
type MySQLMCPServer struct {
	db           *sql.DB
	databaseName string
	initialized  bool
	ctx          context.Context
	cancel       context.CancelFunc
}

// NewMySQLMCPServer creates a new MCP server connected to MySQL
func NewMySQLMCPServer(ctx context.Context, dsn string) (*MySQLMCPServer, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool
	db.SetMaxIdleConns(MaxConnectionsIdle)
	db.SetMaxOpenConns(MaxConnectionsOpen)
	db.SetConnMaxLifetime(time.Hour)

	// Test connection with timeout
	pingCtx, pingCancel := context.WithTimeout(ctx, ConnectionTimeout)
	defer pingCancel()

	if err := db.PingContext(pingCtx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Extract database name from DSN
	dbName := extractDatabaseName(dsn)

	// Set connection to read-only mode
	_, err = db.ExecContext(ctx, "SET SESSION TRANSACTION READ ONLY")
	if err != nil {
		logError("Warning: Could not set read-only transaction mode: %v", err)
	}

	serverCtx, serverCancel := context.WithCancel(ctx)

	return &MySQLMCPServer{
		db:           db,
		databaseName: dbName,
		ctx:          serverCtx,
		cancel:       serverCancel,
	}, nil
}

func extractDatabaseName(dsn string) string {
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

// Run starts the MCP server, reading from stdin and writing to stdout
func (s *MySQLMCPServer) Run() error {
	reader := bufio.NewReader(os.Stdin)

	for {
		select {
		case <-s.ctx.Done():
			return s.ctx.Err()
		default:
		}

		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("failed to read input: %w", err)
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		response := s.handleMessage([]byte(line))
		if response != nil {
			responseBytes, err := json.Marshal(response)
			if err != nil {
				logError("Failed to marshal response: %v", err)
				continue
			}
			fmt.Println(string(responseBytes))
		}
	}
}

func (s *MySQLMCPServer) handleMessage(data []byte) *JSONRPCResponse {
	var req JSONRPCRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return &JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      nil,
			Error: &Error{
				Code:    ParseError,
				Message: "Parse error",
				Data:    err.Error(),
			},
		}
	}

	if req.JSONRPC != "2.0" {
		return &JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &Error{
				Code:    InvalidRequest,
				Message: "Invalid JSON-RPC version",
			},
		}
	}

	return s.handleRequest(&req)
}

func (s *MySQLMCPServer) handleRequest(req *JSONRPCRequest) *JSONRPCResponse {
	var result any
	var err *Error

	switch req.Method {
	case "initialize":
		result, err = s.handleInitialize(req.Params)
	case "initialized":
		// Notification, no response needed
		return nil
	case "tools/list":
		result, err = s.handleListTools()
	case "tools/call":
		result, err = s.handleCallTool(req.Params)
	case "resources/list":
		result, err = s.handleListResources()
	case "resources/read":
		result, err = s.handleReadResource(req.Params)
	case "ping":
		result = map[string]any{}
	default:
		err = &Error{
			Code:    MethodNotFound,
			Message: fmt.Sprintf("Method not found: %s", req.Method),
		}
	}

	return &JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  result,
		Error:   err,
	}
}

// Shutdown gracefully shuts down the server
func (s *MySQLMCPServer) Shutdown() {
	if s.cancel != nil {
		s.cancel()
	}
}

// Close releases all resources
func (s *MySQLMCPServer) Close() error {
	s.Shutdown()
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

func logError(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "[mysql-mcp] "+format+"\n", args...)
}
