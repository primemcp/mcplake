package router_test

import (
	"encoding/json"
	"testing"

	"github.com/atsokha/mcplake/router"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const writerClaims = `{"role": "db-writer"}`
const readerClaims = `{"role": "db-reader"}`
const adminClaims = `{"role": "admin"}`

func matcherFor(t *testing.T, path, pattern string) router.ClaimMatcher {
	t.Helper()
	return router.ClaimMatcher{Rules: []router.ClaimRule{mustRule(t, path, pattern)}}
}

func TestEngine_Authorize_NoPoliciesReturnsFalse(t *testing.T) {
	engine := router.NewEngine(nil, nil)

	ok, err := engine.Authorize(json.RawMessage(writerClaims), "postgres-rw", "get_user")

	require.NoError(t, err)
	assert.False(t, ok)
}

func TestEngine_Authorize_NoMatchingPolicyReturnsFalse(t *testing.T) {
	engine := router.NewEngine([]router.AccessPolicy{
		{
			Name:  "db-reader",
			Match: matcherFor(t, "$.role", "^db-reader$"),
			Grants: []router.Grant{
				{MCP: "postgres-ro", Tools: []string{"get_user"}},
			},
		},
	}, nil)

	ok, err := engine.Authorize(json.RawMessage(writerClaims), "postgres-ro", "get_user")

	require.NoError(t, err)
	assert.False(t, ok, "role db-writer should not match a policy scoped to db-reader")
}

func TestEngine_Authorize_MatchingPolicyGrantsSpecificTool(t *testing.T) {
	engine := router.NewEngine([]router.AccessPolicy{
		{
			Name:  "db-writer",
			Match: matcherFor(t, "$.role", "^db-writer$"),
			Grants: []router.Grant{
				{MCP: "postgres-rw", Tools: []string{"get_user"}},
			},
		},
	}, nil)

	ok, err := engine.Authorize(json.RawMessage(writerClaims), "postgres-rw", "get_user")
	require.NoError(t, err)
	assert.True(t, ok)

	ok, err = engine.Authorize(json.RawMessage(writerClaims), "postgres-rw", "delete_user")
	require.NoError(t, err)
	assert.False(t, ok, "grant only covers get_user, not delete_user")
}

func TestEngine_Authorize_WildcardToolsGrantsEveryToolOnMCP(t *testing.T) {
	engine := router.NewEngine([]router.AccessPolicy{
		{
			Name:  "db-writer",
			Match: matcherFor(t, "$.role", "^db-writer$"),
			Grants: []router.Grant{
				{MCP: "postgres-rw", Tools: []string{"*"}},
			},
		},
	}, nil)

	for _, tool := range []string{"get_user", "delete_user", "anything"} {
		ok, err := engine.Authorize(json.RawMessage(writerClaims), "postgres-rw", tool)
		require.NoError(t, err)
		assert.True(t, ok, "tool %q should be covered by a wildcard grant", tool)
	}

	ok, err := engine.Authorize(json.RawMessage(writerClaims), "postgres-ro", "get_user")
	require.NoError(t, err)
	assert.False(t, ok, "wildcard tools grant does not extend to a different MCP")
}

func TestEngine_Authorize_WildcardMCPGrantsAcrossEveryMCP(t *testing.T) {
	engine := router.NewEngine([]router.AccessPolicy{
		{
			Name:  "admin",
			Match: matcherFor(t, "$.role", "^admin$"),
			Grants: []router.Grant{
				{MCP: "*", Tools: []string{"get_user"}},
			},
		},
	}, nil)

	for _, mcp := range []string{"postgres-ro", "postgres-rw", "filesystem"} {
		ok, err := engine.Authorize(json.RawMessage(adminClaims), mcp, "get_user")
		require.NoError(t, err)
		assert.True(t, ok, "MCP %q should be covered by a wildcard MCP grant", mcp)
	}

	ok, err := engine.Authorize(json.RawMessage(adminClaims), "postgres-ro", "delete_user")
	require.NoError(t, err)
	assert.False(t, ok, "wildcard MCP grant does not extend to an ungranted tool")
}

func TestEngine_Authorize_MultiplePoliciesUnionSemantics(t *testing.T) {
	engine := router.NewEngine([]router.AccessPolicy{
		{
			Name:  "db-reader",
			Match: matcherFor(t, "$.role", "^db-reader$"),
			Grants: []router.Grant{
				{MCP: "postgres-ro", Tools: []string{"get_user"}},
			},
		},
		{
			Name:  "db-writer",
			Match: matcherFor(t, "$.role", "^db-writer$"),
			Grants: []router.Grant{
				{MCP: "postgres-rw", Tools: []string{"get_user"}},
			},
		},
	}, nil)

	// Only the db-reader policy matches this caller, and only its grant applies.
	ok, err := engine.Authorize(json.RawMessage(readerClaims), "postgres-ro", "get_user")
	require.NoError(t, err)
	assert.True(t, ok)

	ok, err = engine.Authorize(json.RawMessage(readerClaims), "postgres-rw", "get_user")
	require.NoError(t, err)
	assert.False(t, ok, "the matching policy's grant does not cover postgres-rw")
}

func TestEngine_Authorize_PropagatesClaimMatchError(t *testing.T) {
	engine := router.NewEngine([]router.AccessPolicy{
		{
			Name:   "any",
			Match:  matcherFor(t, "$.role", "^db-writer$"),
			Grants: []router.Grant{{MCP: "*", Tools: []string{"*"}}},
		},
	}, nil)

	_, err := engine.Authorize(json.RawMessage(`{not json`), "postgres-rw", "get_user")

	assert.Error(t, err)
}
