package config

import (
	"time"

	"github.com/atsokha/mcplake/cache"
)

// SchemaRefreshInterval is how often the gateway re-discovers every active
// MCP's tool schema. Zero means periodic refresh is disabled (the default).
// Config.Validate rejects a negative value.
func (c *Config) SchemaRefreshInterval() time.Duration {
	return c.MCP.SchemaRefreshInterval.Duration
}

// HealthCheckInterval is how often the gateway pings every registered MCP
// and reconnects the ones that have stopped answering (ADR-0019).
//
// Unlike SchemaRefreshInterval, an *omitted* key is not "off": it means
// cache.DefaultHealthCheckInterval. Turning the loop off takes an explicit
// `health_check_interval = "0"`, which is why the field is a pointer —
// there is no other way to tell "the operator said zero" from "the operator
// said nothing". A gateway that silently never recovers from a downstream
// restart is not a sensible default; a gateway whose schemas go stale is.
//
// Config.Validate rejects a negative value.
func (c *Config) HealthCheckInterval() time.Duration {
	if c.MCP.HealthCheckInterval == nil {
		return cache.DefaultHealthCheckInterval
	}
	return c.MCP.HealthCheckInterval.Duration
}
