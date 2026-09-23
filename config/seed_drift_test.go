package config_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/atsokha/mcplake/cache"
	"github.com/atsokha/mcplake/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureLogs runs fn with the default logger swapped for one that records
// everything from DEBUG up, and returns what it wrote. DEBUG is included
// deliberately: several of these tests assert that a skip is reported *only*
// at DEBUG, which a WARN-level handler could not tell apart from no log at
// all.
func captureLogs(t *testing.T, fn func()) string {
	t.Helper()
	var buf bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previous) })
	fn()
	return buf.String()
}

// warnLines returns only the WARN records from captured output, so a test
// asserting "nothing was warned about" is not satisfied or confused by the
// DEBUG line that the same skip always emits.
func warnLines(logs string) []string {
	var out []string
	for _, line := range strings.Split(logs, "\n") {
		if strings.Contains(line, "level=WARN") {
			out = append(out, line)
		}
	}
	return out
}

func seedDriftConfig(mcpArgs []string, grantMCP string) *config.Config {
	return &config.Config{
		Server: config.ServerConfig{DataPlaneAddr: ":8080"},
		OIDC: config.OIDCConfig{
			JWKSURL:  "https://auth.example.com/.well-known/jwks.json",
			Issuer:   "https://auth.example.com",
			Audience: "mcp-gateway",
		},
		MCPs: []config.MCPConfig{{
			Name:      "directory",
			Type:      "stdio",
			Command:   "python3",
			Arguments: mcpArgs,
		}},
		AccessPolicies: []config.AccessPolicyConfig{{
			Name:   "readers",
			Match:  []config.ClaimRuleConfig{{Path: "$.role", Pattern: "^reader$"}},
			Grants: []config.GrantConfig{{MCP: grantMCP, Tools: []string{"*"}}},
		}},
	}
}

// The exact shape that broke the compose demo: an MCP is renamed in the
// config file, the access policy granting it keeps its own name, and so the
// policy's marker suppresses the new grant. The store goes on granting an
// MCP that no longer exists and callers get 403 on the one that does.
func TestSeed_WarnsWhenASeededEntryDiffersFromTheStore(t *testing.T) {
	repos := newSeedRepos(t)
	ctx := context.Background()

	before := seedDriftConfig(nil, "postgres-demo")
	require.NoError(t, before.Seed(ctx, repos.markers, repos.mcp, repos.access, repos.filter))

	after := seedDriftConfig(nil, "employee-directory")
	logs := captureLogs(t, func() {
		require.NoError(t, after.Seed(ctx, repos.markers, repos.mcp, repos.access, repos.filter))
	})

	warns := warnLines(logs)
	require.Len(t, warns, 1, "exactly the one changed entry should warn; got:\n%s", logs)
	assert.Contains(t, warns[0], "access_policy")
	assert.Contains(t, warns[0], "readers")
	assert.Contains(t, warns[0], "grants")

	// The warning must not have changed what is stored -- ADR-0016's
	// guarantee is untouched, it is only now audible.
	stored, ok, err := repos.access.Get(ctx, "readers")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "postgres-demo", stored.Grants[0].MCP)
}

// The ordinary boot: nothing changed, so nothing is worth an operator's
// attention. Warning here would train them to ignore the warning that
// matters.
func TestSeed_DoesNotWarnWhenSeededEntriesAreUnchanged(t *testing.T) {
	repos := newSeedRepos(t)
	ctx := context.Background()
	cfg := seedDriftConfig([]string{"/srv/directory.py"}, "directory")

	require.NoError(t, cfg.Seed(ctx, repos.markers, repos.mcp, repos.access, repos.filter))
	logs := captureLogs(t, func() {
		require.NoError(t, cfg.Seed(ctx, repos.markers, repos.mcp, repos.access, repos.filter))
	})

	assert.Empty(t, warnLines(logs), "an unchanged re-seed must stay quiet; got:\n%s", logs)
	assert.Contains(t, logs, "already seeded", "it should still be visible at DEBUG")
}

