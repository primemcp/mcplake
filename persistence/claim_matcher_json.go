package persistence

import (
	"encoding/json"
	"fmt"

	"github.com/atsokha/mcplake/router"
)

// claimRuleJSON is the wire/storage shape of a router.ClaimRule: just the
// Path/Pattern strings a rule was constructed from. router.ClaimRule's
// compiled jsonpath.Path/regexp.Regexp fields are unexported and thus never
// round-trip through JSON — every ClaimMatcher read back from storage is
// recompiled via router.NewClaimRule instead of unmarshaled directly, so it
// is immediately usable (Matches works without further construction).
type claimRuleJSON struct {
	Path    string `json:"path"`
	Pattern string `json:"pattern"`
}

// marshalClaimMatcher serializes m's rules as their Path/Pattern pairs.
func marshalClaimMatcher(m router.ClaimMatcher) ([]byte, error) {
	rules := make([]claimRuleJSON, len(m.Rules))
	for i, rule := range m.Rules {
		rules[i] = claimRuleJSON{Path: rule.Path, Pattern: rule.Pattern}
	}
	data, err := json.Marshal(rules)
	if err != nil {
		return nil, fmt.Errorf("persistence: marshal match rules: %w", err)
	}
	return data, nil
}

// unmarshalClaimMatcher decodes data (as produced by marshalClaimMatcher)
// and recompiles each rule via router.NewClaimRule. A malformed JSONPath or
// regexp is reported here rather than being silently stored as an unusable
// matcher.
func unmarshalClaimMatcher(data []byte) (router.ClaimMatcher, error) {
	if len(data) == 0 {
		return router.ClaimMatcher{}, nil
	}

	var raw []claimRuleJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return router.ClaimMatcher{}, fmt.Errorf("persistence: unmarshal match rules: %w", err)
	}

	rules := make([]router.ClaimRule, len(raw))
	for i, r := range raw {
		rule, err := router.NewClaimRule(r.Path, r.Pattern)
		if err != nil {
			return router.ClaimMatcher{}, fmt.Errorf("persistence: compile match[%d]: %w", i, err)
		}
		rules[i] = rule
	}
	return router.ClaimMatcher{Rules: rules}, nil
}

// validateClaimMatcher reports an error if any rule in m fails to compile,
// without needing the compiled result. Used to reject a malformed policy at
// write time (Upsert) rather than storing an unusable matcher.
func validateClaimMatcher(m router.ClaimMatcher) error {
	for i, rule := range m.Rules {
		if _, err := router.NewClaimRule(rule.Path, rule.Pattern); err != nil {
			return fmt.Errorf("persistence: match[%d]: %w", i, err)
		}
	}
	return nil
}
