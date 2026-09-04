// Package mcp implements a client connection to a single downstream MCP
// server, built on the official github.com/modelcontextprotocol/go-sdk.
// See docs/architecture/decisions/0003-dynamic-mcp-registration-and-schema-discovery.md.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Config configures a stdio-transport connection to a downstream MCP.
type Config struct {
	Command   string
	Arguments []string
}

// ToolSchema describes one tool a downstream MCP advertises. It mirrors
// cache.ToolSchema; the two aren't the same type because mcp must not
// depend on cache (cache, the Registry, depends on mcp — not the reverse).
type ToolSchema struct {
	Name         string
	InputSchema  json.RawMessage
	OutputSchema json.RawMessage // nil when the MCP doesn't advertise one
}

// ToolResponse is a tool call's raw JSON-RPC result. What to do with it
// (which fields to keep, how to interpret Content vs. StructuredContent) is
// the Policy Engine and Response Filter's job (ADR-0004), not this
// package's — Raw is deliberately the whole marshaled result.
type ToolResponse struct {
	Raw     json.RawMessage
	IsError bool
}

// Client is a live connection to one downstream MCP server.
//
// Client must not be copied after first use.
type Client struct {
	session *sdk.ClientSession
}

// NewClient spawns cfg.Command as a subprocess and performs the MCP
// initialize handshake over stdio. The subprocess's lifetime is tied to ctx
// (via exec.CommandContext): callers that want a long-lived connection (as
// the Registry does once it holds a Client for a registration) must pass a
// context that outlives this call, not a short-lived request context.
func NewClient(ctx context.Context, cfg Config) (*Client, error) {
	if cfg.Command == "" {
		return nil, fmt.Errorf("mcp: Config.Command is required")
	}

	client := sdk.NewClient(&sdk.Implementation{Name: "mcplake-gateway", Version: "0.1.0"}, nil)
	transport := &sdk.CommandTransport{
		Command: exec.CommandContext(ctx, cfg.Command, cfg.Arguments...),
	}

	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, fmt.Errorf("mcp: connect to %q: %w", cfg.Command, err)
	}
	return &Client{session: session}, nil
}

// ListTools returns every tool the MCP currently advertises, transparently
// paging through the server's tools/list results.
func (c *Client) ListTools(ctx context.Context) ([]ToolSchema, error) {
	var schemas []ToolSchema
	for tool, err := range c.session.Tools(ctx, nil) {
		if err != nil {
			return nil, fmt.Errorf("mcp: list tools: %w", err)
		}

		input, err := json.Marshal(tool.InputSchema)
		if err != nil {
			return nil, fmt.Errorf("mcp: marshal input schema for tool %q: %w", tool.Name, err)
		}

		var output json.RawMessage
		if tool.OutputSchema != nil {
			output, err = json.Marshal(tool.OutputSchema)
			if err != nil {
				return nil, fmt.Errorf("mcp: marshal output schema for tool %q: %w", tool.Name, err)
			}
		}

		schemas = append(schemas, ToolSchema{
			Name:         tool.Name,
			InputSchema:  input,
			OutputSchema: output,
		})
	}
	return schemas, nil
}

// CallTool invokes tool with args and returns its raw result.
func (c *Client) CallTool(ctx context.Context, tool string, args map[string]any) (*ToolResponse, error) {
	result, err := c.session.CallTool(ctx, &sdk.CallToolParams{
		Name:      tool,
		Arguments: args,
	})
	if err != nil {
		return nil, fmt.Errorf("mcp: call tool %q: %w", tool, err)
	}

	raw, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("mcp: marshal result for tool %q: %w", tool, err)
	}
	return &ToolResponse{Raw: raw, IsError: result.IsError}, nil
}

// Close terminates the subprocess and releases the session. It is safe to
// call more than once.
func (c *Client) Close() error {
	return c.session.Close()
}
