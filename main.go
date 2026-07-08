package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
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

// manager holds the active connections so it can be shut down cleanly on exit.
var manager *database.ConnectionManager

func main() {
	// Headless mode (Docker, servers, CI): run the MCP server directly, with no
	// system tray — so no display, GTK session, D-Bus, or Xvfb is required.
	if isHeadless() {
		log.Println("Running in headless mode (no system tray)")
		if err := runServer(); err != nil {
			log.Fatalf("server error: %v", err)
		}
		return
	}

	systray.Run(onReady, onExit)
}

// isHeadless reports whether the tray should be skipped.
func isHeadless() bool {
	switch strings.ToLower(os.Getenv("HEADLESS")) {
	case "1", "true", "yes":
		return true
	}
	return false
}

func onReady() {
	systray.SetTemplateIcon(icon.Data, icon.Data)
	systray.SetTooltip("Database MCP Server")

	mAbout := systray.AddMenuItem("About", "About DB MCP")
	mQuit := systray.AddMenuItem("Quit", "Quit the app")

	mAbout.Click(func() {
		script := `display alert "DB MCP Server" message "A Multi-Database Model Context Protocol (MCP) Server.\n\nConfigure your databases dynamically via the 'configure_connection' tool." as informational`
		cmd := exec.Command("osascript", "-e", script)
		_ = cmd.Run()
	})

	mQuit.Click(func() {
		systray.Quit()
	})

	// Run the MCP server in a background goroutine so the tray stays responsive.
	go func() {
		if err := runServer(); err != nil {
			log.Printf("server error: %v", err)
			systray.Quit()
		}
	}()
}

// runServer wires up the MCP server and blocks serving HTTP until it stops.
func runServer() error {
	ctx := context.Background()

	// 1. Initialize the ConnectionManager
	var err error
	manager, err = database.NewConnectionManager(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize connection manager: %w", err)
	}

	if len(manager.GetAvailableConnections()) == 0 {
		log.Println("Note: No database connections configured yet. Use the 'configure_connection' tool to add one.")
	}

	// 2. Initialize the mcp-go server
	s := server.NewMCPServer(
		"go-db-mcp",
		"1.0.0",
		server.WithLogging(),
	)

	// 3. Register the tools
	tools.RegisterConfigureConnectionTool(s, manager)
	tools.RegisterListTablesTool(s, manager)
	tools.RegisterGetSchemaTool(s, manager)
	tools.RegisterReadonlyQueryTool(s, manager)
	registerInfoTool(s, manager)

	// 4. Resolve address
	port := os.Getenv("PORT")
	if port == "" {
		port = os.Getenv("MCP_PORT")
	}
	if port == "" {
		port = "6969"
	}

	// Headless deployments (Docker) need to be reachable, so bind all interfaces
	// there; otherwise bind loopback so the open-ended configure_connection tool
	// isn't exposed to the network. Override explicitly with HOST.
	host := os.Getenv("HOST")
	if host == "" {
		if isHeadless() {
			host = "0.0.0.0"
		} else {
			host = "127.0.0.1"
		}
	}
	addr := host + ":" + port

	// 5. Serve both MCP transports on the same port so modern and legacy clients
	// can connect:
	//   - Streamable HTTP: http://<host>:<port>/mcp   (current spec; Cursor)
	//   - SSE (legacy):    http://<host>:<port>/sse
	mux := http.NewServeMux()
	mux.Handle("/mcp", server.NewStreamableHTTPServer(s))
	sseServer := server.NewSSEServer(s)
	mux.Handle("/sse", sseServer)
	mux.Handle("/message", sseServer)

	httpServer := &http.Server{Addr: addr, Handler: mux}

	log.Printf("Starting MCP server on %s (streamable-http: /mcp, sse: /sse)", addr)
	if !isHeadless() {
		systray.SetTooltip(fmt.Sprintf("Database MCP Server (:%s)", port))
	}

	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("http server error: %w", err)
	}
	return nil
}

func onExit() {
	if manager != nil {
		manager.CloseAll()
	}
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
