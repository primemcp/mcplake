package config

import (
	"fmt"
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
	ProviderURL string
	ClientID    string
	Audience    string
	// TODO: Add JWKS caching configuration
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