// Status and Tools are the gateway's, not the config file's: Register sets
// them on every connect. Comparing them would make a warning fire on every
// boot after the first for every MCP that is simply working.
func TestSeed_DoesNotWarnBecauseAnMCPHasSinceConnected(t *testing.T) {
	repos := newSeedRepos(t)
	ctx := context.Background()
	cfg := seedDriftConfig([]string{"/srv/directory.py"}, "directory")
	require.NoError(t, cfg.Seed(ctx, repos.markers, repos.mcp, repos.access, repos.filter))

	stored, ok, err := repos.mcp.Get(ctx, "directory")
	require.NoError(t, err)
	require.True(t, ok)
	stored.Status = cache.StatusActive
	stored.Tools = map[string]cache.ToolSchema{
		"list_employees": {Name: "list_employees", InputSchema: []byte(`{"type":"object"}`)},
	}
	require.NoError(t, repos.mcp.Upsert(ctx, stored))

	logs := captureLogs(t, func() {
		require.NoError(t, cfg.Seed(ctx, repos.markers, repos.mcp, repos.access, repos.filter))
	})

	assert.Empty(t, warnLines(logs), "a connected MCP is not config drift; got:\n%s", logs)
}

// A config file that omits `arguments` yields nil; the store round-trips the
// same absence as an empty slice. They mean the same thing.
func TestSeed_DoesNotWarnWhenOmittedArgumentsRoundTripAsEmpty(t *testing.T) {
	repos := newSeedRepos(t)
	ctx := context.Background()
	cfg := seedDriftConfig(nil, "directory")

	require.NoError(t, cfg.Seed(ctx, repos.markers, repos.mcp, repos.access, repos.filter))
	logs := captureLogs(t, func() {
		require.NoError(t, cfg.Seed(ctx, repos.markers, repos.mcp, repos.access, repos.filter))
	})

	assert.Empty(t, warnLines(logs), "nil and [] arguments are the same thing; got:\n%s", logs)
}

// Not seeding a deleted entry is ADR-0016's whole point, not a misconfiguration
// -- the operator asked for it to be gone. Warning would be telling them off
// for using the admin API.
func TestSeed_DoesNotWarnAboutAnEntryTheOperatorDeleted(t *testing.T) {
	repos := newSeedRepos(t)
	ctx := context.Background()
	cfg := seedDriftConfig([]string{"/srv/directory.py"}, "directory")
	require.NoError(t, cfg.Seed(ctx, repos.markers, repos.mcp, repos.access, repos.filter))
	require.NoError(t, repos.access.Delete(ctx, "readers"))

	logs := captureLogs(t, func() {
		require.NoError(t, cfg.Seed(ctx, repos.markers, repos.mcp, repos.access, repos.filter))
	})

	assert.Empty(t, warnLines(logs), "a deliberate deletion is not drift; got:\n%s", logs)

	_, ok, err := repos.access.Get(ctx, "readers")
	require.NoError(t, err)
	assert.False(t, ok, "and it must still stay deleted")
}

// An MCP's arguments routinely carry a connection string with a password in
// it -- the compose demo's own former postgres entry did. The warning names
// the fields that differ and never their values, so enabling it cannot put a
// credential into the logs on every boot.
func TestSeed_DriftWarningNamesFieldsNeverValues(t *testing.T) {
	repos := newSeedRepos(t)
	ctx := context.Background()

	const secret = "postgresql://demo:hunter2@postgres:5432/demo"
	before := seedDriftConfig([]string{secret}, "directory")
	require.NoError(t, before.Seed(ctx, repos.markers, repos.mcp, repos.access, repos.filter))

	after := seedDriftConfig([]string{"postgresql://demo:rotated@postgres:5432/demo"}, "directory")
	logs := captureLogs(t, func() {
		require.NoError(t, after.Seed(ctx, repos.markers, repos.mcp, repos.access, repos.filter))
	})

	warns := warnLines(logs)
	require.Len(t, warns, 1, "the changed MCP should warn; got:\n%s", logs)
	assert.Contains(t, warns[0], "connect.arguments", "it should say which field drifted")
	assert.NotContains(t, logs, "hunter2", "the stored value must not be logged")
	assert.NotContains(t, logs, "rotated", "the config value must not be logged either")
}
