package config

import "github.com/atsokha/mcplake/router"

// AdminAuthEnabled reports whether the control-plane admin API should
// enforce JWT authentication and the admin_auth.match claim rules. When it
// returns false the caller must leave /admin/* open and log a warning (see
// ADR-0010).
func (c *Config) AdminAuthEnabled() bool {
	return c.AdminAuth.active()
}

// AdminAuthMatcher compiles admin_auth.match into a router.ClaimMatcher.
// Config.Validate already checks every rule compiles, so an error here
// indicates the config was never validated.
func (c *Config) AdminAuthMatcher() (router.ClaimMatcher, error) {
	return claimMatcherFrom(c.AdminAuth.Match)
}
