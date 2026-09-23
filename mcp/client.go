// Package mcp implements a client connection to a single downstream MCP
// server, built on the official github.com/modelcontextprotocol/go-sdk.
// See docs/architecture/decisions/0003-dynamic-mcp-registration-and-schema-discovery.md.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Config describes how to reach one downstream MCP.
//
// Which fields matter depends on Transport: stdio uses Command/Arguments,
// http and sse use URL. NewClient rejects a Config that omits what its
// transport needs, rather than dialing something half-specified.
type Config struct {
	// Transport is one of the Transport* constants. Empty means
	// TransportStdio -- every caller predating the other two passed a
	// Command and nothing else, and they should keep working unchanged.
	Transport string
	// Command and Arguments are the subprocess to spawn, for stdio.
	Command   string
	Arguments []string
	// URL is the server's endpoint, for http and sse. It must satisfy
	// ValidateEndpointURL.
	URL string
}

// transport resolves Transport's zero value to stdio.
func (c Config) transport() string {
	if c.Transport == "" {
		return TransportStdio
	}
	return c.Transport
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

// NewClient connects to the MCP described by cfg and performs the
// initialize handshake.
//
// For stdio the subprocess's lifetime is tied to ctx (via
// exec.CommandContext): callers that want a long-lived connection (as the
// Registry does once it holds a Client for a registration) must pass a
// context that outlives this call, not a short-lived request context. The
// same applies to http and sse, where ctx bounds the session's underlying
// requests rather than a process.
func NewClient(ctx context.Context, cfg Config) (*Client, error) {
	transport, target, err := newTransport(ctx, cfg)
	if err != nil {
		return nil, err
	}

	client := sdk.NewClient(&sdk.Implementation{Name: "mcplake-gateway", Version: "0.1.0"}, nil)
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, fmt.Errorf("mcp: connect to %q: %w", target, err)
	}
	return &Client{session: session}, nil
}

// newTransport builds the sdk transport cfg names, and returns it with a
// human-readable target for error messages (the command, or the URL).
func newTransport(ctx context.Context, cfg Config) (sdk.Transport, string, error) {
	switch t := cfg.transport(); t {
	case TransportStdio:
		if cfg.Command == "" {
			return nil, "", fmt.Errorf("mcp: Config.Command is required for the %s transport", TransportStdio)
		}
		return &sdk.CommandTransport{
			Command: exec.CommandContext(ctx, cfg.Command, cfg.Arguments...),
		}, cfg.Command, nil

	case TransportHTTP:
		if err := validateURLFor(t, cfg.URL); err != nil {
			return nil, "", err
		}
		return &sdk.StreamableClientTransport{
			Endpoint:   cfg.URL,
			HTTPClient: httpTransportClient,
			// The standalone SSE stream is the transport's optional
			// second channel: a GET the client holds open for the
			// lifetime of the session so the server can push
			// notifications (tools/list_changed and friends) unprompted.
			//
			// This gateway consumes none of them. ADR-0013 settled on
			// polling tools/list on an interval instead, precisely so
			// schema freshness does not depend on a downstream
			// implementing notifications. Leaving the stream on would
			// therefore hold one connection open per registered MCP
			// forever, plus a reconnect loop behind it, and buy nothing.
			//
			// Turn this back on if the gateway ever starts reacting to
			// server-initiated messages -- at which point ADR-0013's
			// polling is what should be reconsidered, not this flag on
			// its own.
			DisableStandaloneSSE: true,
		}, cfg.URL, nil

	case TransportSSE:
		if err := validateURLFor(t, cfg.URL); err != nil {
			return nil, "", err
		}
		return &sdk.SSEClientTransport{
			Endpoint:   cfg.URL,
			HTTPClient: httpTransportClient,
		}, cfg.URL, nil

	default:
		return nil, "", fmt.Errorf("mcp: unsupported transport %q (supported: %s)",
			t, strings.Join(SupportedTransports(), ", "))
	}
}

func validateURLFor(transport, raw string) error {
	if raw == "" {
		return fmt.Errorf("mcp: Config.URL is required for the %s transport", transport)
	}
	if err := ValidateEndpointURL(raw); err != nil {
		return fmt.Errorf("mcp: Config.URL: %w", err)
	}
	return nil
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
