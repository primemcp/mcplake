package router

import (
	"encoding/json"
	"fmt"
)

// Grant is a unit of access an AccessPolicy confers once its Match matches:
// permission to call any of Tools on MCP. "*" for MCP means every registered
// MCP; "*" within Tools means every tool on the matched MCP(s).
type Grant struct {
	MCP   string
	Tools []string
}

// covers reports whether g grants access to (mcp, tool).
func (g Grant) covers(mcp, tool string) bool {
	if g.MCP != "*" && g.MCP != mcp {
		return false
	}
	for _, t := range g.Tools {
		if t == "*" || t == tool {
			return true
		}
	}
	return false
}

// AccessPolicy grants its Grants to any caller whose claims satisfy Match.
// See ADR-0004: policies are purely additive in this milestone (no explicit
// deny) — a call is authorized if ANY policy matches and grants it.
type AccessPolicy struct {
	Name   string
	Match  ClaimMatcher
	Grants []Grant
}

// Engine evaluates a caller's claims against a set of AccessPolicy and
// FilterPolicy to answer "is this call authorized" (Authorize) and "what
// response fields should be stripped" (FieldsToRemove).
type Engine struct {
	accessPolicies []AccessPolicy
	filterPolicies []FilterPolicy
}

// NewEngine constructs an Engine over the given access and filter policies.
func NewEngine(accessPolicies []AccessPolicy, filterPolicies []FilterPolicy) *Engine {
	return &Engine{accessPolicies: accessPolicies, filterPolicies: filterPolicies}
}

// Authorize reports whether claims are granted access to (mcp, tool): true
// iff at least one AccessPolicy whose Match matches claims has a Grant
// covering that pair.
func (e *Engine) Authorize(claims json.RawMessage, mcp, tool string) (bool, error) {
	for _, policy := range e.accessPolicies {
		matched, err := policy.Match.Matches(claims)
		if err != nil {
			return false, fmt.Errorf("router: evaluate access policy %q: %w", policy.Name, err)
		}
		if !matched {
			continue
		}
		for _, grant := range policy.Grants {
			if grant.covers(mcp, tool) {
				return true, nil
			}
		}
	}
	return false, nil
}
