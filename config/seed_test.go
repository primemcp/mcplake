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
	markers *persistence.SeedMarkerRepo
	mcp     *persistence.MCPRegistrationRepo
	access  *persistence.AccessPolicyRepo
	filter  *persistence.FilterPolicyRepo
}

func newSeedRepos(t *testing.T) seedRepos {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := persistence.Open(persistence.Config{DSN: path})
	require.NoError(t, err)
	return seedRepos{
		markers: persistence.NewSeedMarkerRepo(db),
		mcp:     persistence.NewMCPRegistrationRepo(db),
		access:  persistence.NewAccessPolicyRepo(db),
		filter:  persistence.NewFilterPolicyRepo(db),
	}
}

func TestSeed_ExampleConfigProducesMatchingRows(t *testing.T) {
	cfg, err := config.Load("../config.example.toml")
	require.NoError(t, err)
	repos := newSeedRepos(t)
	ctx := context.Background()

	require.NoError(t, cfg.Seed(ctx, repos.markers, repos.mcp, repos.access, repos.filter))

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
	cfg, err := config.Load("../config.example.toml")
	require.NoError(t, err)
	repos := newSeedRepos(t)
	ctx := context.Background()
	require.NoError(t, cfg.Seed(ctx, repos.markers, repos.mcp, repos.access, repos.filter))

	reg, ok, err := repos.mcp.Get(ctx, "postgres-ro")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, cache.StatusConnecting, reg.Status)
}

func TestSeed_IsIdempotentNoDuplicateRows(t *testing.T) {
	cfg, err := config.Load("../config.example.toml")
	require.NoError(t, err)
	repos := newSeedRepos(t)
	ctx := context.Background()

	require.NoError(t, cfg.Seed(ctx, repos.markers, repos.mcp, repos.access, repos.filter))
	require.NoError(t, cfg.Seed(ctx, repos.markers, repos.mcp, repos.access, repos.filter))

	mcps, err := repos.mcp.List(ctx)
	require.NoError(t, err)
	assert.Len(t, mcps, len(cfg.MCPs))

	accessPolicies, err := repos.access.List(ctx)
	require.NoError(t, err)
	assert.Len(t, accessPolicies, len(cfg.AccessPolicies))
}

// Replaces TestSeed_ReSeedReflectsChangedConfig, which pinned the opposite
// contract: config used to be re-applied on every boot, which is what
// silently reverted operator actions (ADR-0016). Editing an already-seeded
// entry in the config file now has no effect -- the stored record wins, as
// CONFIG.md has always said it would.
func TestSeed_ReSeedDoesNotReapplyAChangedConfigEntry(t *testing.T) {
	cfg, err := config.Load("../config.example.toml")
	require.NoError(t, err)
	repos := newSeedRepos(t)
	ctx := context.Background()
	require.NoError(t, cfg.Seed(ctx, repos.markers, repos.mcp, repos.access, repos.filter))

	before, ok, err := repos.access.Get(ctx, "db-reader")
	require.NoError(t, err)
	require.True(t, ok)

	for i := range cfg.AccessPolicies {
		if cfg.AccessPolicies[i].Name == "db-reader" {
			cfg.AccessPolicies[i].Grants = []config.GrantConfig{{MCP: "postgres-rw", Tools: []string{"get_user"}}}
		}
	}
	require.NoError(t, cfg.Seed(ctx, repos.markers, repos.mcp, repos.access, repos.filter))

	after, ok, err := repos.access.Get(ctx, "db-reader")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, before.Grants, after.Grants)
}

