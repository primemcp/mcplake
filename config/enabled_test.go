package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/atsokha/mcplake/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeConfig writes body to a temp file and returns its path.
func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

const enabledBaseConfig = `
server:
  data_plane_addr: ":8080"
oidc:
  jwks_url: "https://auth.example.com/jwks.json"
  issuer: "https://auth.example.com"
  audience: "mcp-gateway"
`

func TestLoad_EnabledDefaultsToTrueWhenKeyOmitted(t *testing.T) {
	path := writeConfig(t, enabledBaseConfig+`
mcps:
  - name: postgres-ro
    command: mcp-server-postgres
access_policies:
  - name: db-reader
    match:
      - path: "$.role"
        pattern: "^db-reader$"
    grants:
      - mcp: postgres-ro
        tools: ["*"]
filter_policies:
  - name: hide-pii
    match:
      - path: "$.role"
        pattern: "^user$"
    mcp: postgres-ro
    tool: get_user
    drop_fields: ["$.ssn"]
`)

	cfg, err := config.Load(path)
	require.NoError(t, err)

	require.Nil(t, cfg.MCPs[0].Enabled)
	require.Nil(t, cfg.AccessPolicies[0].Enabled)
	require.Nil(t, cfg.FilterPolicies[0].Enabled)

	regs := cfg.MCPRegistrations()
	assert.True(t, regs[0].Enabled, "an omitted enabled key must default the registration to enabled")

	access, err := cfg.RouterAccessPolicies()
	require.NoError(t, err)
	assert.True(t, access[0].Enabled, "an omitted enabled key must default the access policy to enabled")

	filter, err := cfg.RouterFilterPolicies()
	require.NoError(t, err)
	assert.True(t, filter[0].Enabled, "an omitted enabled key must default the filter policy to enabled")
}

func TestLoad_ExplicitEnabledFalseDisablesEntry(t *testing.T) {
	path := writeConfig(t, enabledBaseConfig+`
mcps:
  - name: postgres-ro
    command: mcp-server-postgres
    enabled: false
access_policies:
  - name: db-reader
    enabled: false
    match:
      - path: "$.role"
        pattern: "^db-reader$"
    grants:
      - mcp: postgres-ro
        tools: ["*"]
filter_policies:
  - name: hide-pii
    enabled: false
    match:
      - path: "$.role"
        pattern: "^user$"
    mcp: postgres-ro
    tool: get_user
    drop_fields: ["$.ssn"]
`)

	cfg, err := config.Load(path)
	require.NoError(t, err)

	regs := cfg.MCPRegistrations()
	assert.False(t, regs[0].Enabled)

	access, err := cfg.RouterAccessPolicies()
	require.NoError(t, err)
	assert.False(t, access[0].Enabled)

	filter, err := cfg.RouterFilterPolicies()
	require.NoError(t, err)
	assert.False(t, filter[0].Enabled)
}

func TestLoad_ExplicitEnabledTruePassesThrough(t *testing.T) {
	path := writeConfig(t, enabledBaseConfig+`
mcps:
  - name: postgres-ro
    command: mcp-server-postgres
    enabled: true
`)

	cfg, err := config.Load(path)
	require.NoError(t, err)

	regs := cfg.MCPRegistrations()
	assert.True(t, regs[0].Enabled)
}
