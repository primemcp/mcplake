package router

import (
	"encoding/json"
	"fmt"
)

// FilterPolicy contributes a set of response-field paths to strip from a
// specific (MCP, Tool) call's response, for any caller whose claims satisfy
// Match. Unlike AccessPolicy's Grant, there is no wildcard MCP/Tool here —
// a filter always targets one specific tool response shape. See ADR-0004.
type FilterPolicy struct {
	Name       string
	Match      ClaimMatcher
	MCP        string
	Tool       string
	DropFields []string
	// Enabled is the operator's on/off switch for this policy. A disabled
	// policy is skipped entirely by Engine.FieldsToRemove, so it strips no
	// fields and the response is returned unfiltered (see #95). It defaults
	// to true at every boundary that constructs a policy (config,
	// persistence, admin API); a zero-valued FilterPolicy literal is
	// disabled, so in-process constructors must set it explicitly.
	Enabled bool
}

// FieldsToRemove returns the union (deduplicated, order of first
// appearance) of DropFields from every FilterPolicy whose Match matches
// claims and whose (MCP, Tool) equals the requested pair. No matching
// policy returns an empty, non-nil-error result.
func (e *Engine) FieldsToRemove(claims json.RawMessage, mcp, tool string) ([]string, error) {
	seen := make(map[string]struct{})
	fields := make([]string, 0)

	for _, policy := range e.filterPolicies {
		if !policy.Enabled {
			// A disabled policy strips no fields: skip it so the response is
			// returned unfiltered by this policy (see #95).
			continue
		}
		if policy.MCP != mcp || policy.Tool != tool {
			continue
		}
		matched, err := policy.Match.Matches(claims)
		if err != nil {
			return nil, fmt.Errorf("router: evaluate filter policy %q: %w", policy.Name, err)
		}
		if !matched {
			continue
		}
		for _, field := range policy.DropFields {
			if _, ok := seen[field]; ok {
				continue
			}
			seen[field] = struct{}{}
			fields = append(fields, field)
		}
	}
	return fields, nil
}
