package config

import (
	"fmt"
	"time"
)

type Config struct {
	Server    ServerConfig
	OIDC      OIDCConfig
	MCPs      []MCPConfig
	Routing   []RoutingRule
	Filtering []FilteringRule
}

type ServerConfig struct {
	Port   int
	UIPort int
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

type RoutingRule struct {
	ClaimKey    string
	ClaimValue  string
	MCPInstance string
	Priority    int
}

type FilteringRule struct {
	ClaimKey   string
	ClaimValue string
	MCPName    string
	ToolName   string
	HideFields []string
}

func Load(path string) (*Config, error) {
	// TODO: Load YAML configuration
	// TODO: Validate configuration
	return nil, fmt.Errorf("not implemented")
}

func (c *Config) Validate() error {
	// TODO: Validate all required fields
	// TODO: Check for conflicts in rules
	return fmt.Errorf("not implemented")
}
