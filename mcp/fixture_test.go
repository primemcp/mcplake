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

type getenvArgs struct {
	Name string `json:"name" jsonschema:"the environment variable to read from this process's own environment"`
}

type getenvResult struct {
	Value string `json:"value"`
	// Set distinguishes "the variable is absent" from "the variable is set
	// to the empty string" -- os.Getenv alone can't, and env-isolation
	// tests need exactly that distinction.
	Set bool `json:"set"`
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
	// Lets tests observe exactly what environment the real spawned
	// subprocess sees, rather than only what mcp.Config asked for --
	// proving isolation (or a leak) end to end instead of trusting the
	// caller's side of exec.Cmd construction.
	sdk.AddTool(server, &sdk.Tool{
		Name:        "getenv",
		Description: "reads a named environment variable from this process's own environment",
	}, func(_ context.Context, _ *sdk.CallToolRequest, args getenvArgs) (*sdk.CallToolResult, getenvResult, error) {
		value, set := os.LookupEnv(args.Name)
		return nil, getenvResult{Value: value, Set: set}, nil
	})

	if err := server.Run(context.Background(), &sdk.StdioTransport{}); err != nil {
		fmt.Fprintln(os.Stderr, "fixture server exited:", err)
		os.Exit(1)
	}
}
