package config

import (
	"fmt"

	"github.com/primemcp/mcplake/router"
)

// RouterAccessPolicies converts every AccessPolicies entry into a
// router.AccessPolicy, compiling each Match rule via router.NewClaimRule.
// Config.Validate already checks every rule compiles, so an error here
// indicates the config was never validated.
func (c *Config) RouterAccessPolicies() ([]router.AccessPolicy, error) {
	policies := make([]router.AccessPolicy, 0, len(c.AccessPolicies))
	for _, p := range c.AccessPolicies {
		matcher, err := claimMatcherFrom(p.Match)
		if err != nil {
			return nil, fmt.Errorf("config: access_policies (%s): %w", p.Name, err)
		}

		grants := make([]router.Grant, len(p.Grants))
		for i, g := range p.Grants {
			grants[i] = router.Grant{MCP: g.MCP, Tools: g.Tools}
		}

		policies = append(policies, router.AccessPolicy{
			Name:    p.Name,
			Match:   matcher,
			Grants:  grants,
			Enabled: enabledOrDefault(p.Enabled),
		})
	}
	return policies, nil
}

// RouterFilterPolicies converts every FilterPolicies entry into a
// router.FilterPolicy, compiling each Match rule via router.NewClaimRule.
func (c *Config) RouterFilterPolicies() ([]router.FilterPolicy, error) {
	policies := make([]router.FilterPolicy, 0, len(c.FilterPolicies))
	for _, p := range c.FilterPolicies {
		matcher, err := claimMatcherFrom(p.Match)
		if err != nil {
			return nil, fmt.Errorf("config: filter_policies (%s): %w", p.Name, err)
		}

		policies = append(policies, router.FilterPolicy{
			Name:       p.Name,
			Match:      matcher,
			MCP:        p.MCP,
			Tool:       p.Tool,
			DropFields: p.DropFields,
			Enabled:    enabledOrDefault(p.Enabled),
		})
	}
	return policies, nil
}

func claimMatcherFrom(rules []ClaimRuleConfig) (router.ClaimMatcher, error) {
	compiled := make([]router.ClaimRule, len(rules))
	for i, r := range rules {
		rule, err := router.NewClaimRule(r.Path, r.Pattern)
		if err != nil {
			return router.ClaimMatcher{}, fmt.Errorf("match[%d]: %w", i, err)
		}
		compiled[i] = rule
	}
	return router.ClaimMatcher{Rules: compiled}, nil
}
