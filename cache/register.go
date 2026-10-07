package cache

import (
	"context"
	"fmt"
	"strings"

	"github.com/primemcp/mcplake/mcp"
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
//
// reg.Transport selects how the MCP is reached: stdio spawns it as a
// subprocess of this gateway, http and sse connect to one running elsewhere
// (see ADR-0017). An empty Transport means stdio.
func (r *Registry) Register(ctx context.Context, reg MCPRegistration) error {
	if reg.Name == "" {
		return fmt.Errorf("cache: registration name is required")
	}
	if !mcp.TransportSupported(reg.Transport) {
		err := fmt.Errorf("cache: unsupported transport %q for %q (supported: %s)",
			reg.Transport, reg.Name, strings.Join(mcp.SupportedTransports(), ", "))
		r.setIfAbsent(unreachable(reg))
		return err
	}
	// An omitted transport means stdio. Settle that here, once, so the empty
	// string never reaches the store or an admin API response -- every
	// reader downstream sees a concrete transport name.
	if reg.Transport == "" {
		reg.Transport = mcp.TransportStdio
	}

	// mcp.NewClient owns the rest of the validation -- that Command is set
	// for stdio, that URL is set and passes ValidateEndpointURL for http and
	// sse. Doing it there rather than here keeps one answer for every write
	// path: config seeding, POST /admin/mcps and the ADR-0011 control server
	// all arrive at Register, and Register arrives here.
	client, err := newMCPClient(ctx, mcp.Config{
		Transport: reg.Transport,
		Command:   reg.Connect.Command,
		Arguments: reg.Connect.Arguments,
		Env:       reg.Connect.Env,
		URL:       reg.Connect.URL,
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

	toolMap := toolMapFrom(tools)

	previous, hadPrevious := r.set(MCPRegistration{
		Name:      reg.Name,
		Transport: reg.Transport,
		Connect:   reg.Connect,
		Status:    StatusActive,
		Tools:     toolMap,
		Client:    client,
		// Status/Tools/Client are recomputed here, but Enabled is operator
		// intent carried in from the caller (like Connect), not a fresh
		// value — so a registration seeded as disabled stays disabled
		// across a (re-)Register.
		Enabled: reg.Enabled,
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
		Enabled:   reg.Enabled,
	}
}
