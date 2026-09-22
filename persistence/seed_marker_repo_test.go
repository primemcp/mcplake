package persistence_test

import (
	"context"
	"testing"

	"github.com/atsokha/mcplake/persistence"
	"github.com/atsokha/mcplake/router"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newSeedMarkerRepo(t *testing.T) *persistence.SeedMarkerRepo {
	t.Helper()
	return persistence.NewSeedMarkerRepo(openTestDB(t))
}

func TestSeedMarkerRepo_EmptyStoreHasNothingSeeded(t *testing.T) {
	repo := newSeedMarkerRepo(t)

	seeded, err := repo.Seeded(context.Background())

	require.NoError(t, err)
	assert.False(t, seeded.Has(persistence.SeedKindMCP, "postgres-ro"))
}

func TestSeedMarkerRepo_MarkThenSeeded(t *testing.T) {
	repo := newSeedMarkerRepo(t)
	ctx := context.Background()
	require.NoError(t, repo.Mark(ctx, persistence.SeedKindMCP, "postgres-ro"))
	require.NoError(t, repo.Mark(ctx, persistence.SeedKindAccessPolicy, "db-reader"))

	seeded, err := repo.Seeded(ctx)

	require.NoError(t, err)
	assert.True(t, seeded.Has(persistence.SeedKindMCP, "postgres-ro"))
	assert.True(t, seeded.Has(persistence.SeedKindAccessPolicy, "db-reader"))
	// Kind is part of the key: the same name under another kind is a
	// different entry.
	assert.False(t, seeded.Has(persistence.SeedKindFilterPolicy, "db-reader"))
	assert.False(t, seeded.Has(persistence.SeedKindMCP, "db-reader"))
}

// Marking twice must not fail: a crash between writing an entry and marking
// it should cost one harmless re-seed, never a failed startup.
func TestSeedMarkerRepo_MarkIsIdempotent(t *testing.T) {
	repo := newSeedMarkerRepo(t)
	ctx := context.Background()

	require.NoError(t, repo.Mark(ctx, persistence.SeedKindMCP, "postgres-ro"))
	require.NoError(t, repo.Mark(ctx, persistence.SeedKindMCP, "postgres-ro"))

	seeded, err := repo.Seeded(ctx)
	require.NoError(t, err)
	assert.True(t, seeded.Has(persistence.SeedKindMCP, "postgres-ro"))
}

// The marker has to outlive the row it describes -- that is the only thing
// standing between a deleted policy and its resurrection on the next boot.
func TestSeedMarkerRepo_MarkerSurvivesTheEntryItDescribes(t *testing.T) {
	db := openTestDB(t)
	markers := persistence.NewSeedMarkerRepo(db)
	policies := persistence.NewAccessPolicyRepo(db)
	ctx := context.Background()

	require.NoError(t, policies.Upsert(ctx, router.AccessPolicy{
		Name:    "db-reader",
		Enabled: true,
		Grants:  []router.Grant{{MCP: "postgres-ro", Tools: []string{"*"}}},
	}))
	require.NoError(t, markers.Mark(ctx, persistence.SeedKindAccessPolicy, "db-reader"))
	require.NoError(t, policies.Delete(ctx, "db-reader"))

	seeded, err := markers.Seeded(ctx)
	require.NoError(t, err)
	assert.True(t, seeded.Has(persistence.SeedKindAccessPolicy, "db-reader"))
}
