package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/dothanhlam/go-db-mcp/database"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// RegisterListTablesTool registers the list_tables tool with the MCP server.
func RegisterListTablesTool(s *server.MCPServer, manager *database.ConnectionManager) {
	tool := mcp.NewTool("list_tables",
		mcp.WithDescription("Get a list of all tables in the specified database connection."),
		mcp.WithString("connection_id", mcp.Required(), mcp.Description("The ID of the database connection (e.g., 'pg_main', 'mysql_legacy').")),
	)

	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		connectionID, err := request.RequireString("connection_id")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("connection_id must be a string: %v", err)), nil
		}

		client, err := manager.GetClient(connectionID)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		tables, err := client.ListTables(ctx)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to list tables: %v", err)), nil
		}

		result := fmt.Sprintf("Tables in %s:\n%s", connectionID, strings.Join(tables, "\n"))
		return mcp.NewToolResultText(result), nil
	})
}
