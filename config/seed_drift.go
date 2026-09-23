package config

import (
	"encoding/json"
	"fmt"

	"github.com/atsokha/mcplake/cache"
	"github.com/atsokha/mcplake/router"
)

// seedField is one config-owned attribute of a seeded entry, rendered as
// JSON so two of them can be compared without reflection -- router.ClaimRule
// carries unexported compiled *jsonpath.Path and *regexp.Regexp fields, so
// reflect.DeepEqual on a whole policy compares two compilations of the same
// pattern rather than the pattern itself.
//
// Only the *names* of mismatched fields are ever logged, never the values:
// an MCP's arguments routinely carry a connection string with a password in
// it (the compose demo's own former postgres entry did), and a WARN on every
// boot is exactly the wrong place to put one.
type seedField struct {
	name  string
	value string
}

// seedFieldsDiffer returns the names of the fields on which want and got
// disagree, in declaration order, or nil when they agree on all of them.
func seedFieldsDiffer(want, got []seedField) []string {
	var names []string
	for i, w := range want {
		if i >= len(got) || got[i].value != w.value {
			names = append(names, w.name)
		}
	}
	return names
}

// mcpSeedFields projects an MCPRegistration onto what a `[[mcps]]` entry
// actually declares. Status and Tools are deliberately absent: both are
// owned by the gateway at runtime (Register computes them fresh on every
// connect), so including them would report drift on every boot merely
// because the MCP has since connected and advertised its tools.
func mcpSeedFields(reg cache.MCPRegistration) []seedField {
	return []seedField{
		{"transport", seedJSON(reg.Transport)},
		{"connect.command", seedJSON(reg.Connect.Command)},
		{"connect.arguments", seedJSON(seedStrings(reg.Connect.Arguments))},
		{"connect.url", seedJSON(reg.Connect.URL)},
		{"enabled", seedJSON(reg.Enabled)},
	}
}

func accessPolicySeedFields(p router.AccessPolicy) []seedField {
	return []seedField{
		{"match", seedJSON(seedClaimRules(p.Match))},
		{"grants", seedJSON(seedGrants(p.Grants))},
		{"enabled", seedJSON(p.Enabled)},
	}
}

func filterPolicySeedFields(p router.FilterPolicy) []seedField {
	return []seedField{
		{"match", seedJSON(seedClaimRules(p.Match))},
		{"mcp", seedJSON(p.MCP)},
		{"tool", seedJSON(p.Tool)},
		{"drop_fields", seedJSON(seedStrings(p.DropFields))},
		{"enabled", seedJSON(p.Enabled)},
	}
}

// seedClaimRuleJSON and seedGrantJSON pin the compared shape here rather
// than relying on how router's own types happen to marshal, so a future
// json tag (or a new exported field) on either cannot quietly change what
// counts as drift.
type seedClaimRuleJSON struct {
	Path    string `json:"path"`
	Pattern string `json:"pattern"`
}

type seedGrantJSON struct {
	MCP   string   `json:"mcp"`
	Tools []string `json:"tools"`
}

func seedClaimRules(m router.ClaimMatcher) []seedClaimRuleJSON {
	out := make([]seedClaimRuleJSON, 0, len(m.Rules))
	for _, r := range m.Rules {
		out = append(out, seedClaimRuleJSON{Path: r.Path, Pattern: r.Pattern})
	}
	return out
}

func seedGrants(grants []router.Grant) []seedGrantJSON {
	out := make([]seedGrantJSON, 0, len(grants))
	for _, g := range grants {
		out = append(out, seedGrantJSON{MCP: g.MCP, Tools: seedStrings(g.Tools)})
	}
	return out
}

// seedStrings normalizes a nil slice to an empty one. A config file that
// omits `arguments` yields nil while the store round-trips the same absence
// as `[]`; the two mean the same thing and must not read as a difference.
func seedStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func seedJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		// Unreachable for the concrete types above, all of which are
		// plain strings, bools and slices of those.
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}
