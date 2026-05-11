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
	"github.com/getlantern/systray"
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

	go func() {
		for {
			select {
			case <-mAbout.ClickedCh:
				script := `display alert "DB MCP Server" message "A Multi-Database Model Context Protocol (MCP) Server.\n\nRunning in background." as informational`
				cmd := exec.Command("osascript", "-e", script)
				_ = cmd.Run()
			case <-mQuit.ClickedCh:
				systray.Quit()
			}
		}
	}()

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

		// 4. Start the server using STDIO
		if err := server.ServeStdio(s); err != nil {
			log.Printf("Server error: %v", err)
		}

		// If ServeStdio returns, it usually means stdin is closed. We should quit.
		systray.Quit()
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
