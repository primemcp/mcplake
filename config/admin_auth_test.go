package config_test

import (
	"testing"

	"github.com/atsokha/mcplake/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const adminAuthBaseConfig = `
server:
  data_plane_addr: ":8080"
oidc:
  jwks_url: "https://auth.example.com/jwks.json"
  issuer: "https://auth.example.com"
  audience: "mcp-gateway"
`

func TestLoad_AdminAuthAbsentLeavesControlPlaneOpen(t *testing.T) {
	cfg, err := config.Load(writeConfig(t, adminAuthBaseConfig))
	require.NoError(t, err)

	assert.False(t, cfg.AdminAuthEnabled())
	assert.Empty(t, cfg.AdminAuth.Match)
	assert.Nil(t, cfg.AdminAuth.Enabled)
}

func TestLoad_AdminAuthWithMatchRulesEnablesIt(t *testing.T) {
	body := adminAuthBaseConfig + `
admin_auth:
  match:
    - path: "$.role"
      pattern: "^admin$"
    - path: "$.iss"
      pattern: "^https://auth\\.example\\.com$"
`
	cfg, err := config.Load(writeConfig(t, body))
	require.NoError(t, err)

	assert.True(t, cfg.AdminAuthEnabled())

	matcher, err := cfg.AdminAuthMatcher()
	require.NoError(t, err)
	require.Len(t, matcher.Rules, 2)

	ok, err := matcher.Matches([]byte(`{"role":"admin","iss":"https://auth.example.com"}`))
	require.NoError(t, err)
	assert.True(t, ok)

	ok, err = matcher.Matches([]byte(`{"role":"db-reader","iss":"https://auth.example.com"}`))
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestLoad_AdminAuthExplicitEnabledFalseStaysOpenEvenWithRules(t *testing.T) {
	body := adminAuthBaseConfig + `
admin_auth:
  enabled: false
  match:
    - path: "$.role"
      pattern: "^admin$"
`
	cfg, err := config.Load(writeConfig(t, body))
	require.NoError(t, err)

	assert.False(t, cfg.AdminAuthEnabled())
}

func TestLoad_AdminAuthEnabledTrueWithoutRulesIsRejected(t *testing.T) {
	body := adminAuthBaseConfig + `
admin_auth:
  enabled: true
`
	_, err := config.Load(writeConfig(t, body))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "admin_auth.match must contain at least one rule")
}

func TestLoad_AdminAuthRejectsMalformedMatchRule(t *testing.T) {
	body := adminAuthBaseConfig + `
admin_auth:
  match:
    - path: "$.role"
      pattern: "(unclosed"
`
	_, err := config.Load(writeConfig(t, body))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "admin_auth")
}

func TestConfig_Validate_AdminAuthEnabledPointerVariants(t *testing.T) {
	enabled, disabled := true, false
	tests := []struct {
		name    string
		admin   config.AdminAuthConfig
		want    bool
		wantErr bool
	}{
		{"nil enabled, no rules", config.AdminAuthConfig{}, false, false},
		{
			name:  "nil enabled, one rule",
			admin: config.AdminAuthConfig{Match: []config.ClaimRuleConfig{{Path: "$.role", Pattern: "^admin$"}}},
			want:  true,
		},
		{
			name:  "explicit true, one rule",
			admin: config.AdminAuthConfig{Enabled: &enabled, Match: []config.ClaimRuleConfig{{Path: "$.role", Pattern: "^admin$"}}},
			want:  true,
		},
		{"explicit true, no rules", config.AdminAuthConfig{Enabled: &enabled}, false, true},
		{
			name:  "explicit false, one rule",
			admin: config.AdminAuthConfig{Enabled: &disabled, Match: []config.ClaimRuleConfig{{Path: "$.role", Pattern: "^admin$"}}},
			want:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			cfg.AdminAuth = tt.admin

			err := cfg.Validate()
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, cfg.AdminAuthEnabled())
		})
	}
}
