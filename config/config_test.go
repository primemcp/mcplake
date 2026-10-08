package config_test

import (
	"testing"

	"github.com/primemcp/mcplake/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validConfig() config.Config {
	return config.Config{
		Server: config.ServerConfig{DataPlaneAddr: ":8080"},
		OIDC: config.OIDCConfig{
			JWKSURL:  "https://auth.example.com/.well-known/jwks.json",
			Issuer:   "https://auth.example.com",
			Audience: "mcp-gateway",
		},
	}
}

func TestConfig_Validate_ValidMinimalConfig(t *testing.T) {
	cfg := validConfig()

	assert.NoError(t, cfg.Validate())
}

func TestConfig_Validate_RequiresServerDataPlaneAddr(t *testing.T) {
	cfg := validConfig()
	cfg.Server.DataPlaneAddr = ""

	assert.Error(t, cfg.Validate())
}

func TestConfig_Validate_RequiresOIDCFields(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*config.Config)
	}{
		{"missing jwks_url", func(c *config.Config) { c.OIDC.JWKSURL = "" }},
		{"missing issuer", func(c *config.Config) { c.OIDC.Issuer = "" }},
		{"missing audience", func(c *config.Config) { c.OIDC.Audience = "" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			tt.mutate(&cfg)
			assert.Error(t, cfg.Validate())
		})
	}
}

func TestConfig_Validate_AccessPolicyRequiresName(t *testing.T) {
	cfg := validConfig()
	cfg.AccessPolicies = []config.AccessPolicyConfig{
		{
			Match:  []config.ClaimRuleConfig{{Path: "$.role", Pattern: "^db-reader$"}},
			Grants: []config.GrantConfig{{MCP: "postgres-ro", Tools: []string{"*"}}},
		},
	}

	err := cfg.Validate()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "name is required")
}

func TestConfig_Validate_AccessPolicyRejectsMalformedJSONPath(t *testing.T) {
	cfg := validConfig()
	cfg.AccessPolicies = []config.AccessPolicyConfig{
		{
			Name:   "db-reader",
			Match:  []config.ClaimRuleConfig{{Path: "not a valid jsonpath $$", Pattern: "^db-reader$"}},
			Grants: []config.GrantConfig{{MCP: "postgres-ro", Tools: []string{"*"}}},
		},
	}

	err := cfg.Validate()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "access_policies[0]")
	assert.Contains(t, err.Error(), "db-reader")
}

func TestConfig_Validate_AccessPolicyRejectsMalformedRegexp(t *testing.T) {
	cfg := validConfig()
	cfg.AccessPolicies = []config.AccessPolicyConfig{
		{
			Name:   "db-reader",
			Match:  []config.ClaimRuleConfig{{Path: "$.role", Pattern: "(unclosed"}},
			Grants: []config.GrantConfig{{MCP: "postgres-ro", Tools: []string{"*"}}},
		},
	}

	assert.Error(t, cfg.Validate())
}

func TestConfig_Validate_AccessPolicyRequiresGrantMCP(t *testing.T) {
	cfg := validConfig()
	cfg.AccessPolicies = []config.AccessPolicyConfig{
		{
			Name:   "db-reader",
			Match:  []config.ClaimRuleConfig{{Path: "$.role", Pattern: "^db-reader$"}},
			Grants: []config.GrantConfig{{Tools: []string{"*"}}},
		},
	}

	err := cfg.Validate()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "mcp is required")
}

func TestConfig_Validate_ValidAccessPolicyPasses(t *testing.T) {
	cfg := validConfig()
	cfg.AccessPolicies = []config.AccessPolicyConfig{
		{
			Name:   "db-reader",
			Match:  []config.ClaimRuleConfig{{Path: "$.role", Pattern: "^db-reader$"}},
			Grants: []config.GrantConfig{{MCP: "postgres-ro", Tools: []string{"*"}}},
		},
	}

	assert.NoError(t, cfg.Validate())
}

func TestConfig_Validate_FilterPolicyRequiresNameMCPAndTool(t *testing.T) {
	tests := []struct {
		name   string
		policy config.FilterPolicyConfig
	}{
		{
			name: "missing name",
			policy: config.FilterPolicyConfig{
				MCP: "postgres-ro", Tool: "get_user",
				Match: []config.ClaimRuleConfig{{Path: "$.role", Pattern: "^user$"}},
			},
		},
		{
			name: "missing mcp",
			policy: config.FilterPolicyConfig{
				Name: "hide-pii", Tool: "get_user",
				Match: []config.ClaimRuleConfig{{Path: "$.role", Pattern: "^user$"}},
			},
		},
		{
			name: "missing tool",
			policy: config.FilterPolicyConfig{
				Name: "hide-pii", MCP: "postgres-ro",
				Match: []config.ClaimRuleConfig{{Path: "$.role", Pattern: "^user$"}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			cfg.FilterPolicies = []config.FilterPolicyConfig{tt.policy}
			assert.Error(t, cfg.Validate())
		})
	}
}

func TestConfig_Validate_FilterPolicyRejectsMalformedJSONPath(t *testing.T) {
	cfg := validConfig()
	cfg.FilterPolicies = []config.FilterPolicyConfig{
		{
			Name:       "hide-pii",
			MCP:        "postgres-ro",
			Tool:       "get_user",
			Match:      []config.ClaimRuleConfig{{Path: "$$invalid", Pattern: "^user$"}},
			DropFields: []string{"$.email"},
		},
	}

	err := cfg.Validate()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "filter_policies[0]")
	assert.Contains(t, err.Error(), "hide-pii")
}

func TestConfig_Validate_ValidFilterPolicyPasses(t *testing.T) {
	cfg := validConfig()
	cfg.FilterPolicies = []config.FilterPolicyConfig{
		{
			Name:       "hide-pii",
			MCP:        "postgres-ro",
			Tool:       "get_user",
			Match:      []config.ClaimRuleConfig{{Path: "$.role", Pattern: "^user$"}},
			DropFields: []string{"$.email", "$.hashed_password"},
		},
	}

	assert.NoError(t, cfg.Validate())
}
