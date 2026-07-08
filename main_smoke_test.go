package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dothanhlam/go-db-mcp/database"
	"github.com/dothanhlam/go-db-mcp/tools"
	"github.com/mark3labs/mcp-go/server"
)

// buildTestMux mirrors the transport wiring in onReady so we can exercise the
// HTTP endpoints without the systray/GUI main loop.
func buildTestMux(t *testing.T) *http.ServeMux {
	t.Helper()
	mgr, err := database.NewConnectionManager(nil)
	if err != nil {
		t.Fatal(err)
	}
	s := server.NewMCPServer("go-db-mcp", "test")
	tools.RegisterConfigureConnectionTool(s, mgr)
	tools.RegisterListTablesTool(s, mgr)
	tools.RegisterGetSchemaTool(s, mgr)
	tools.RegisterReadonlyQueryTool(s, mgr)

	mux := http.NewServeMux()
	mux.Handle("/mcp", server.NewStreamableHTTPServer(s))
	mux.Handle("/sse", server.NewSSEServer(s))
	return mux
}

func TestStreamableHTTPInitialize(t *testing.T) {
	ts := httptest.NewServer(buildTestMux(t))
	defer ts.Close()

	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request to /mcp failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from /mcp, got %d", resp.StatusCode)
	}
	out, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(out), "serverInfo") {
		t.Fatalf("expected initialize result with serverInfo, got: %s", out)
	}
}
