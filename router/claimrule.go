// Package router implements the Policy Engine's claim-rule matching (see
// docs/architecture/decisions/0002-jsonpath-regexp-claim-rule-engine.md) and,
// built on it, claims-based routing/authorization.
package router

import (
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/theory/jsonpath"
)

// ClaimRule extracts a value from a JWT claim document via a JSONPath
// expression and tests it against a regexp. See ADR-0002 for the matching
// semantics, including how a JSON array result is handled.
type ClaimRule struct {
	Path    string
	Pattern string

	path    *jsonpath.Path
	pattern *regexp.Regexp
}

// NewClaimRule compiles path and pattern once, so Matches never pays
// parse/compile cost per call. A malformed path or pattern is rejected here,
// at construction time, not discovered later during request evaluation.
//
// An empty pattern is rejected along with a malformed one. `regexp.Compile("")`
// succeeds and then matches every value, so such a rule reads like a
// constraint in a config file or an admin API response while constraining
// nothing -- and matching is unanchored, so it cannot even be salvaged by
// reading it as "the claim exists". Spell that intent as `.+`. This is the
// one place every write path (config, admin API, MCP control server,
// persistence) constructs a rule, so rejecting it here closes all of them.
func NewClaimRule(path, pattern string) (ClaimRule, error) {
	if path == "" {
		return ClaimRule{}, fmt.Errorf("router: claim rule path is required")
	}
	if pattern == "" {
		return ClaimRule{}, fmt.Errorf("router: claim rule pattern is required (use %q to mean \"the claim is present\")", ".+")
	}
	compiledPath, err := jsonpath.Parse(path)
	if err != nil {
		return ClaimRule{}, fmt.Errorf("router: parse JSONPath %q: %w", path, err)
	}
	compiledPattern, err := regexp.Compile(pattern)
	if err != nil {
		return ClaimRule{}, fmt.Errorf("router: compile pattern %q: %w", pattern, err)
	}
	return ClaimRule{
		Path:    path,
		Pattern: pattern,
		path:    compiledPath,
		pattern: compiledPattern,
	}, nil
}

// Matches reports whether claims (a decoded JWT claim document, e.g.
// auth.Claims.Raw) satisfies the rule.
//
// The JSONPath expression is evaluated against claims; the rule matches if
// any resulting value matches Pattern as a string. If a resulting value is
// itself a JSON array (e.g. Path is "$.groups" rather than "$.groups[*]"),
// each element of that array is tested individually — this is the "is there
// an element in the list that satisfies the regexp" case. A Path that
// resolves to nothing (the claim is absent) is not a match and not an error.
func (r ClaimRule) Matches(claims json.RawMessage) (bool, error) {
	var doc any
	if err := json.Unmarshal(claims, &doc); err != nil {
		return false, fmt.Errorf("router: unmarshal claims: %w", err)
	}

	for _, node := range r.path.Select(doc) {
		if r.matchNode(node) {
			return true, nil
		}
	}
	return false, nil
}

func (r ClaimRule) matchNode(node any) bool {
	if list, ok := node.([]any); ok {
		for _, elem := range list {
			if r.matchScalar(elem) {
				return true
			}
		}
		return false
	}
	return r.matchScalar(node)
}

func (r ClaimRule) matchScalar(v any) bool {
	return r.pattern.MatchString(stringify(v))
}

// stringify renders a decoded JSON value (string, float64, bool, nil, or a
// nested map/slice for a malformed rule that targets a non-scalar) as a
// string for regexp matching.
func stringify(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case nil:
		return ""
	default:
		return fmt.Sprint(val)
	}
}

// ClaimMatcher is a set of ClaimRules combined with AND: every rule must
// match for the matcher as a whole to match. An empty matcher (no rules)
// matches everything.
type ClaimMatcher struct {
	Rules []ClaimRule
}

// Matches reports whether every rule in the matcher matches claims.
func (m ClaimMatcher) Matches(claims json.RawMessage) (bool, error) {
	for _, rule := range m.Rules {
		ok, err := rule.Matches(claims)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
	}
	return true, nil
}
