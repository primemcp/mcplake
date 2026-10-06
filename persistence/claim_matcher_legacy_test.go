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

// TestList_LoadsALegacyEmptyPatternRowInsteadOfFailingTheWholeList simulates
// a row written before router.NewClaimRule started rejecting an empty
// Pattern: the MCP-control-server tool surface had no required-ness check
// on it until that validation existed, so such a row could already be on
// disk when an operator upgrades to this version. unmarshalClaimMatcher
// must load it as if its pattern were ".+" rather than erroring -- and
// List() must not abort for every *other* stored policy just because one
// row predates the new validation.
func TestList_LoadsALegacyEmptyPatternRowInsteadOfFailingTheWholeList(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "legacy.db")
	ctx := context.Background()

	db, err := persistence.Open(persistence.Config{DSN: dsn})
	require.NoError(t, err)
	accessRepo := persistence.NewAccessPolicyRepo(db)
	filterRepo := persistence.NewFilterPolicyRepo(db)

	require.NoError(t, accessRepo.Upsert(ctx, router.AccessPolicy{
		Name:    "current-policy",
		Enabled: true,
		Match:   router.ClaimMatcher{Rules: []router.ClaimRule{mustClaimRule(t, "$.role", "^admin$")}},
		Grants:  []router.Grant{{MCP: "postgres-ro", Tools: []string{"*"}}},
	}))

	// A legacy row: match = [{"path":"$.role","pattern":""}]. Written
	// directly, bypassing Upsert's validateClaimMatcher, to simulate data
	// that predates this validation rather than testing the validation
	// itself (that's covered elsewhere).
	require.NoError(t, db.Exec(
		`INSERT INTO access_policy_rows (created_at, updated_at, name, match, grants, enabled)
		 VALUES (datetime('now'), datetime('now'), 'legacy-policy',
		         '[{"path":"$.role","pattern":""}]', '[{"MCP":"postgres-ro","Tools":["*"]}]', true)`).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO filter_policy_rows (created_at, updated_at, name, match, mcp, tool, drop_fields, enabled)
		 VALUES (datetime('now'), datetime('now'), 'legacy-filter',
		         '[{"path":"$.role","pattern":""}]', 'postgres-ro', 'get_user', '["$.ssn"]', true)`).Error)

	accessPolicies, err := accessRepo.List(ctx)
	require.NoError(t, err, "one legacy row must not take down every other stored policy")
	names := make([]string, len(accessPolicies))
	for i, p := range accessPolicies {
		names[i] = p.Name
	}
	assert.ElementsMatch(t, []string{"current-policy", "legacy-policy"}, names)

	filterPolicies, err := filterRepo.List(ctx)
	require.NoError(t, err)
	require.Len(t, filterPolicies, 1)

	// The legacy rule must keep matching exactly what it matched before —
	// "this claim is present" — not silently become unreachable (which
	// would revoke the access-policy grant) or match everything regardless
	// of the claim (which would be a *wider* grant than it ever had).
	legacy, ok := findByName(accessPolicies, "legacy-policy")
	require.True(t, ok)
	matchedWithClaim, err := legacy.Match.Matches([]byte(`{"role":"anything"}`))
	require.NoError(t, err)
	assert.True(t, matchedWithClaim)
	matchedWithoutClaim, err := legacy.Match.Matches([]byte(`{}`))
	require.NoError(t, err)
	assert.False(t, matchedWithoutClaim, "the claim being present was always the actual condition")
}

func mustClaimRule(t *testing.T, path, pattern string) router.ClaimRule {
	t.Helper()
	rule, err := router.NewClaimRule(path, pattern)
	require.NoError(t, err)
	return rule
}

func findByName(policies []router.AccessPolicy, name string) (router.AccessPolicy, bool) {
	for _, p := range policies {
		if p.Name == name {
			return p, true
		}
	}
	return router.AccessPolicy{}, false
}
