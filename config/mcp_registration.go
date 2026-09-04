package config

import "github.com/atsokha/mcplake/cache"

// defaultMCPTransport matches the documented default for MCPConfig.Type in
// docs/CONFIG.md.
const defaultMCPTransport = "stdio"

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
			},
		})
	}
	return regs
}
