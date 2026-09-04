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
// its tools, and stores the result as StatusActive. This is the single
// write path used both by static config.yaml entries at startup (ticket
// #19) and, later, the admin API — see ADR-0003.
//
// Registering an existing name is make-before-break, per ADR-0003: the new
// client is built and its tools discovered *before* any shared state is
// touched. If that succeeds, the swap is one atomic map write (Registry.set)
// — a concurrent reader observes either the complete old registration or
// the complete new one, never a mix — and only then is the superseded
// client closed. If it fails, an existing registration (of any status) is
// left completely untouched; only a name with no existing entry gets
// recorded as StatusUnreachable, so operators can still see a first-attempt
// failure for an MCP that was never up.
func (r *Registry) Register(ctx context.Context, reg MCPRegistration) error {
	if reg.Name == "" {
		return fmt.Errorf("cache: registration name is required")
	}
	if reg.Transport != "stdio" {
		err := fmt.Errorf("cache: unsupported transport %q for %q (only stdio is implemented)", reg.Transport, reg.Name)
		r.setIfAbsent(unreachable(reg))
		return err
	}

	client, err := newMCPClient(ctx, mcp.Config{
		Command:   reg.Connect.Command,
		Arguments: reg.Connect.Arguments,
	})
	if err != nil {
		r.setIfAbsent(unreachable(reg))
		return fmt.Errorf("cache: connect to %q: %w", reg.Name, err)
	}

	tools, err := client.ListTools(ctx)
	if err != nil {
		_ = client.Close()
		r.setIfAbsent(unreachable(reg))
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

	previous, hadPrevious := r.set(MCPRegistration{
		Name:      reg.Name,
		Transport: reg.Transport,
		Connect:   reg.Connect,
		Status:    StatusActive,
		Tools:     toolMap,
		Client:    client,
	})
	if hadPrevious && previous.Client != nil {
		_ = previous.Client.Close()
	}
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
