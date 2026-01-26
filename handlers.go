package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

func (s *MySQLMCPServer) handleInitialize(params json.RawMessage) (*InitializeResult, *Error) {
	var initParams InitializeParams
	if params != nil {
		if err := json.Unmarshal(params, &initParams); err != nil {
			return nil, &Error{
				Code:    InvalidParams,
				Message: "Invalid initialize parameters",
				Data:    err.Error(),
			}
		}
	}

	s.initialized = true

	return &InitializeResult{
		ProtocolVersion: ProtocolVersion,
		Capabilities: ServerCapabilities{
			Tools:     &ToolsCapability{},
			Resources: &ResourcesCapability{},
		},
		ServerInfo: ServerInfo{
			Name:    ServerName,
			Version: ServerVersion,
		},
	}, nil
}

func (s *MySQLMCPServer) handleListTools() (*ListToolsResult, *Error) {
	return &ListToolsResult{
		Tools: []Tool{
			{
				Name:        "query",
				Description: "Execute a read-only SQL query (SELECT, SHOW, DESCRIBE, EXPLAIN only)",
				InputSchema: InputSchema{
					Type: "object",
					Properties: map[string]Property{
						"sql": {
							Type:        "string",
							Description: "The SQL query to execute (SELECT, SHOW, DESCRIBE, or EXPLAIN)",
						},
					},
					Required: []string{"sql"},
				},
			},
		},
	}, nil
}

func (s *MySQLMCPServer) handleCallTool(params json.RawMessage) (*CallToolResult, *Error) {
	var callParams CallToolParams
	if err := json.Unmarshal(params, &callParams); err != nil {
		return nil, &Error{
			Code:    InvalidParams,
			Message: "Invalid parameters",
			Data:    err.Error(),
		}
	}

	switch callParams.Name {
	case "query":
		return s.executeQuery(callParams.Arguments)
	default:
		return nil, &Error{
			Code:    MethodNotFound,
			Message: fmt.Sprintf("Unknown tool: %s", callParams.Name),
		}
	}
}

func (s *MySQLMCPServer) executeQuery(args map[string]any) (*CallToolResult, *Error) {
	sqlQuery, ok := args["sql"].(string)
	if !ok || sqlQuery == "" {
		return nil, &Error{
			Code:    InvalidParams,
			Message: "Missing or invalid 'sql' parameter",
		}
	}

	// Validate query is read-only
	if err := validateReadOnlyQuery(sqlQuery); err != nil {
		return &CallToolResult{
			Content: []Content{{Type: "text", Text: fmt.Sprintf("Query rejected: %v", err)}},
			IsError: true,
		}, nil
	}

	// Execute query with timeout
	ctx, cancel := context.WithTimeout(s.ctx, QueryTimeout)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, sqlQuery)
	if err != nil {
		return &CallToolResult{
			Content: []Content{{Type: "text", Text: fmt.Sprintf("Query error: %v", err)}},
			IsError: true,
		}, nil
	}
	defer rows.Close()

	// Get column names
	columns, err := rows.Columns()
	if err != nil {
		return &CallToolResult{
			Content: []Content{{Type: "text", Text: fmt.Sprintf("Failed to get columns: %v", err)}},
			IsError: true,
		}, nil
	}

	// Fetch rows with limit
	var results []map[string]any
	rowCount := 0
	for rows.Next() {
		if rowCount >= MaxResultRows {
			results = append(results, map[string]any{
				"_warning": fmt.Sprintf("Result truncated at %d rows", MaxResultRows),
			})
			break
		}

		values := make([]any, len(columns))
		valuePtrs := make([]any, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return &CallToolResult{
				Content: []Content{{Type: "text", Text: fmt.Sprintf("Failed to scan row %d: %v", rowCount+1, err)}},
				IsError: true,
			}, nil
		}

		row := make(map[string]any)
		for i, col := range columns {
			val := values[i]
			// Convert []byte to string for JSON serialization
			if b, ok := val.([]byte); ok {
				row[col] = string(b)
			} else {
				row[col] = val
			}
		}
		results = append(results, row)
		rowCount++
	}

	if err := rows.Err(); err != nil {
		return &CallToolResult{
			Content: []Content{{Type: "text", Text: fmt.Sprintf("Row iteration error: %v", err)}},
			IsError: true,
		}, nil
	}

	// Format result as JSON
	resultJSON, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return &CallToolResult{
			Content: []Content{{Type: "text", Text: fmt.Sprintf("Failed to marshal results: %v", err)}},
			IsError: true,
		}, nil
	}

	return &CallToolResult{
		Content: []Content{{Type: "text", Text: string(resultJSON)}},
	}, nil
}

