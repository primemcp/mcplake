package router_test

import (
	"encoding/json"
	"testing"

	"github.com/atsokha/mcplake/filter"
	"github.com/atsokha/mcplake/router"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPolicyPipeline_AuthorizedAndFiltered is the end-to-end test called for
// by Epic #3 (#25): a claim set that is both granted access to a (mcp, tool)
// by an AccessPolicy and matches a FilterPolicy for that same (mcp, tool)
// must have its call authorized AND the specified fields stripped from the
// response — the full "JWT eval -> tool -> resp_filter" pipeline from
// docs/architecture/overview.rst, minus the actual HTTP/MCP transport (that's
// Epic #1 ticket #9).
func TestPolicyPipeline_AuthorizedAndFiltered(t *testing.T) {
	claims := json.RawMessage(`{"role": "user"}`)

	userMatch := router.ClaimMatcher{
		Rules: []router.ClaimRule{mustRule(t, "$.role", "^user$")},
	}

	engine := router.NewEngine(
		[]router.AccessPolicy{
			{
				Name:    "plain-users-read-only",
				Enabled: true,
				Match:   userMatch,
				Grants: []router.Grant{
					{MCP: "postgres-ro", Tools: []string{"get_user"}},
				},
			},
		},
		[]router.FilterPolicy{
			{
				Name:       "hide-pii-for-plain-users",
				Enabled:    true,
				Match:      userMatch,
				MCP:        "postgres-ro",
				Tool:       "get_user",
				DropFields: []string{"$.hashed_password", "$.internal_id"},
			},
		},
	)

	// 1. JWT eval -> authorize.
	authorized, err := engine.Authorize(claims, "postgres-ro", "get_user")
	require.NoError(t, err)
	require.True(t, authorized, "a user role should be granted get_user on postgres-ro")

	// Access to a tool this policy doesn't grant must still be denied.
	authorized, err = engine.Authorize(claims, "postgres-ro", "delete_user")
	require.NoError(t, err)
	require.False(t, authorized)

	// 2. tool -> (simulated) raw response from the downstream MCP.
	rawResponse := json.RawMessage(`{
		"id": 42,
		"email": "user@example.com",
		"hashed_password": "$2a$10$abc",
		"internal_id": "i-001"
	}`)

	// 3. resp_filter -> JWT eval (same engine, filter policies this time).
	fieldsToRemove, err := engine.FieldsToRemove(claims, "postgres-ro", "get_user")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"$.hashed_password", "$.internal_id"}, fieldsToRemove)

	filtered, err := filter.Strip(rawResponse, fieldsToRemove)
	require.NoError(t, err)
	assert.JSONEq(t, `{"id": 42, "email": "user@example.com"}`, string(filtered))
}

// TestPolicyPipeline_UnauthorizedNeverReachesFilter documents that when
// Authorize denies a call, the pipeline stops there — FieldsToRemove/Strip
// are never consulted for a call that shouldn't happen. This is enforced by
// the gateway's request handling (ticket #9), not by the Engine itself, but
// this test pins the contract each half of the pipeline provides.
func TestPolicyPipeline_UnauthorizedNeverReachesFilter(t *testing.T) {
	claims := json.RawMessage(`{"role": "user"}`)
	engine := router.NewEngine(nil, nil) // no policies at all: nothing is granted

	authorized, err := engine.Authorize(claims, "postgres-rw", "delete_user")
	require.NoError(t, err)
	assert.False(t, authorized)
}

// TestPolicyPipeline_DisabledPoliciesStopTheirHalfOfThePipeline pins the
// two policy-level disable semantics from #95 in one place: a disabled
// AccessPolicy authorizes nothing (the pipeline never starts), and a
// disabled FilterPolicy strips nothing (the response is returned
// unfiltered).
func TestPolicyPipeline_DisabledPoliciesStopTheirHalfOfThePipeline(t *testing.T) {
	claims := json.RawMessage(`{"role": "user"}`)
	userMatch := router.ClaimMatcher{
		Rules: []router.ClaimRule{mustRule(t, "$.role", "^user$")},
	}

	engine := router.NewEngine(
		[]router.AccessPolicy{{
			Name:    "plain-users-read-only",
			Enabled: false,
			Match:   userMatch,
			Grants:  []router.Grant{{MCP: "postgres-ro", Tools: []string{"get_user"}}},
		}},
		[]router.FilterPolicy{{
			Name:       "hide-pii-for-plain-users",
			Enabled:    false,
			Match:      userMatch,
			MCP:        "postgres-ro",
			Tool:       "get_user",
			DropFields: []string{"$.hashed_password"},
		}},
	)

	authorized, err := engine.Authorize(claims, "postgres-ro", "get_user")
	require.NoError(t, err)
	assert.False(t, authorized, "a disabled access policy grants nothing")

	rawResponse := json.RawMessage(`{"id":42,"hashed_password":"$2a$10$abc"}`)
	fieldsToRemove, err := engine.FieldsToRemove(claims, "postgres-ro", "get_user")
	require.NoError(t, err)
	assert.Empty(t, fieldsToRemove, "a disabled filter policy strips nothing")

	filtered, err := filter.Strip(rawResponse, fieldsToRemove)
	require.NoError(t, err)
	assert.JSONEq(t, string(rawResponse), string(filtered), "response passes through unfiltered")
}
