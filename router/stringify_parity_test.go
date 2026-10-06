package router_test

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/primemcp/mcplake/router"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stringifyCases is the shared fixture the TypeScript twin of this
// stringification also reads (webui/src/lib/claimMatch.test.ts). Keeping one
// table means the two implementations cannot drift apart without a test
// failing on one side -- which is exactly what happened before #162: a
// numeric claim of 1000042 matched "^1000042$" in the Request path
// simulator and did not match in the gateway, so the UI reported a
// redaction that never happened.
type stringifyCases struct {
	Cases []struct {
		Name  string          `json:"name"`
		Value json.RawMessage `json:"value"`
		Want  string          `json:"want"`
	} `json:"cases"`
}

func loadStringifyCases(t *testing.T) stringifyCases {
	t.Helper()
	raw, err := os.ReadFile("../testdata/claim_stringify_cases.json")
	require.NoError(t, err)
	var cases stringifyCases
	require.NoError(t, json.Unmarshal(raw, &cases))
	require.NotEmpty(t, cases.Cases)
	return cases
}

// TestClaimRule_StringifyParity pins what the gateway actually compares a
// pattern against, for every case in the shared fixture. It drives the real
// matching path rather than an exported helper: a rule anchored on the
// expected rendering must match, and one anchored on anything else must not.
func TestClaimRule_StringifyParity(t *testing.T) {
	for _, tc := range loadStringifyCases(t).Cases {
		t.Run(tc.Name, func(t *testing.T) {
			claims := json.RawMessage(fmt.Sprintf(`{"claim":%s}`, tc.Value))

			rule, err := router.NewClaimRule("$.claim", "^"+regexpQuote(tc.Want)+"$")
			require.NoError(t, err)
			matched, err := rule.Matches(claims)

			require.NoError(t, err)
			assert.True(t, matched, "claim %s must stringify to %q", tc.Value, tc.Want)
		})
	}
}

// regexpQuote escapes the fixture's expected rendering so it can be used as
// an anchored literal pattern (values like "1e+06" and "1.5" contain regexp
// metacharacters).
func regexpQuote(s string) string {
	out := make([]rune, 0, len(s)*2)
	for _, r := range s {
		switch r {
		case '.', '+', '*', '?', '(', ')', '[', ']', '{', '}', '^', '$', '|', '\\':
			out = append(out, '\\', r)
		default:
			out = append(out, r)
		}
	}
	return string(out)
}
