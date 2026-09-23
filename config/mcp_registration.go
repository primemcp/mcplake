package config

import (
	"fmt"
	"strings"

	"github.com/atsokha/mcplake/cache"
	"github.com/atsokha/mcplake/mcp"
)

// defaultMCPTransport matches the documented default for MCPConfig.Type in
// docs/CONFIG.md.
const defaultMCPTransport = cache.TransportStdio

// MCPRegistrations converts every MCPs entry into a cache.MCPRegistration,
// ready to pass to Registry.RegisterAll at startup (ticket #19). This is the
// only place config-shaped MCP entries become cache's domain type — cache
// itself has no knowledge of the YAML config shape.
func (c *Config) MCPRegistrations() []cache.MCPRegistration {
	regs := make([]cache.MCPRegistration, 0, len(c.MCPs))
	for _, m := range c.MCPs {
		transport := m.Type
		if transport == "" {
			transport = defaultMCPTransport
		}
		regs = append(regs, cache.MCPRegistration{
			Name:      m.Name,
			Transport: transport,
			Connect: cache.ConnectConfig{
				Command:   m.Command,
				Arguments: m.Arguments,
				Env:       m.Env,
				URL:       m.URL,
			},
			Enabled: enabledOrDefault(m.Enabled),
		})
	}
	return regs
}

// validateMCP checks one `[[mcps]]` entry is complete and usable for the
// transport it names. Before ADR-0017 this array was not validated at all:
// a missing command surfaced as a registration failure at boot, logged and
// skipped, leaving an "unreachable" MCP the operator had to work backwards
// from. With a URL now in play that gets worse -- a plaintext endpoint on a
// remote host is a confidentiality problem worth refusing at startup, not
// at first tool call.
//
// The rules themselves are mcp's, not restated here: mcp.NewClient enforces
// exactly the same ones on every other write path (POST /admin/mcps, the
// ADR-0011 control server). This runs them early, so an operator gets a
// startup error naming the offending entry rather than a warning buried in
// the boot log.
func validateMCP(m MCPConfig) error {
	if m.Name == "" {
		return fmt.Errorf("name is required")
	}
	transport := m.Type
	if transport == "" {
		transport = defaultMCPTransport
	}
	switch transport {
	case cache.TransportStdio:
		if m.Command == "" {
			return fmt.Errorf("command is required for type = %q", transport)
		}
		if m.URL != "" {
			return fmt.Errorf("url is not used by type = %q; did you mean type = %q?",
				transport, cache.TransportHTTP)
		}
	case cache.TransportHTTP, cache.TransportSSE:
		if m.URL == "" {
			return fmt.Errorf("url is required for type = %q", transport)
		}
		if err := mcp.ValidateEndpointURL(m.URL); err != nil {
			return fmt.Errorf("url: %w", err)
		}
		if m.Command != "" {
			return fmt.Errorf("command is not used by type = %q (the gateway does not start a remote MCP)", transport)
		}
		if len(m.Env) > 0 {
			return fmt.Errorf("env is not used by type = %q (the gateway does not start a remote MCP)", transport)
		}
	default:
		return fmt.Errorf("unsupported type %q (supported: %s)",
			m.Type, strings.Join(mcp.SupportedTransports(), ", "))
	}
	return nil
}

// namedSuffix renders an entry's name for an error message, or nothing at
// all when the entry is the one that has no name to report.
func namedSuffix(name string) string {
	if name == "" {
		return ""
	}
	return fmt.Sprintf(" (%s)", name)
}
