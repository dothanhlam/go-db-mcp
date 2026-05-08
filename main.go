package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/dothanhlam/go-db-mcp/database"
	"github.com/dothanhlam/go-db-mcp/tools"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func main() {
	ctx := context.Background()

	// 1. Initialize the ConnectionManager
	manager, err := database.NewConnectionManager(ctx)
	if err != nil {
		log.Fatalf("Failed to initialize connection manager: %v", err)
	}

	availableConnections := manager.GetAvailableConnections()
	if len(availableConnections) == 0 {
		log.Println("Warning: No database connections configured. Check your environment variables (POSTGRES_DSN, MYSQL_DSN).")
	}

	// 2. Initialize the mcp-go server
	s := server.NewMCPServer(
		"go-db-mcp",
		"1.0.0",
		server.WithLogging(),
	)

	// Expose available connections as resources if needed in the future,
	// but for now, we just log them.

	// 3. Register the 3 tools
	tools.RegisterListTablesTool(s, manager)
	tools.RegisterGetSchemaTool(s, manager)
	tools.RegisterReadonlyQueryTool(s, manager)

	// Add an informational tool to get configured connections
	registerInfoTool(s, manager)

	// 4. Start the server using STDIO
	// NOTE: We do not log to stdout here, as it would corrupt the JSON-RPC stream!
	// fmt.Println("Starting Multi-Database MCP Server over STDIO...")
	
	if err := server.ServeStdio(s); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

// registerInfoTool is a helper tool to let the client know which connections are available.
func registerInfoTool(s *server.MCPServer, manager *database.ConnectionManager) {
	tool := mcp.NewTool("list_connections",
		mcp.WithDescription("Get a list of available database connection IDs."),
	)
	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		conns := manager.GetAvailableConnections()
		if len(conns) == 0 {
			return mcp.NewToolResultText("No database connections configured."), nil
		}
		
		result := fmt.Sprintf("Available connections:\n- %s", strings.Join(conns, "\n- "))
		return mcp.NewToolResultText(result), nil
	})
}
