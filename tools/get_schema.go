package tools

import (
	"context"
	"fmt"

	"github.com/dothanhlam/go-db-mcp/database"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// RegisterGetSchemaTool registers the get_schema tool with the MCP server.
func RegisterGetSchemaTool(s *server.MCPServer, manager *database.ConnectionManager) {
	tool := mcp.NewTool("get_schema",
		mcp.WithDescription("Extract schema or metadata for a specific table."),
		mcp.WithString("connection_id", mcp.Required(), mcp.Description("The ID of the database connection.")),
		mcp.WithString("table_name", mcp.Required(), mcp.Description("The name of the table to inspect.")),
	)

	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		connectionID, err := request.RequireString("connection_id")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("connection_id must be a string: %v", err)), nil
		}

		tableName, err := request.RequireString("table_name")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("table_name must be a string: %v", err)), nil
		}

		client, err := manager.GetClient(connectionID)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		schemaJSON, err := client.GetSchema(ctx, tableName)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to get schema for table %s: %v", tableName, err)), nil
		}

		return mcp.NewToolResultText(schemaJSON), nil
	})
}
