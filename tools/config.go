package tools

import (
	"context"
	"fmt"

	"github.com/dothanhlam/go-db-mcp/database"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func RegisterConfigureConnectionTool(s *server.MCPServer, manager *database.ConnectionManager) {
	tool := mcp.NewTool("configure_connection",
		mcp.WithDescription("Dynamically configure a new database connection at runtime."),
		mcp.WithString("connection_id", mcp.Required(), mcp.Description("A unique identifier for this connection (e.g., 'pg_main')")),
		mcp.WithString("db_type", mcp.Required(), mcp.Description("The type of database: 'postgres', 'mysql', or 'sqlite'")),
		mcp.WithString("dsn", mcp.Required(), mcp.Description("The connection string (DSN)")),
	)

	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := request.RequireString("connection_id")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("connection_id is required: %v", err)), nil
		}
		dbType, err := request.RequireString("db_type")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("db_type is required: %v", err)), nil
		}
		dsn, err := request.RequireString("dsn")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("dsn is required: %v", err)), nil
		}

		err = manager.AddConnection(ctx, id, dbType, dsn)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to add connection: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Successfully configured %s connection: %s", dbType, id)), nil
	})
}
