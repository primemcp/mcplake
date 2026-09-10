package config

import "time"

// SchemaRefreshInterval is how often the gateway re-discovers every active
// MCP's tool schema. Zero means periodic refresh is disabled (the default).
// Config.Validate rejects a negative value.
func (c *Config) SchemaRefreshInterval() time.Duration {
	return c.MCP.SchemaRefreshInterval.Duration
}
