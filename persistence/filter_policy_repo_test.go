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

func newFilterPolicyRepo(t *testing.T) *persistence.FilterPolicyRepo {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := persistence.Open(persistence.Config{DSN: path})
	require.NoError(t, err)
	return persistence.NewFilterPolicyRepo(db)
}

func sampleFilterPolicy() router.FilterPolicy {
	rule, err := router.NewClaimRule("$.role", "^user$")
	if err != nil {
		panic(err)
	}
	return router.FilterPolicy{
		Name:       "hide-pii",
		Match:      router.ClaimMatcher{Rules: []router.ClaimRule{rule}},
		MCP:        "postgres-ro",
		Tool:       "get_user",
		DropFields: []string{"$.hashed_password", "$.api_key"},
	}
}

func TestFilterPolicyRepo_UpsertThenGetIsImmediatelyUsable(t *testing.T) {
	repo := newFilterPolicyRepo(t)
	ctx := context.Background()
	require.NoError(t, repo.Upsert(ctx, sampleFilterPolicy()))

	got, ok, err := repo.Get(ctx, "hide-pii")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "postgres-ro", got.MCP)
	assert.Equal(t, "get_user", got.Tool)
	assert.Equal(t, []string{"$.hashed_password", "$.api_key"}, got.DropFields)

	matched, err := got.Match.Matches([]byte(`{"role":"user"}`))
	require.NoError(t, err)
	assert.True(t, matched, "recompiled ClaimMatcher must be directly usable")
}

func TestFilterPolicyRepo_GetUnknownNameReturnsNotFoundNotError(t *testing.T) {
	repo := newFilterPolicyRepo(t)

	_, ok, err := repo.Get(context.Background(), "does-not-exist")

	require.NoError(t, err)
	assert.False(t, ok)
}

func TestFilterPolicyRepo_UpsertOnExistingNameUpdatesInPlace(t *testing.T) {
	repo := newFilterPolicyRepo(t)
	ctx := context.Background()
	policy := sampleFilterPolicy()
	require.NoError(t, repo.Upsert(ctx, policy))

	policy.DropFields = []string{"$.email"}
	require.NoError(t, repo.Upsert(ctx, policy))

	got, ok, err := repo.Get(ctx, "hide-pii")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, []string{"$.email"}, got.DropFields)

	all, err := repo.List(ctx)
	require.NoError(t, err)
	assert.Len(t, all, 1, "upsert must not create a duplicate row")
}

func TestFilterPolicyRepo_UpsertRejectsMalformedRuleAndDoesNotPersist(t *testing.T) {
	repo := newFilterPolicyRepo(t)
	ctx := context.Background()
	policy := router.FilterPolicy{
		Name:       "broken",
		Match:      router.ClaimMatcher{Rules: []router.ClaimRule{{Path: "$.role", Pattern: "(unclosed"}}},
		MCP:        "postgres-ro",
		Tool:       "get_user",
		DropFields: []string{"$.email"},
	}

	err := repo.Upsert(ctx, policy)

	require.Error(t, err)
	all, listErr := repo.List(ctx)
	require.NoError(t, listErr)
	assert.Empty(t, all)
}

func TestFilterPolicyRepo_ListReturnsAllPolicies(t *testing.T) {
	repo := newFilterPolicyRepo(t)
	ctx := context.Background()
	require.NoError(t, repo.Upsert(ctx, sampleFilterPolicy()))
	other := sampleFilterPolicy()
	other.Name = "hide-pii-list"
	require.NoError(t, repo.Upsert(ctx, other))

	all, err := repo.List(ctx)

	require.NoError(t, err)
	assert.Len(t, all, 2)
}

func TestFilterPolicyRepo_DeleteRemovesRow(t *testing.T) {
	repo := newFilterPolicyRepo(t)
	ctx := context.Background()
	require.NoError(t, repo.Upsert(ctx, sampleFilterPolicy()))

	require.NoError(t, repo.Delete(ctx, "hide-pii"))

	_, ok, err := repo.Get(ctx, "hide-pii")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestFilterPolicyRepo_DeleteThenUpsertSameNameSucceeds(t *testing.T) {
	repo := newFilterPolicyRepo(t)
	ctx := context.Background()
	require.NoError(t, repo.Upsert(ctx, sampleFilterPolicy()))
	require.NoError(t, repo.Delete(ctx, "hide-pii"))

	require.NoError(t, repo.Upsert(ctx, sampleFilterPolicy()))

	_, ok, err := repo.Get(ctx, "hide-pii")
	require.NoError(t, err)
	assert.True(t, ok)
}
