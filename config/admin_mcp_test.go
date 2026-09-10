package config_test

import (
	"testing"

	"github.com/atsokha/mcplake/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const adminMCPBaseConfig = `
[server]
data_plane_addr = ":8080"

[oidc]
jwks_url = "https://auth.example.com/jwks.json"
issuer = "https://auth.example.com"
audience = "mcp-gateway"
`

func TestLoad_AdminMCPDefaultsOffWithDefaultPath(t *testing.T) {
	cfg, err := config.Load(writeConfig(t, adminMCPBaseConfig))
	require.NoError(t, err)

	assert.False(t, cfg.AdminMCPEnabled())
	assert.Equal(t, config.DefaultAdminMCPPath, cfg.AdminMCPPath())
}

func TestLoad_AdminMCPEnabledWithCustomPath(t *testing.T) {
	body := adminMCPBaseConfig + `
[admin_mcp]
enabled = true
path = "/admin/control/mcp"
`
	cfg, err := config.Load(writeConfig(t, body))
	require.NoError(t, err)

	assert.True(t, cfg.AdminMCPEnabled())
	assert.Equal(t, "/admin/control/mcp", cfg.AdminMCPPath())
}

func TestLoad_AdminMCPRejectsPathOutsideAdmin(t *testing.T) {
	body := adminMCPBaseConfig + `
[admin_mcp]
enabled = true
path = "/mcp"
`
	_, err := config.Load(writeConfig(t, body))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "admin_mcp.path must be under /admin/")
}

func TestLoad_AdminMCPExplicitDisabled(t *testing.T) {
	body := adminMCPBaseConfig + `
[admin_mcp]
enabled = false
path = "/admin/mcp"
`
	cfg, err := config.Load(writeConfig(t, body))
	require.NoError(t, err)

	assert.False(t, cfg.AdminMCPEnabled())
}
