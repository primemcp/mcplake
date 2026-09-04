package cache

import (
	"context"
	"fmt"

	"github.com/atsokha/mcplake/mcp"
)

// newMCPClient constructs a live connection to a downstream MCP. It's a
// package-level var, overridable in tests, so Register can be exercised
// against a fake MCPClient without spawning a real subprocess.
var newMCPClient = func(ctx context.Context, cfg mcp.Config) (MCPClient, error) {
	return mcp.NewClient(ctx, cfg)
}

// Register connects to the MCP described by reg (using reg.Name, reg.
// Transport, and reg.Connect only — any Status/Tools/Client on reg are
// ignored, since Register always computes fresh values for them), discovers
// its tools, and stores the result as StatusActive. On failure, reg is
// stored as StatusUnreachable and the error is returned; the Registry is
// left in a consistent state either way (each outcome is written as one
// complete replacement of the entry, never a partial one).
//
// This is the single write path used both by static config.yaml entries at
// startup (ticket #19) and, later, the admin API — see ADR-0003.
//
// Re-registering an existing, already-active name is refined in #18
// (make-before-break: connect and confirm the new client before touching
// shared state, then close the old one). This ticket doesn't implement that
// yet: registering an existing name here simply overwrites it, which would
// leak the prior registration's client connection if one existed.
func (r *Registry) Register(ctx context.Context, reg MCPRegistration) error {
	if reg.Name == "" {
		return fmt.Errorf("cache: registration name is required")
	}
	if reg.Transport != "stdio" {
		err := fmt.Errorf("cache: unsupported transport %q for %q (only stdio is implemented)", reg.Transport, reg.Name)
		r.set(unreachable(reg))
		return err
	}

	client, err := newMCPClient(ctx, mcp.Config{
		Command:   reg.Connect.Command,
		Arguments: reg.Connect.Arguments,
	})
	if err != nil {
		r.set(unreachable(reg))
		return fmt.Errorf("cache: connect to %q: %w", reg.Name, err)
	}

	tools, err := client.ListTools(ctx)
	if err != nil {
		_ = client.Close()
		r.set(unreachable(reg))
		return fmt.Errorf("cache: discover tools for %q: %w", reg.Name, err)
	}

	toolMap := make(map[string]ToolSchema, len(tools))
	for _, t := range tools {
		toolMap[t.Name] = ToolSchema{
			Name:         t.Name,
			InputSchema:  t.InputSchema,
			OutputSchema: t.OutputSchema,
		}
	}

	r.set(MCPRegistration{
		Name:      reg.Name,
		Transport: reg.Transport,
		Connect:   reg.Connect,
		Status:    StatusActive,
		Tools:     toolMap,
		Client:    client,
	})
	return nil
}

// unreachable returns the StatusUnreachable form of reg: identity fields
// only, no stale tools or client.
func unreachable(reg MCPRegistration) MCPRegistration {
	return MCPRegistration{
		Name:      reg.Name,
		Transport: reg.Transport,
		Connect:   reg.Connect,
		Status:    StatusUnreachable,
	}
}
