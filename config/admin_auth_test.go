package config_test

import (
	"testing"

	"github.com/atsokha/mcplake/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const adminAuthBaseConfig = `
[server]
data_plane_addr = ":8080"

[oidc]
jwks_url = "https://auth.example.com/jwks.json"
issuer = "https://auth.example.com"
audience = "mcp-gateway"
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
[[admin_auth.match]]
path = "$.role"
pattern = "^admin$"
[[admin_auth.match]]
path = "$.iss"
pattern = "^https://auth\\.example\\.com$"
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
[admin_auth]
enabled = false
[[admin_auth.match]]
path = "$.role"
pattern = "^admin$"
`
	cfg, err := config.Load(writeConfig(t, body))
	require.NoError(t, err)

	assert.False(t, cfg.AdminAuthEnabled())
}

func TestLoad_AdminAuthEnabledTrueWithoutRulesIsRejected(t *testing.T) {
	body := adminAuthBaseConfig + `
[admin_auth]
enabled = true
`
	_, err := config.Load(writeConfig(t, body))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "admin_auth.match must contain at least one rule")
}

func TestLoad_AdminAuthRejectsMalformedMatchRule(t *testing.T) {
	body := adminAuthBaseConfig + `
[[admin_auth.match]]
path = "$.role"
pattern = "(unclosed"
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

// --- admin_auth.login: how the admin web UI obtains a token (ADR-0014) ---

func TestLoad_AdminLoginAbsentIsNotConfigured(t *testing.T) {
	cfg, err := config.Load(writeConfig(t, adminAuthBaseConfig))
	require.NoError(t, err)

	login, ok := cfg.AdminLogin()

	assert.False(t, ok)
	assert.Zero(t, login)
}

func TestLoad_AdminLoginComplete(t *testing.T) {
	body := adminAuthBaseConfig + `
[[admin_auth.match]]
path = "$.role"
pattern = "^admin$"

[admin_auth.login]
client_id = "mcplake-admin-ui"
authorization_endpoint = "https://auth.example.com/authorize"
token_endpoint = "https://auth.example.com/oauth/token"
scopes = ["openid", "email"]
`
	cfg, err := config.Load(writeConfig(t, body))
	require.NoError(t, err)

	login, ok := cfg.AdminLogin()

	require.True(t, ok)
	assert.Equal(t, "mcplake-admin-ui", login.ClientID)
	assert.Equal(t, "https://auth.example.com/authorize", login.AuthorizationEndpoint)
	assert.Equal(t, "https://auth.example.com/oauth/token", login.TokenEndpoint)
	assert.Equal(t, []string{"openid", "email"}, login.ScopesOrDefault())
}

func TestLoad_AdminLoginOmittedScopesDefaultToOpenIDProfileEmail(t *testing.T) {
	body := adminAuthBaseConfig + `
[admin_auth.login]
client_id = "mcplake-admin-ui"
authorization_endpoint = "https://auth.example.com/authorize"
token_endpoint = "https://auth.example.com/oauth/token"
`
	cfg, err := config.Load(writeConfig(t, body))
	require.NoError(t, err)

	login, ok := cfg.AdminLogin()

	require.True(t, ok)
	assert.Equal(t, []string{"openid", "profile", "email"}, login.ScopesOrDefault())
}

// A half-filled table is a configuration mistake, not a "login is off"
// signal: the UI would redirect to an empty endpoint or with no client_id.
func TestLoad_AdminLoginPartiallyFilledIsRejected(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		missing string
	}{
		{
			name: "no client_id",
			body: `
[admin_auth.login]
authorization_endpoint = "https://auth.example.com/authorize"
token_endpoint = "https://auth.example.com/oauth/token"
`,
			missing: "client_id",
		},
		{
			name: "no authorization_endpoint",
			body: `
[admin_auth.login]
client_id = "mcplake-admin-ui"
token_endpoint = "https://auth.example.com/oauth/token"
`,
			missing: "authorization_endpoint",
		},
		{
			name: "no token_endpoint",
			body: `
[admin_auth.login]
client_id = "mcplake-admin-ui"
authorization_endpoint = "https://auth.example.com/authorize"
`,
			missing: "token_endpoint",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := config.Load(writeConfig(t, adminAuthBaseConfig+tt.body))

			require.Error(t, err)
			assert.Contains(t, err.Error(), "admin_auth.login")
			assert.Contains(t, err.Error(), tt.missing)
		})
	}
}

