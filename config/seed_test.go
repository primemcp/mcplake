package config_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/atsokha/mcplake/cache"
	"github.com/atsokha/mcplake/config"
	"github.com/atsokha/mcplake/persistence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type seedRepos struct {
	mcp    *persistence.MCPRegistrationRepo
	access *persistence.AccessPolicyRepo
	filter *persistence.FilterPolicyRepo
}

func newSeedRepos(t *testing.T) seedRepos {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := persistence.Open(persistence.Config{DSN: path})
	require.NoError(t, err)
	return seedRepos{
		mcp:    persistence.NewMCPRegistrationRepo(db),
		access: persistence.NewAccessPolicyRepo(db),
		filter: persistence.NewFilterPolicyRepo(db),
	}
}

func TestSeed_ExampleConfigProducesMatchingRows(t *testing.T) {
	cfg, err := config.Load("../config.example.yaml")
	require.NoError(t, err)
	repos := newSeedRepos(t)
	ctx := context.Background()

	require.NoError(t, cfg.Seed(ctx, repos.mcp, repos.access, repos.filter))

	mcps, err := repos.mcp.List(ctx)
	require.NoError(t, err)
	assert.Len(t, mcps, len(cfg.MCPs))

	accessPolicies, err := repos.access.List(ctx)
	require.NoError(t, err)
	assert.Len(t, accessPolicies, len(cfg.AccessPolicies))

	filterPolicies, err := repos.filter.List(ctx)
	require.NoError(t, err)
	assert.Len(t, filterPolicies, len(cfg.FilterPolicies))

	dbReader, ok, err := repos.access.Get(ctx, "db-reader")
	require.NoError(t, err)
	require.True(t, ok)
	matched, err := dbReader.Match.Matches([]byte(`{"role":"db-reader"}`))
	require.NoError(t, err)
	assert.True(t, matched)
}

func TestSeed_SeededMCPStartsInConnectingStatus(t *testing.T) {
	cfg, err := config.Load("../config.example.yaml")
	require.NoError(t, err)
	repos := newSeedRepos(t)
	ctx := context.Background()
	require.NoError(t, cfg.Seed(ctx, repos.mcp, repos.access, repos.filter))

	reg, ok, err := repos.mcp.Get(ctx, "postgres-ro")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, cache.StatusConnecting, reg.Status)
}

func TestSeed_IsIdempotentNoDuplicateRows(t *testing.T) {
	cfg, err := config.Load("../config.example.yaml")
	require.NoError(t, err)
	repos := newSeedRepos(t)
	ctx := context.Background()

	require.NoError(t, cfg.Seed(ctx, repos.mcp, repos.access, repos.filter))
	require.NoError(t, cfg.Seed(ctx, repos.mcp, repos.access, repos.filter))

	mcps, err := repos.mcp.List(ctx)
	require.NoError(t, err)
	assert.Len(t, mcps, len(cfg.MCPs))

	accessPolicies, err := repos.access.List(ctx)
	require.NoError(t, err)
	assert.Len(t, accessPolicies, len(cfg.AccessPolicies))
}

func TestSeed_ReSeedReflectsChangedConfig(t *testing.T) {
	cfg, err := config.Load("../config.example.yaml")
	require.NoError(t, err)
	repos := newSeedRepos(t)
	ctx := context.Background()
	require.NoError(t, cfg.Seed(ctx, repos.mcp, repos.access, repos.filter))

	for i := range cfg.AccessPolicies {
		if cfg.AccessPolicies[i].Name == "db-reader" {
			cfg.AccessPolicies[i].Grants = []config.GrantConfig{{MCP: "postgres-rw", Tools: []string{"get_user"}}}
		}
	}
	require.NoError(t, cfg.Seed(ctx, repos.mcp, repos.access, repos.filter))

	dbReader, ok, err := repos.access.Get(ctx, "db-reader")
	require.NoError(t, err)
	require.True(t, ok)
	require.Len(t, dbReader.Grants, 1)
	assert.Equal(t, "postgres-rw", dbReader.Grants[0].MCP)
}

func TestSeed_EmptyConfigSeedsNothing(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{DataPlaneAddr: ":8080"},
		OIDC: config.OIDCConfig{
			JWKSURL: "https://auth.example.com/.well-known/jwks.json", Issuer: "https://auth.example.com", Audience: "mcp-gateway",
		},
	}
	repos := newSeedRepos(t)
	ctx := context.Background()

	require.NoError(t, cfg.Seed(ctx, repos.mcp, repos.access, repos.filter))

	mcps, err := repos.mcp.List(ctx)
	require.NoError(t, err)
	assert.Empty(t, mcps)
}
