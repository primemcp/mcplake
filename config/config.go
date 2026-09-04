package config

import (
	"fmt"
	"time"

	"github.com/atsokha/mcplake/router"
)

type Config struct {
	Server         ServerConfig
	OIDC           OIDCConfig
	MCPs           []MCPConfig
	AccessPolicies []AccessPolicyConfig
	FilterPolicies []FilterPolicyConfig
}

type ServerConfig struct {
	// DataPlaneAddr is the listen address (host:port) for the fasthttp
	// tool-call proxy. See ADR-0001.
	DataPlaneAddr string
	// ControlPlaneAddr is the listen address (host:port) for the Gin admin
	// API. See ADR-0005.
	ControlPlaneAddr string
	// TODO: Add TLS configuration
}

type OIDCConfig struct {
	// JWKSURL is the OIDC provider's JWKS endpoint. See auth.Config.JWKSURL;
	// OIDC discovery from a provider/issuer URL is not yet implemented.
	JWKSURL string
	// Issuer is the required `iss` claim value. See auth.Config.Issuer.
	Issuer string
	// Audience is the required `aud` claim value. See auth.Config.Audience.
	Audience string
	// JWKSCacheTTL is how long fetched keys are cached before a background
	// refresh. See auth.Config.JWKSCacheTTL.
	JWKSCacheTTL time.Duration
}

type MCPConfig struct {
	Name      string
	Type      string // "stdio", "sse", etc.
	Command   string
	Arguments []string
}

// ClaimRuleConfig is the YAML shape of a router.ClaimRule. See ADR-0002.
type ClaimRuleConfig struct {
	Path    string
	Pattern string
}

// GrantConfig is the YAML shape of a router.Grant.
type GrantConfig struct {
	MCP   string
	Tools []string
}

// AccessPolicyConfig is the YAML shape of a router.AccessPolicy. See
// ADR-0004 and docs/architecture/data.md#access-policy.
type AccessPolicyConfig struct {
	Name   string
	Match  []ClaimRuleConfig
	Grants []GrantConfig
}

// FilterPolicyConfig is the YAML shape of a router.FilterPolicy. See
// ADR-0004 and docs/architecture/data.md#filter-policy.
type FilterPolicyConfig struct {
	Name       string
	Match      []ClaimRuleConfig
	MCP        string
	Tool       string
	DropFields []string
}

func Load(path string) (*Config, error) {
	// TODO: Load YAML configuration
	return nil, fmt.Errorf("not implemented")
}

// Validate checks that Config is structurally sound before the gateway
// starts serving. Its primary purpose (per ADR-0002/ADR-0004) is compiling
// every AccessPolicy/FilterPolicy claim rule up front: a malformed JSONPath
// expression or regexp is a configuration error to catch at startup, not a
// per-request failure discovered later.
func (c *Config) Validate() error {
	if c.Server.DataPlaneAddr == "" {
		return fmt.Errorf("config: server.data_plane_addr is required")
	}
	if c.OIDC.JWKSURL == "" {
		return fmt.Errorf("config: oidc.jwks_url is required")
	}
	if c.OIDC.Issuer == "" {
		return fmt.Errorf("config: oidc.issuer is required")
	}
	if c.OIDC.Audience == "" {
		return fmt.Errorf("config: oidc.audience is required")
	}

	for i, policy := range c.AccessPolicies {
		if policy.Name == "" {
			return fmt.Errorf("config: access_policies[%d]: name is required", i)
		}
		if err := validateClaimRules(policy.Match); err != nil {
			return fmt.Errorf("config: access_policies[%d] (%s): %w", i, policy.Name, err)
		}
		for j, grant := range policy.Grants {
			if grant.MCP == "" {
				return fmt.Errorf("config: access_policies[%d] (%s): grants[%d]: mcp is required", i, policy.Name, j)
			}
		}
	}

	for i, policy := range c.FilterPolicies {
		if policy.Name == "" {
			return fmt.Errorf("config: filter_policies[%d]: name is required", i)
		}
		if policy.MCP == "" {
			return fmt.Errorf("config: filter_policies[%d] (%s): mcp is required", i, policy.Name)
		}
		if policy.Tool == "" {
			return fmt.Errorf("config: filter_policies[%d] (%s): tool is required", i, policy.Name)
		}
		if err := validateClaimRules(policy.Match); err != nil {
			return fmt.Errorf("config: filter_policies[%d] (%s): %w", i, policy.Name, err)
		}
	}

	return nil
}

// validateClaimRules compiles every rule via router.NewClaimRule, returning
// the first error encountered, identifying which rule failed.
func validateClaimRules(rules []ClaimRuleConfig) error {
	for i, rule := range rules {
		if _, err := router.NewClaimRule(rule.Path, rule.Pattern); err != nil {
			return fmt.Errorf("match[%d]: %w", i, err)
		}
	}
	return nil
}
