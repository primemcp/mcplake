package persistence_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/primemcp/mcplake/persistence"
	"github.com/primemcp/mcplake/router"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newAccessPolicyRepo(t *testing.T) *persistence.AccessPolicyRepo {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := persistence.Open(persistence.Config{DSN: path})
	require.NoError(t, err)
	return persistence.NewAccessPolicyRepo(db)
}

func sampleAccessPolicy() router.AccessPolicy {
	rule, err := router.NewClaimRule("$.role", "^db-reader$")
	if err != nil {
		panic(err)
	}
	return router.AccessPolicy{
		Name:   "db-reader",
		Match:  router.ClaimMatcher{Rules: []router.ClaimRule{rule}},
		Grants: []router.Grant{{MCP: "postgres-ro", Tools: []string{"*"}}},
	}
}

func TestAccessPolicyRepo_UpsertThenGetIsImmediatelyUsable(t *testing.T) {
	repo := newAccessPolicyRepo(t)
	ctx := context.Background()
	require.NoError(t, repo.Upsert(ctx, sampleAccessPolicy()))

	got, ok, err := repo.Get(ctx, "db-reader")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "db-reader", got.Name)
	assert.Equal(t, []router.Grant{{MCP: "postgres-ro", Tools: []string{"*"}}}, got.Grants)

	matched, err := got.Match.Matches([]byte(`{"role":"db-reader"}`))
	require.NoError(t, err)
	assert.True(t, matched, "recompiled ClaimMatcher must be directly usable")
}

func TestAccessPolicyRepo_GetUnknownNameReturnsNotFoundNotError(t *testing.T) {
	repo := newAccessPolicyRepo(t)

	_, ok, err := repo.Get(context.Background(), "does-not-exist")

	require.NoError(t, err)
	assert.False(t, ok)
}

func TestAccessPolicyRepo_UpsertOnExistingNameUpdatesInPlace(t *testing.T) {
	repo := newAccessPolicyRepo(t)
	ctx := context.Background()
	policy := sampleAccessPolicy()
	require.NoError(t, repo.Upsert(ctx, policy))

	policy.Grants = []router.Grant{{MCP: "postgres-rw", Tools: []string{"get_user"}}}
	require.NoError(t, repo.Upsert(ctx, policy))

	got, ok, err := repo.Get(ctx, "db-reader")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, []router.Grant{{MCP: "postgres-rw", Tools: []string{"get_user"}}}, got.Grants)

	all, err := repo.List(ctx)
	require.NoError(t, err)
	assert.Len(t, all, 1, "upsert must not create a duplicate row")
}

func TestAccessPolicyRepo_UpsertRejectsMalformedRuleAndDoesNotPersist(t *testing.T) {
	repo := newAccessPolicyRepo(t)
	ctx := context.Background()
	policy := router.AccessPolicy{
		Name:   "broken",
		Match:  router.ClaimMatcher{Rules: []router.ClaimRule{{Path: "not a valid jsonpath $$", Pattern: "^x$"}}},
		Grants: []router.Grant{{MCP: "postgres-ro", Tools: []string{"*"}}},
	}

	err := repo.Upsert(ctx, policy)

	require.Error(t, err)
	all, listErr := repo.List(ctx)
	require.NoError(t, listErr)
	assert.Empty(t, all)
}

func TestAccessPolicyRepo_ListReturnsAllPolicies(t *testing.T) {
	repo := newAccessPolicyRepo(t)
	ctx := context.Background()
	require.NoError(t, repo.Upsert(ctx, sampleAccessPolicy()))
	other := sampleAccessPolicy()
	other.Name = "admin"
	require.NoError(t, repo.Upsert(ctx, other))

	all, err := repo.List(ctx)

	require.NoError(t, err)
	assert.Len(t, all, 2)
}

func TestAccessPolicyRepo_DeleteRemovesRow(t *testing.T) {
	repo := newAccessPolicyRepo(t)
	ctx := context.Background()
	require.NoError(t, repo.Upsert(ctx, sampleAccessPolicy()))

	require.NoError(t, repo.Delete(ctx, "db-reader"))

	_, ok, err := repo.Get(ctx, "db-reader")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestAccessPolicyRepo_DeleteThenUpsertSameNameSucceeds(t *testing.T) {
	repo := newAccessPolicyRepo(t)
	ctx := context.Background()
	require.NoError(t, repo.Upsert(ctx, sampleAccessPolicy()))
	require.NoError(t, repo.Delete(ctx, "db-reader"))

	require.NoError(t, repo.Upsert(ctx, sampleAccessPolicy()))

	_, ok, err := repo.Get(ctx, "db-reader")
	require.NoError(t, err)
	assert.True(t, ok)
}
