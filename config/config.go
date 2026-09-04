package config

import (
	"fmt"
	"os"
	"time"

	"github.com/atsokha/mcplake/router"
	"go.yaml.in/yaml/v3"
)

type Config struct {
	Server         ServerConfig         `yaml:"server"`
	OIDC           OIDCConfig           `yaml:"oidc"`
	MCPs           []MCPConfig          `yaml:"mcps"`
	AccessPolicies []AccessPolicyConfig `yaml:"access_policies"`
	FilterPolicies []FilterPolicyConfig `yaml:"filter_policies"`
}

type ServerConfig struct {
	// DataPlaneAddr is the listen address (host:port) for the fasthttp
	// tool-call proxy. See ADR-0001.
	DataPlaneAddr string `yaml:"data_plane_addr"`
	// ControlPlaneAddr is the listen address (host:port) for the Gin admin
	// API. See ADR-0005.
	ControlPlaneAddr string `yaml:"control_plane_addr"`
	// TODO: Add TLS configuration
}

type OIDCConfig struct {
	// JWKSURL is the OIDC provider's JWKS endpoint. See auth.Config.JWKSURL;
	// OIDC discovery from a provider/issuer URL is not yet implemented.
	JWKSURL string `yaml:"jwks_url"`
	// Issuer is the required `iss` claim value. See auth.Config.Issuer.
	Issuer string `yaml:"issuer"`
	// Audience is the required `aud` claim value. See auth.Config.Audience.
	Audience string `yaml:"audience"`
	// JWKSCacheTTL is how long fetched keys are cached before a background
	// refresh. See auth.Config.JWKSCacheTTL.
	JWKSCacheTTL time.Duration `yaml:"-"`
}

// oidcConfigYAML mirrors OIDCConfig but with JWKSCacheTTL as a duration
// string (e.g. "1h"), since time.Duration has no native YAML representation.
type oidcConfigYAML struct {
	JWKSURL      string `yaml:"jwks_url"`
	Issuer       string `yaml:"issuer"`
	Audience     string `yaml:"audience"`
	JWKSCacheTTL string `yaml:"jwks_cache_ttl"`
}

// UnmarshalYAML decodes JWKSCacheTTL from its string form (e.g. "1h") via
// time.ParseDuration; an empty/absent value leaves JWKSCacheTTL at its zero
// value, letting callers (e.g. auth.NewValidator) apply their own default.
func (c *OIDCConfig) UnmarshalYAML(node *yaml.Node) error {
	var raw oidcConfigYAML
	if err := node.Decode(&raw); err != nil {
		return err
	}
	c.JWKSURL = raw.JWKSURL
	c.Issuer = raw.Issuer
	c.Audience = raw.Audience
	if raw.JWKSCacheTTL == "" {
		c.JWKSCacheTTL = 0
		return nil
	}
	ttl, err := time.ParseDuration(raw.JWKSCacheTTL)
	if err != nil {
		return fmt.Errorf("config: oidc.jwks_cache_ttl: %w", err)
	}
	c.JWKSCacheTTL = ttl
	return nil
}

type MCPConfig struct {
	Name      string   `yaml:"name"`
	Type      string   `yaml:"type"` // "stdio", "sse", etc.
	Command   string   `yaml:"command"`
	Arguments []string `yaml:"arguments"`
}

// ClaimRuleConfig is the YAML shape of a router.ClaimRule. See ADR-0002.
type ClaimRuleConfig struct {
	Path    string `yaml:"path"`
	Pattern string `yaml:"pattern"`
}

// GrantConfig is the YAML shape of a router.Grant.
type GrantConfig struct {
	MCP   string   `yaml:"mcp"`
	Tools []string `yaml:"tools"`
}

// AccessPolicyConfig is the YAML shape of a router.AccessPolicy. See
// ADR-0004 and docs/architecture/data.md#access-policy.
type AccessPolicyConfig struct {
	Name   string            `yaml:"name"`
	Match  []ClaimRuleConfig `yaml:"match"`
	Grants []GrantConfig     `yaml:"grants"`
}

// FilterPolicyConfig is the YAML shape of a router.FilterPolicy. See
// ADR-0004 and docs/architecture/data.md#filter-policy.
type FilterPolicyConfig struct {
	Name       string            `yaml:"name"`
	Match      []ClaimRuleConfig `yaml:"match"`
	MCP        string            `yaml:"mcp"`
	Tool       string            `yaml:"tool"`
	DropFields []string          `yaml:"drop_fields"`
}

// Load reads and parses the YAML configuration file at path, then runs
// Validate on the result so a malformed policy rule (or any other
// structural problem Validate checks) is reported at load time rather than
// discovered later during request handling.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("config: parse %s: %w", path, err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
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
