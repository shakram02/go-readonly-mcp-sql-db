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

// Server configuration defaults (QueryTimeout is overridable via MCP_QUERY_TIMEOUT)
var QueryTimeout = 30 * time.Second

const (
	ConnectionTimeout  = 10 * time.Second
	MaxConnectionsIdle = 5
	MaxConnectionsOpen = 10
)

// MCPServer handles MCP protocol over stdio
type MCPServer struct {
	db           *sql.DB
	adapter      DBAdapter
	databaseName string
	initialized  bool
	ctx          context.Context
	cancel       context.CancelFunc
}

// NewMCPServer creates a new MCP server connected to the database via the adapter
func NewMCPServer(ctx context.Context, adapter DBAdapter, dsn string) (*MCPServer, error) {
	db, err := sql.Open(adapter.DriverName(), dsn)
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

	// Extract database name using adapter-specific parsing
	dbName := adapter.DatabaseName(dsn)

	// Enforce read-only mode using adapter-specific mechanism
	if err := adapter.EnforceReadOnly(ctx, db); err != nil {
		logError("Warning: Could not set read-only mode: %v", err)
	}

	serverCtx, serverCancel := context.WithCancel(ctx)

	return &MCPServer{
		db:           db,
		adapter:      adapter,
		databaseName: dbName,
		ctx:          serverCtx,
		cancel:       serverCancel,
	}, nil
}

// Run starts the MCP server, reading from stdin and writing to stdout
func (s *MCPServer) Run() error {
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

func (s *MCPServer) handleMessage(data []byte) *JSONRPCResponse {
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

func (s *MCPServer) handleRequest(req *JSONRPCRequest) *JSONRPCResponse {
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
func (s *MCPServer) Shutdown() {
	if s.cancel != nil {
		s.cancel()
	}
}

// Close releases all resources
func (s *MCPServer) Close() error {
	s.Shutdown()
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

func logError(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "[mcp-server] "+format+"\n", args...)
}
