package tools

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/dothanhlam/go-db-mcp/database"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// DestructiveKeywords defines SQL keywords that are not allowed in read-only queries.
var destructiveKeywords = []string{
	"DROP", "DELETE", "UPDATE", "INSERT", "TRUNCATE", "ALTER", "GRANT", "REVOKE",
}

// isReadonlyQuery checks if the query contains any destructive keywords.
// This is a basic string-matching validation. For a production system, an SQL parser would be better.
func isReadonlyQuery(query string) bool {
	upperQuery := strings.ToUpper(query)
	// Use regex to match whole words to prevent matching "DROP" inside "DROP_COLUMN" if it was a table name
	// but for simplicity and safety, we will be strict.
	for _, keyword := range destructiveKeywords {
		// Basic check with word boundaries
		matched, _ := regexp.MatchString(fmt.Sprintf(`\b%s\b`, keyword), upperQuery)
		if matched {
			return false
		}
	}
	return true
}

// RegisterReadonlyQueryTool registers the run_readonly_query tool with the MCP server.
func RegisterReadonlyQueryTool(s *server.MCPServer, manager *database.ConnectionManager) {
	tool := mcp.NewTool("run_readonly_query",
		mcp.WithDescription("Execute a read-only SELECT query. A LIMIT 50 will be automatically appended if not present. Destructive commands are blocked."),
		mcp.WithString("connection_id", mcp.Required(), mcp.Description("The ID of the database connection.")),
		mcp.WithString("sql_query", mcp.Required(), mcp.Description("The SELECT query to execute.")),
	)

	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		connectionID, err := request.RequireString("connection_id")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("connection_id must be a string: %v", err)), nil
		}

		sqlQuery, err := request.RequireString("sql_query")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("sql_query must be a string: %v", err)), nil
		}

		// Validation: Block destructive queries
		if !isReadonlyQuery(sqlQuery) {
			return mcp.NewToolResultError("Query rejected: contains destructive keywords. Only read-only queries are allowed."), nil
		}

		client, err := manager.GetClient(connectionID)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		resultJSON, err := client.RunReadonlyQuery(ctx, sqlQuery)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to execute query: %v", err)), nil
		}

		return mcp.NewToolResultText(resultJSON), nil
	})
}
