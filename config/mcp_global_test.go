package config_test

import (
	"testing"
	"time"

	"github.com/atsokha/mcplake/cache"
	"github.com/atsokha/mcplake/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const mcpGlobalBaseConfig = `
[server]
data_plane_addr = ":8080"

[oidc]
jwks_url = "https://auth.example.com/jwks.json"
issuer = "https://auth.example.com"
audience = "mcp-gateway"
`

func TestLoad_SchemaRefreshIntervalSet(t *testing.T) {
	cfg, err := config.Load(writeConfig(t, mcpGlobalBaseConfig+`
[mcp]
schema_refresh_interval = "15m"
`))
	require.NoError(t, err)

	assert.Equal(t, 15*time.Minute, cfg.SchemaRefreshInterval())
}

func TestLoad_SchemaRefreshIntervalAbsentIsZero(t *testing.T) {
	cfg, err := config.Load(writeConfig(t, mcpGlobalBaseConfig))
	require.NoError(t, err)

	assert.Zero(t, cfg.SchemaRefreshInterval())
}

func TestLoad_SchemaRefreshIntervalZeroString(t *testing.T) {
	cfg, err := config.Load(writeConfig(t, mcpGlobalBaseConfig+`
[mcp]
schema_refresh_interval = "0"
`))
	require.NoError(t, err)

	assert.Zero(t, cfg.SchemaRefreshInterval())
}

func TestLoad_SchemaRefreshIntervalNegativeIsRejected(t *testing.T) {
	_, err := config.Load(writeConfig(t, mcpGlobalBaseConfig+`
[mcp]
schema_refresh_interval = "-5m"
`))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "mcp.schema_refresh_interval must not be negative")
}

func TestLoad_SchemaRefreshIntervalInvalidStringIsRejected(t *testing.T) {
	_, err := config.Load(writeConfig(t, mcpGlobalBaseConfig+`
[mcp]
schema_refresh_interval = "soon"
`))

	require.Error(t, err)
}

// The default is on, and deliberately not zero: without a health check an
// http/sse MCP never recovers from a restart of the server behind it, which
// is not something an operator should have to find a config key to fix.
// See ADR-0019.
func TestLoad_HealthCheckIntervalAbsentIsTheDefault(t *testing.T) {
	cfg, err := config.Load(writeConfig(t, mcpGlobalBaseConfig))
	require.NoError(t, err)

	assert.Equal(t, cache.DefaultHealthCheckInterval, cfg.HealthCheckInterval())
}

func TestLoad_HealthCheckIntervalSet(t *testing.T) {
	cfg, err := config.Load(writeConfig(t, mcpGlobalBaseConfig+`
[mcp]
health_check_interval = "90s"
`))
	require.NoError(t, err)

	assert.Equal(t, 90*time.Second, cfg.HealthCheckInterval())
}

// An explicit zero is the only way to turn the loop off, and has to be
// distinguishable from an omitted key — which is why the field is a
// pointer, unlike schema_refresh_interval.
func TestLoad_HealthCheckIntervalExplicitZeroDisables(t *testing.T) {
	cfg, err := config.Load(writeConfig(t, mcpGlobalBaseConfig+`
[mcp]
health_check_interval = "0"
`))
	require.NoError(t, err)

	assert.Zero(t, cfg.HealthCheckInterval())
}

func TestLoad_HealthCheckIntervalNegativeIsRejected(t *testing.T) {
	_, err := config.Load(writeConfig(t, mcpGlobalBaseConfig+`
[mcp]
health_check_interval = "-1m"
`))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "mcp.health_check_interval must not be negative")
}
