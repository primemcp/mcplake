package persistence_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/primemcp/mcplake/persistence"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMCPRegistrationRepo_RoundTripsEnabled(t *testing.T) {
	repo := newMCPRegistrationRepo(t)
	ctx := context.Background()

	for _, enabled := range []bool{true, false} {
		reg := sampleRegistration()
		reg.Enabled = enabled
		require.NoError(t, repo.Upsert(ctx, reg))

		got, ok, err := repo.Get(ctx, reg.Name)
		require.NoError(t, err)
		require.True(t, ok)
		assert.Equal(t, enabled, got.Enabled)
	}
}

func TestAccessPolicyRepo_RoundTripsEnabled(t *testing.T) {
	repo := newAccessPolicyRepo(t)
	ctx := context.Background()

	for _, enabled := range []bool{true, false} {
		policy := sampleAccessPolicy()
		policy.Enabled = enabled
		require.NoError(t, repo.Upsert(ctx, policy))

		got, ok, err := repo.Get(ctx, policy.Name)
		require.NoError(t, err)
		require.True(t, ok)
		assert.Equal(t, enabled, got.Enabled)
	}
}

func TestFilterPolicyRepo_RoundTripsEnabled(t *testing.T) {
	repo := newFilterPolicyRepo(t)
	ctx := context.Background()

	for _, enabled := range []bool{true, false} {
		policy := sampleFilterPolicy()
		policy.Enabled = enabled
		require.NoError(t, repo.Upsert(ctx, policy))

		got, ok, err := repo.Get(ctx, policy.Name)
		require.NoError(t, err)
		require.True(t, ok)
		assert.Equal(t, enabled, got.Enabled)
	}
}

// TestAutoMigrate_BackfillsEnabledOnLegacyRows simulates a database created
// before the enabled column existed: the column is dropped and a row
// inserted without it, then Open (which runs AutoMigrate) is called again on
// the same file. The re-added column's NOT NULL DEFAULT true must backfill
// the pre-existing row as enabled, so an upgrade changes no behaviour.
func TestAutoMigrate_BackfillsEnabledOnLegacyRows(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "legacy.db")
	ctx := context.Background()

	db, err := persistence.Open(persistence.Config{DSN: dsn})
	require.NoError(t, err)

	require.NoError(t, db.Exec(`ALTER TABLE mcp_registration_rows DROP COLUMN enabled`).Error)
	require.NoError(t, db.Exec(`ALTER TABLE access_policy_rows DROP COLUMN enabled`).Error)
	require.NoError(t, db.Exec(`ALTER TABLE filter_policy_rows DROP COLUMN enabled`).Error)

	require.NoError(t, db.Exec(
		`INSERT INTO mcp_registration_rows (created_at, updated_at, name, transport, status)
		 VALUES (datetime('now'), datetime('now'), 'legacy-mcp', 'stdio', 'active')`).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO access_policy_rows (created_at, updated_at, name, match, grants)
		 VALUES (datetime('now'), datetime('now'), 'legacy-access', '[]', '[]')`).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO filter_policy_rows (created_at, updated_at, name, match, mcp, tool, drop_fields)
		 VALUES (datetime('now'), datetime('now'), 'legacy-filter', '[]', 'legacy-mcp', 'get_user', '[]')`).Error)

	// Re-open: AutoMigrate re-adds the enabled column and backfills.
	db2, err := persistence.Open(persistence.Config{DSN: dsn})
	require.NoError(t, err)

	reg, ok, err := persistence.NewMCPRegistrationRepo(db2).Get(ctx, "legacy-mcp")
	require.NoError(t, err)
	require.True(t, ok)
	assert.True(t, reg.Enabled, "legacy MCP row must read back as enabled after migration")

	access, ok, err := persistence.NewAccessPolicyRepo(db2).Get(ctx, "legacy-access")
	require.NoError(t, err)
	require.True(t, ok)
	assert.True(t, access.Enabled, "legacy access policy row must read back as enabled after migration")

	filter, ok, err := persistence.NewFilterPolicyRepo(db2).Get(ctx, "legacy-filter")
	require.NoError(t, err)
	require.True(t, ok)
	assert.True(t, filter.Enabled, "legacy filter policy row must read back as enabled after migration")
}
