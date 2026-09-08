package router_test

import (
	"encoding/json"
	"testing"

	"github.com/atsokha/mcplake/router"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const userClaims = `{"role": "user"}`

func TestEngine_FieldsToRemove_NoMatchingPolicyReturnsEmpty(t *testing.T) {
	engine := router.NewEngine(nil, []router.FilterPolicy{
		{
			Name:       "hide-pii",
			Enabled:    true,
			Match:      matcherFor(t, "$.role", "^admin$"),
			MCP:        "postgres-ro",
			Tool:       "get_user",
			DropFields: []string{"$.email"},
		},
	})

	fields, err := engine.FieldsToRemove(json.RawMessage(userClaims), "postgres-ro", "get_user")

	require.NoError(t, err)
	assert.Empty(t, fields)
}

func TestEngine_FieldsToRemove_SingleMatch(t *testing.T) {
	engine := router.NewEngine(nil, []router.FilterPolicy{
		{
			Name:       "hide-pii",
			Enabled:    true,
			Match:      matcherFor(t, "$.role", "^user$"),
			MCP:        "postgres-ro",
			Tool:       "get_user",
			DropFields: []string{"$.email", "$.hashed_password"},
		},
	})

	fields, err := engine.FieldsToRemove(json.RawMessage(userClaims), "postgres-ro", "get_user")

	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"$.email", "$.hashed_password"}, fields)
}

func TestEngine_FieldsToRemove_WrongMCPOrToolNeverContributes(t *testing.T) {
	engine := router.NewEngine(nil, []router.FilterPolicy{
		{
			Name:       "hide-pii-wrong-mcp",
			Enabled:    true,
			Match:      matcherFor(t, "$.role", "^user$"),
			MCP:        "postgres-rw",
			Tool:       "get_user",
			DropFields: []string{"$.email"},
		},
		{
			Name:       "hide-pii-wrong-tool",
			Enabled:    true,
			Match:      matcherFor(t, "$.role", "^user$"),
			MCP:        "postgres-ro",
			Tool:       "list_users",
			DropFields: []string{"$.phone_number"},
		},
	})

	fields, err := engine.FieldsToRemove(json.RawMessage(userClaims), "postgres-ro", "get_user")

	require.NoError(t, err)
	assert.Empty(t, fields)
}

func TestEngine_FieldsToRemove_MultipleMatchesUnionDeduplicated(t *testing.T) {
	engine := router.NewEngine(nil, []router.FilterPolicy{
		{
			Name:       "hide-email",
			Enabled:    true,
			Match:      matcherFor(t, "$.role", "^user$"),
			MCP:        "postgres-ro",
			Tool:       "get_user",
			DropFields: []string{"$.email", "$.hashed_password"},
		},
		{
			Name:       "hide-internal-id",
			Enabled:    true,
			Match:      matcherFor(t, "$.role", "^user$"),
			MCP:        "postgres-ro",
			Tool:       "get_user",
			DropFields: []string{"$.internal_id", "$.email"}, // $.email overlaps
		},
	})

	fields, err := engine.FieldsToRemove(json.RawMessage(userClaims), "postgres-ro", "get_user")

	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"$.email", "$.hashed_password", "$.internal_id"}, fields)
}

func TestEngine_FieldsToRemove_PropagatesClaimMatchError(t *testing.T) {
	engine := router.NewEngine(nil, []router.FilterPolicy{
		{
			Name:       "hide-pii",
			Enabled:    true,
			Match:      matcherFor(t, "$.role", "^user$"),
			MCP:        "postgres-ro",
			Tool:       "get_user",
			DropFields: []string{"$.email"},
		},
	})

	_, err := engine.FieldsToRemove(json.RawMessage(`{not json`), "postgres-ro", "get_user")

	assert.Error(t, err)
}

func TestEngine_FieldsToRemove_SkipsDisabledPolicy(t *testing.T) {
	engine := router.NewEngine(nil, []router.FilterPolicy{
		{
			Name:       "hide-pii",
			Enabled:    false,
			Match:      matcherFor(t, "$.role", "^user$"),
			MCP:        "postgres-ro",
			Tool:       "get_user",
			DropFields: []string{"$.hashed_password", "$.email"},
		},
	})

	fields, err := engine.FieldsToRemove(json.RawMessage(userClaims), "postgres-ro", "get_user")

	require.NoError(t, err)
	assert.Empty(t, fields, "a disabled filter policy contributes no fields, so the response is returned unfiltered")
}

func TestEngine_FieldsToRemove_DisabledPolicyDoesNotSuppressAnEnabledOne(t *testing.T) {
	engine := router.NewEngine(nil, []router.FilterPolicy{
		{
			Name:       "hide-pii-disabled",
			Enabled:    false,
			Match:      matcherFor(t, "$.role", "^user$"),
			MCP:        "postgres-ro",
			Tool:       "get_user",
			DropFields: []string{"$.email"},
		},
		{
			Name:       "hide-secret-enabled",
			Enabled:    true,
			Match:      matcherFor(t, "$.role", "^user$"),
			MCP:        "postgres-ro",
			Tool:       "get_user",
			DropFields: []string{"$.hashed_password"},
		},
	})

	fields, err := engine.FieldsToRemove(json.RawMessage(userClaims), "postgres-ro", "get_user")

	require.NoError(t, err)
	assert.Equal(t, []string{"$.hashed_password"}, fields, "only the enabled policy's fields are stripped")
}
