package router_test

import (
	"encoding/json"
	"testing"

	"github.com/atsokha/mcplake/router"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sampleClaims = `{
	"role": "db-writer",
	"sub": "user-123",
	"groups": ["oncall-db", "team-data-eng"],
	"empty_groups": [],
	"https://example.com/org": {"tier": "enterprise"},
	"id": 42
}`

func mustRule(t *testing.T, path, pattern string) router.ClaimRule {
	t.Helper()
	r, err := router.NewClaimRule(path, pattern)
	require.NoError(t, err)
	return r
}

func TestClaimRule_Matches(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		pattern string
		want    bool
	}{
		{name: "scalar match", path: "$.role", pattern: "^db-writer$", want: true},
		{name: "scalar non-match", path: "$.role", pattern: "^db-reader$", want: false},
		{name: "scalar prefix match", path: "$.role", pattern: "^db-.*$", want: true},
		{name: "list-any-match via wildcard", path: "$.groups[*]", pattern: "^oncall-.*$", want: true},
		{name: "list-any-match via whole array", path: "$.groups", pattern: "^oncall-.*$", want: true},
		{name: "list-no-match", path: "$.groups[*]", pattern: "^admin-.*$", want: false},
		{name: "empty list never matches", path: "$.empty_groups[*]", pattern: ".*", want: false},
		{name: "missing claim path is not a match", path: "$.nonexistent", pattern: ".*", want: false},
		{name: "nested namespaced path", path: `$["https://example.com/org"].tier`, pattern: "^enterprise$", want: true},
		{name: "nested namespaced path non-match", path: `$["https://example.com/org"].tier`, pattern: "^free$", want: false},
		{name: "numeric claim stringified", path: "$.id", pattern: "^42$", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule := mustRule(t, tt.path, tt.pattern)

			got, err := rule.Matches(json.RawMessage(sampleClaims))

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNewClaimRule_RejectsMalformedPath(t *testing.T) {
	_, err := router.NewClaimRule("not a valid jsonpath $$", "^ok$")
	assert.Error(t, err)
}

func TestNewClaimRule_RejectsMalformedPattern(t *testing.T) {
	_, err := router.NewClaimRule("$.role", "(unclosed")
	assert.Error(t, err)
}

func TestClaimRule_Matches_InvalidClaimsJSON(t *testing.T) {
	rule := mustRule(t, "$.role", ".*")

	_, err := rule.Matches(json.RawMessage(`{not json`))

	assert.Error(t, err)
}

func TestClaimMatcher_Matches(t *testing.T) {
	tests := []struct {
		name  string
		rules []router.ClaimRule
		want  bool
	}{
		{
			name:  "empty matcher matches everything",
			rules: nil,
			want:  true,
		},
		{
			name: "single matching rule",
			rules: []router.ClaimRule{
				mustRule(t, "$.role", "^db-writer$"),
			},
			want: true,
		},
		{
			name: "single non-matching rule",
			rules: []router.ClaimRule{
				mustRule(t, "$.role", "^db-reader$"),
			},
			want: false,
		},
		{
			name: "all rules match (AND)",
			rules: []router.ClaimRule{
				mustRule(t, "$.role", "^db-writer$"),
				mustRule(t, "$.groups[*]", "^oncall-.*$"),
			},
			want: true,
		},
		{
			name: "one rule fails means matcher fails (AND)",
			rules: []router.ClaimRule{
				mustRule(t, "$.role", "^db-writer$"),
				mustRule(t, "$.groups[*]", "^admin-.*$"),
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := router.ClaimMatcher{Rules: tt.rules}

			got, err := matcher.Matches(json.RawMessage(sampleClaims))

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