// The endpoints end up in a browser redirect, so a relative or malformed
// URL has to fail at startup rather than at the operator's first login.
func TestLoad_AdminLoginRejectsNonAbsoluteHTTPEndpoints(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
	}{
		{name: "relative path", endpoint: "/authorize"},
		{name: "scheme only", endpoint: "https://"},
		{name: "not http(s)", endpoint: "ftp://auth.example.com/authorize"},
		{name: "not a url at all", endpoint: "://nope"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := adminAuthBaseConfig + `
[admin_auth.login]
client_id = "mcplake-admin-ui"
authorization_endpoint = "` + tt.endpoint + `"
token_endpoint = "https://auth.example.com/oauth/token"
`
			_, err := config.Load(writeConfig(t, body))

			require.Error(t, err)
			assert.Contains(t, err.Error(), "admin_auth.login.authorization_endpoint")
		})
	}
}

// --- transport security for the URLs the gateway and the browser fetch ---

// The JWKS is the root of trust for every token the gateway accepts. Over
// plaintext, anyone on the path can serve their own key and mint a token
// that satisfies both the data plane and admin_auth.match.
func TestLoad_RejectsPlaintextJWKSURLForANonLoopbackHost(t *testing.T) {
	body := `
[server]
data_plane_addr = ":8080"

[oidc]
jwks_url = "http://auth.example.com/.well-known/jwks.json"
issuer = "https://auth.example.com"
audience = "mcp-gateway"
`
	_, err := config.Load(writeConfig(t, body))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "oidc.jwks_url")
	assert.Contains(t, err.Error(), "https")
}

// Loopback is exempt: the demo stack, the test fixtures and any local
// development provider serve plaintext over an interface no attacker is on.
func TestLoad_AllowsPlaintextJWKSURLOnLoopback(t *testing.T) {
	for _, host := range []string{"127.0.0.1:9999", "localhost:9999", "[::1]:9999"} {
		t.Run(host, func(t *testing.T) {
			body := `
[server]
data_plane_addr = ":8080"

[oidc]
jwks_url = "http://` + host + `/jwks.json"
issuer = "local-demo"
audience = "mcp-gateway"
`
			_, err := config.Load(writeConfig(t, body))

			require.NoError(t, err)
		})
	}
}

// The token endpoint carries the PKCE code and code_verifier from the
// operator's browser; over plaintext both are readable on the wire.
func TestLoad_RejectsPlaintextLoginEndpointsForANonLoopbackHost(t *testing.T) {
	for _, key := range []string{"authorization_endpoint", "token_endpoint"} {
		t.Run(key, func(t *testing.T) {
			endpoints := map[string]string{
				"authorization_endpoint": "https://auth.example.com/authorize",
				"token_endpoint":         "https://auth.example.com/oauth/token",
			}
			endpoints[key] = "http://auth.example.com/plaintext"
			body := adminAuthBaseConfig + `
[admin_auth.login]
client_id = "mcplake-admin-ui"
authorization_endpoint = "` + endpoints["authorization_endpoint"] + `"
token_endpoint = "` + endpoints["token_endpoint"] + `"
`
			_, err := config.Load(writeConfig(t, body))

			require.Error(t, err)
			assert.Contains(t, err.Error(), "admin_auth.login."+key)
			assert.Contains(t, err.Error(), "https")
		})
	}
}

// --- degenerate claim rules ---

// An empty pattern matches everything, so a config that carries one reports
// itself as secure while constraining nothing. router.NewClaimRule rejects
// it; this pins that config surfaces the failure rather than swallowing it.
func TestLoad_RejectsAnEmptyPatternInAdminAuthMatch(t *testing.T) {
	body := adminAuthBaseConfig + `
[[admin_auth.match]]
path = "$.role"
`
	_, err := config.Load(writeConfig(t, body))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "admin_auth")
	assert.Contains(t, err.Error(), "pattern")
}

func TestLoad_RejectsAnEmptyPatternInAnAccessPolicy(t *testing.T) {
	body := adminAuthBaseConfig + `
[[access_policies]]
name = "everyone"
[[access_policies.match]]
path = "$.role"
[[access_policies.grants]]
mcp = "postgres-ro"
tools = ["*"]
`
	_, err := config.Load(writeConfig(t, body))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "access_policies[0]")
	assert.Contains(t, err.Error(), "pattern")
}
