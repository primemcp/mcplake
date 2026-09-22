package config_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/atsokha/mcplake/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_ValidExampleConfig(t *testing.T) {
	cfg, err := config.Load("../config.example.toml")
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Equal(t, ":8080", cfg.Server.DataPlaneAddr)
	// 127.0.0.1, not ":8081" -- #160/PR167 defaulted the example to
	// loopback, since registering an MCP starts a process on this host.
	assert.Equal(t, "127.0.0.1:8081", cfg.Server.ControlPlaneAddr)

	assert.Equal(t, "https://auth.example.com/.well-known/jwks.json", cfg.OIDC.JWKSURL)
	assert.Equal(t, "https://auth.example.com", cfg.OIDC.Issuer)
	assert.Equal(t, "mcp-gateway", cfg.OIDC.Audience)

	require.Len(t, cfg.MCPs, 3)
	assert.Equal(t, "postgres-ro", cfg.MCPs[0].Name)
	assert.Equal(t, "stdio", cfg.MCPs[0].Type)
	assert.Equal(t, "mcp-server-postgres", cfg.MCPs[0].Command)
	assert.Equal(t, []string{
		"--connection-string", "postgresql://reader@localhost/mydb", "--read-only",
	}, cfg.MCPs[0].Arguments)

	require.Len(t, cfg.AccessPolicies, 3)
	dbReader := cfg.AccessPolicies[0]
	assert.Equal(t, "db-reader", dbReader.Name)
	require.Len(t, dbReader.Match, 1)
	assert.Equal(t, "$.role", dbReader.Match[0].Path)
	assert.Equal(t, "^db-reader$", dbReader.Match[0].Pattern)
	require.Len(t, dbReader.Grants, 1)
	assert.Equal(t, "postgres-ro", dbReader.Grants[0].MCP)
	assert.Equal(t, []string{"*"}, dbReader.Grants[0].Tools)

	require.Len(t, cfg.FilterPolicies, 2)
	hidePII := cfg.FilterPolicies[0]
	assert.Equal(t, "hide-pii-for-plain-users-get-user", hidePII.Name)
	assert.Equal(t, "postgres-ro", hidePII.MCP)
	assert.Equal(t, "get_user", hidePII.Tool)
	assert.Equal(t, []string{"$.hashed_password", "$.api_key", "$.internal_id"}, hidePII.DropFields)

	require.NoError(t, cfg.Validate())
}

func TestLoad_MissingFile(t *testing.T) {
	_, err := config.Load(filepath.Join(t.TempDir(), "does-not-exist.toml"))
	require.Error(t, err)
}

func TestLoad_MalformedTOML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.toml")
	require.NoError(t, os.WriteFile(path, []byte("data_plane_addr = \nthis is not valid toml ]["), 0o600))

	_, err := config.Load(path)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse")
}

func TestLoad_RejectsYAMLInput(t *testing.T) {
	// A leftover config.yaml pasted verbatim is not valid TOML and must be
	// rejected outright, not silently half-parsed.
	path := filepath.Join(t.TempDir(), "old.toml")
	body := "server:\n  data_plane_addr: \":8080\"\n"
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))

	_, err := config.Load(path)

	require.Error(t, err)
}

func TestLoad_FailsValidateForSemanticallyInvalidConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing-issuer.toml")
	body := `
[server]
data_plane_addr = ":8080"

[oidc]
jwks_url = "https://auth.example.com/.well-known/jwks.json"
audience = "mcp-gateway"
`
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))

	_, err := config.Load(path)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "issuer")
}

func TestLoad_ParsesJWKSCacheTTL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "with-ttl.toml")
	body := `
[server]
data_plane_addr = ":8080"

[oidc]
jwks_url = "https://auth.example.com/.well-known/jwks.json"
issuer = "https://auth.example.com"
audience = "mcp-gateway"
jwks_cache_ttl = "30m"
`
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))

	cfg, err := config.Load(path)

	require.NoError(t, err)
	assert.Equal(t, 30*time.Minute, cfg.OIDC.JWKSCacheTTL.Duration)
}

func TestLoad_JWKSCacheTTLDefaultsToZeroWhenUnset(t *testing.T) {
	path := filepath.Join(t.TempDir(), "no-ttl.toml")
	body := `
[server]
data_plane_addr = ":8080"

[oidc]
jwks_url = "https://auth.example.com/.well-known/jwks.json"
issuer = "https://auth.example.com"
audience = "mcp-gateway"
`
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))

	cfg, err := config.Load(path)

	require.NoError(t, err)
	assert.Zero(t, cfg.OIDC.JWKSCacheTTL.Duration)
}

func TestLoad_RejectsInvalidJWKSCacheTTL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad-ttl.toml")
	body := `
[server]
data_plane_addr = ":8080"

[oidc]
jwks_url = "https://auth.example.com/.well-known/jwks.json"
issuer = "https://auth.example.com"
audience = "mcp-gateway"
jwks_cache_ttl = "not-a-duration"
`
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))

	_, err := config.Load(path)

	require.Error(t, err)
}
