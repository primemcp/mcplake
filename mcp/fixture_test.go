package mcp_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// helperProcessMarker is the sentinel trailing argument that tells
// TestHelperMCPServerProcess to actually run as a fixture MCP server instead
// of being skipped as a no-op test. This is the standard Go
// "re-exec the test binary as a subprocess" pattern (as used by
// os/exec_test.go), which avoids needing a second compiled binary or a
// `go run` invocation for the test-fixture MCP server the ticket calls for.
const helperProcessMarker = "MCPLAKE_MCP_FIXTURE_SERVER"

type echoArgs struct {
	Message string `json:"message" jsonschema:"the message to echo back"`
}

type echoResult struct {
	Message string `json:"message"`
}

// TestHelperMCPServerProcess is not a real test: run directly (without the
// marker argument) it's a no-op. The mcp.Client tests re-exec the test
// binary with -test.run pinned to this function plus the marker argument,
// making it act as a minimal fixture MCP server over stdio.
func TestHelperMCPServerProcess(t *testing.T) {
	if len(os.Args) == 0 || os.Args[len(os.Args)-1] != helperProcessMarker {
		t.Skip("not invoked as the MCP fixture server helper process")
	}

	server := sdk.NewServer(&sdk.Implementation{Name: "mcplake-fixture", Version: "0.1.0"}, nil)
	sdk.AddTool(server, &sdk.Tool{
		Name:        "echo",
		Description: "echoes the given message back",
	}, func(_ context.Context, _ *sdk.CallToolRequest, args echoArgs) (*sdk.CallToolResult, echoResult, error) {
		return nil, echoResult{Message: args.Message}, nil
	})

	if err := server.Run(context.Background(), &sdk.StdioTransport{}); err != nil {
		fmt.Fprintln(os.Stderr, "fixture server exited:", err)
		os.Exit(1)
	}
}