// An entry the config file gains after the first boot has no marker, so it
// is still seeded -- "seed once per entry", not "seed once, ever".
func TestSeed_SeedsAnEntryAddedToConfigLater(t *testing.T) {
	cfg, err := config.Load("../config.example.toml")
	require.NoError(t, err)
	repos := newSeedRepos(t)
	ctx := context.Background()
	require.NoError(t, cfg.Seed(ctx, repos.markers, repos.mcp, repos.access, repos.filter))

	cfg.AccessPolicies = append(cfg.AccessPolicies, config.AccessPolicyConfig{
		Name:   "added-later",
		Match:  []config.ClaimRuleConfig{{Path: "$.role", Pattern: "^auditor$"}},
		Grants: []config.GrantConfig{{MCP: "postgres-ro", Tools: []string{"get_user"}}},
	})
	require.NoError(t, cfg.Seed(ctx, repos.markers, repos.mcp, repos.access, repos.filter))

	added, ok, err := repos.access.Get(ctx, "added-later")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "postgres-ro", added.Grants[0].MCP)
}

// The security case, end to end through the store: an MCP the operator
// disabled at runtime must still be disabled after a restart re-runs Seed.
// Before ADR-0016 the config's implied enabled=true overwrote it.
func TestSeed_DoesNotRevertARuntimeDisable(t *testing.T) {
	cfg, err := config.Load("../config.example.toml")
	require.NoError(t, err)
	repos := newSeedRepos(t)
	ctx := context.Background()
	require.NoError(t, cfg.Seed(ctx, repos.markers, repos.mcp, repos.access, repos.filter))

	name := cfg.MCPRegistrations()[0].Name
	reg, ok, err := repos.mcp.Get(ctx, name)
	require.NoError(t, err)
	require.True(t, ok)
	require.True(t, reg.Enabled, "the seeded entry starts enabled")
	reg.Enabled = false
	require.NoError(t, repos.mcp.Upsert(ctx, reg))

	// Restart.
	require.NoError(t, cfg.Seed(ctx, repos.markers, repos.mcp, repos.access, repos.filter))

	after, ok, err := repos.mcp.Get(ctx, name)
	require.NoError(t, err)
	require.True(t, ok)
	assert.False(t, after.Enabled, "a revocation that does not survive a restart is not a revocation")
}

// The other half: a policy deleted through the admin API must stay deleted.
// Insert-only seeding alone would resurrect it; the marker is what doesn't.
func TestSeed_DoesNotResurrectADeletedPolicy(t *testing.T) {
	cfg, err := config.Load("../config.example.toml")
	require.NoError(t, err)
	repos := newSeedRepos(t)
	ctx := context.Background()
	require.NoError(t, cfg.Seed(ctx, repos.markers, repos.mcp, repos.access, repos.filter))
	require.NoError(t, repos.access.Delete(ctx, "db-reader"))

	require.NoError(t, cfg.Seed(ctx, repos.markers, repos.mcp, repos.access, repos.filter))

	_, ok, err := repos.access.Get(ctx, "db-reader")
	require.NoError(t, err)
	assert.False(t, ok, "a deleted policy must not come back on restart")
}

// Same for a filter policy whose drop_fields an operator tightened at
// runtime -- reverting that is a data leak, not just a config drift.
func TestSeed_DoesNotRevertTightenedDropFields(t *testing.T) {
	cfg, err := config.Load("../config.example.toml")
	require.NoError(t, err)
	repos := newSeedRepos(t)
	ctx := context.Background()
	require.NoError(t, cfg.Seed(ctx, repos.markers, repos.mcp, repos.access, repos.filter))

	policies, err := cfg.RouterFilterPolicies()
	require.NoError(t, err)
	require.NotEmpty(t, policies)
	name := policies[0].Name

	stored, ok, err := repos.filter.Get(ctx, name)
	require.NoError(t, err)
	require.True(t, ok)
	stored.DropFields = append(stored.DropFields, "$.newly_discovered_pii")
	require.NoError(t, repos.filter.Upsert(ctx, stored))

	require.NoError(t, cfg.Seed(ctx, repos.markers, repos.mcp, repos.access, repos.filter))

	after, ok, err := repos.filter.Get(ctx, name)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Contains(t, after.DropFields, "$.newly_discovered_pii")
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

	require.NoError(t, cfg.Seed(ctx, repos.markers, repos.mcp, repos.access, repos.filter))

	mcps, err := repos.mcp.List(ctx)
	require.NoError(t, err)
	assert.Empty(t, mcps)
}

