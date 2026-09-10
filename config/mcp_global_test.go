package config_test

import (
	"testing"
	"time"

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
