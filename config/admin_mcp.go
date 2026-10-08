package config

// AdminMCPEnabled reports whether the in-process MCP control server should
// be mounted on the control-plane listener.
func (c *Config) AdminMCPEnabled() bool {
	return c.AdminMCP.enabled()
}

// AdminMCPPath is the path the MCP control server mounts at, defaulting to
// DefaultAdminMCPPath. Config.Validate guarantees it is under "/admin/".
func (c *Config) AdminMCPPath() string {
	return c.AdminMCP.path()
}