// TestSeed_BackfillsMarkersForRowsThatPredateThem simulates the exact
// upgrade hazard an adversarial review found: a store from a version
// before seed_markers existed, where an operator had already disabled an
// MCP through the admin API. Without a backfill, the first Seed call on
// this store finds no marker for that name and reverts the disable right
// on the boot that installs the fix for reverted disables.
func TestSeed_BackfillsMarkersForRowsThatPredateThem(t *testing.T) {
	cfg, err := config.Load("../config.example.toml")
	require.NoError(t, err)
	repos := newSeedRepos(t)
	ctx := context.Background()

	// Simulate the pre-ADR-0016 world directly: write the config's rows
	// via Upsert (as the old, marker-less Seed always did), with no
	// markers recorded at all -- the shape of a store from before this
	// table existed.
	for _, reg := range cfg.MCPRegistrations() {
		reg.Status = cache.StatusConnecting
		require.NoError(t, repos.mcp.Upsert(ctx, reg))
	}
	accessPolicies, err := cfg.RouterAccessPolicies()
	require.NoError(t, err)
	for _, p := range accessPolicies {
		require.NoError(t, repos.access.Upsert(ctx, p))
	}

	// The operator disables one MCP and deletes one access policy, exactly
	// as they would through the admin API, before ever upgrading.
	disabledName := cfg.MCPRegistrations()[0].Name
	disabled, ok, err := repos.mcp.Get(ctx, disabledName)
	require.NoError(t, err)
	require.True(t, ok)
	disabled.Enabled = false
	require.NoError(t, repos.mcp.Upsert(ctx, disabled))

	deletedName := accessPolicies[0].Name
	require.NoError(t, repos.access.Delete(ctx, deletedName))

	// Upgrade: run Seed for the first time against this store.
	require.NoError(t, cfg.Seed(ctx, repos.markers, repos.mcp, repos.access, repos.filter))

	stillDisabled, ok, err := repos.mcp.Get(ctx, disabledName)
	require.NoError(t, err)
	require.True(t, ok)
	assert.False(t, stillDisabled.Enabled,
		"backfill must mark the existing row as already-seeded, so the disable is not reverted on the upgrade boot")

	// The deleted policy has no row to backfill from, so it is seeded
	// fresh exactly once on this boot -- a documented, structural
	// limitation (no record exists that it was ever deleted), not
	// something this test asserts should be fixed.
	_, ok, err = repos.access.Get(ctx, deletedName)
	require.NoError(t, err)
	assert.True(t, ok, "documented one-time exception: a pre-marker delete is not recoverable across the upgrade boot")

	// Crucially: a SECOND Seed call (the next restart) must not revert
	// anything further -- the backfilled markers, and the marker Seed
	// itself just wrote for the resurrected policy, both hold now.
	require.NoError(t, repos.access.Delete(ctx, deletedName))
	require.NoError(t, cfg.Seed(ctx, repos.markers, repos.mcp, repos.access, repos.filter))
	_, ok, err = repos.access.Get(ctx, deletedName)
	require.NoError(t, err)
	assert.False(t, ok, "once actually marked, a later restart must not resurrect it again")
}

// A brand-new deployment (no rows, no markers at all) must seed normally
// -- the backfill step finds nothing and must not skip real seeding.
func TestSeed_BackfillIsANoOpOnAFreshStore(t *testing.T) {
	cfg, err := config.Load("../config.example.toml")
	require.NoError(t, err)
	repos := newSeedRepos(t)
	ctx := context.Background()

	require.NoError(t, cfg.Seed(ctx, repos.markers, repos.mcp, repos.access, repos.filter))

	mcps, err := repos.mcp.List(ctx)
	require.NoError(t, err)
	assert.Len(t, mcps, len(cfg.MCPs))
}
