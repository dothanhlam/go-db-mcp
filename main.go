package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"

	"github.com/dothanhlam/go-db-mcp/database"
	"github.com/dothanhlam/go-db-mcp/icon"
	"github.com/dothanhlam/go-db-mcp/tools"
	"github.com/energye/systray"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func main() {
	systray.Run(onReady, onExit)
}

func onReady() {
	systray.SetTemplateIcon(icon.Data, icon.Data)
	systray.SetTooltip("Database MCP Server")


	mAbout := systray.AddMenuItem("About", "About DB MCP")
	mQuit := systray.AddMenuItem("Quit", "Quit the app")

	mAbout.Click(func() {
		script := `display alert "DB MCP Server" message "A Multi-Database Model Context Protocol (MCP) Server.\n\nRunning as HTTP/SSE server on port 6969 by default." as informational`
		cmd := exec.Command("osascript", "-e", script)
		_ = cmd.Run()
	})

	mQuit.Click(func() {
		systray.Quit()
	})

	// Run the MCP server in a background goroutine
	go func() {
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

		// 3. Register the 3 tools
		tools.RegisterListTablesTool(s, manager)
		tools.RegisterGetSchemaTool(s, manager)
		tools.RegisterReadonlyQueryTool(s, manager)

		// Add an informational tool to get configured connections
		registerInfoTool(s, manager)

		// 4. Start the server
		port := os.Getenv("PORT")
		if port == "" {
			port = os.Getenv("MCP_PORT")
		}
		if port == "" {
			port = "6969"
		}

		// Start as SSE server
		sse := server.NewSSEServer(s)

		log.Printf("Starting MCP SSE server on :%s", port)
		// Update tooltip to show port
		systray.SetTooltip(fmt.Sprintf("Database MCP Server (SSE :%s)", port))

		if err := sse.Start(":" + port); err != nil {
			log.Printf("SSE server error: %v", err)
			systray.Quit()
		}
	}()
}

func onExit() {
	os.Exit(0)
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