func (s *MySQLMCPServer) handleListResources() (*ListResourcesResult, *Error) {
	if s.databaseName == "" {
		return &ListResourcesResult{Resources: []Resource{}}, nil
	}

	ctx, cancel := context.WithTimeout(s.ctx, QueryTimeout)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, `
		SELECT table_name
		FROM information_schema.tables
		WHERE table_schema = ?
	`, s.databaseName)
	if err != nil {
		return nil, &Error{
			Code:    InternalError,
			Message: fmt.Sprintf("Failed to list tables: %v", err),
		}
	}
	defer rows.Close()

	var resources []Resource
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			logError("Failed to scan table name: %v", err)
			continue
		}
		resources = append(resources, Resource{
			URI:      fmt.Sprintf("mysql://%s/%s/schema", s.databaseName, tableName),
			Name:     fmt.Sprintf("Schema for table '%s'", tableName),
			MimeType: "application/json",
		})
	}

	if err := rows.Err(); err != nil {
		return nil, &Error{
			Code:    InternalError,
			Message: fmt.Sprintf("Error iterating tables: %v", err),
		}
	}

	return &ListResourcesResult{Resources: resources}, nil
}

func (s *MySQLMCPServer) handleReadResource(params json.RawMessage) (*ReadResourceResult, *Error) {
	var readParams ReadResourceParams
	if err := json.Unmarshal(params, &readParams); err != nil {
		return nil, &Error{
			Code:    InvalidParams,
			Message: "Invalid parameters",
			Data:    err.Error(),
		}
	}

	// Parse URI: mysql://dbname/tablename/schema
	uri := readParams.URI
	if !strings.HasPrefix(uri, "mysql://") {
		return nil, &Error{
			Code:    InvalidParams,
			Message: "Invalid resource URI: must start with mysql://",
		}
	}

	parts := strings.Split(strings.TrimPrefix(uri, "mysql://"), "/")
	if len(parts) < 3 || parts[2] != "schema" {
		return nil, &Error{
			Code:    InvalidParams,
			Message: "Invalid resource URI format: expected mysql://dbname/tablename/schema",
		}
	}

	dbName := parts[0]
	tableName := parts[1]

	ctx, cancel := context.WithTimeout(s.ctx, QueryTimeout)
	defer cancel()

	// Get column information
	rows, err := s.db.QueryContext(ctx, `
		SELECT column_name, data_type, is_nullable, column_key, column_default, extra
		FROM information_schema.columns
		WHERE table_schema = ? AND table_name = ?
		ORDER BY ordinal_position
	`, dbName, tableName)
	if err != nil {
		return nil, &Error{
			Code:    InternalError,
			Message: fmt.Sprintf("Failed to get schema: %v", err),
		}
	}
	defer rows.Close()

	var columns []map[string]any
	for rows.Next() {
		var colName, dataType, isNullable, colKey string
		var colDefault, extra sql.NullString

		if err := rows.Scan(&colName, &dataType, &isNullable, &colKey, &colDefault, &extra); err != nil {
			logError("Failed to scan column info: %v", err)
			continue
		}

		col := map[string]any{
			"column_name": colName,
			"data_type":   dataType,
			"is_nullable": isNullable,
			"column_key":  colKey,
		}
		if colDefault.Valid {
			col["column_default"] = colDefault.String
		}
		if extra.Valid && extra.String != "" {
			col["extra"] = extra.String
		}
		columns = append(columns, col)
	}

	if err := rows.Err(); err != nil {
		return nil, &Error{
			Code:    InternalError,
			Message: fmt.Sprintf("Error reading schema: %v", err),
		}
	}

	schemaJSON, err := json.MarshalIndent(columns, "", "  ")
	if err != nil {
		return nil, &Error{
			Code:    InternalError,
			Message: fmt.Sprintf("Failed to marshal schema: %v", err),
		}
	}

	return &ReadResourceResult{
		Contents: []ResourceContent{
			{
				URI:      uri,
				MimeType: "application/json",
				Text:     string(schemaJSON),
			},
		},
	}, nil
}
